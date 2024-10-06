package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"shoot.me/shoot/packet"
)

type TCPServer struct {
	listener net.Listener

	quit chan interface{}
	wg   sync.WaitGroup

	connectionsMutex sync.RWMutex
	connections      []*ServerSideClientConnection
}

func NewTCPServer(port int) (*TCPServer, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}

	return &TCPServer{
		listener: listener,
		quit:     make(chan interface{}),
		wg:       sync.WaitGroup{},
	}, nil
}

func (t *TCPServer) Close() {
	close(t.quit)
	t.listener.Close()
	t.wg.Wait()
}

const CURRENT_VERSION = 0

var (
	ErrVersionMismatch   = fmt.Errorf("version mismatch")
	ErrInvalidPacketType = fmt.Errorf("invalid packet type")
)

type ServerSideClientConnection struct {
	// Connection state
	conn net.Conn

	// Simple heartbeat detection
	ctx    context.Context
	cancel context.CancelFunc

	heartbeatPulsedMutex sync.RWMutex
	heartbeatPulsed      bool

	started bool
}

func NewConnection(conn net.Conn) *ServerSideClientConnection {
	ctx, cancel := context.WithCancel(context.Background())
	return &ServerSideClientConnection{
		conn: conn,

		ctx:    ctx,
		cancel: cancel,

		heartbeatPulsedMutex: sync.RWMutex{},
		heartbeatPulsed:      true,

		started: false,
	}
}

func (c *ServerSideClientConnection) handlePacket(p *packet.RawPacket) (egress *[]*packet.RawPacket, err error) {
	if p == nil {
		return nil, ErrInvalidPacketType
	}
	switch p.PacketType {
	case packet.Heartbeat:
		log.Debug().Msg("heartbeat received")
		c.setHeartbeatPulsed(true)
		return nil, nil
	case packet.Move:
		movePacket, err := packet.ParseMovePacket(p)
		if err != nil {
			return nil, err
		}

		log.Debug().
			Uint32("playerId", movePacket.PlayerId).
			Int32("x", movePacket.PositionX).
			Int32("y", movePacket.PositionY).
			Msg("player move")

		log.Debug().Msg("sending packet to egress")
		return &[]*packet.RawPacket{
			{
				Body:       p.Body,
				Version:    p.Version,
				PacketType: p.PacketType,
			},
		}, nil
	}
	return nil, ErrInvalidPacketType
}

func (c *ServerSideClientConnection) setHeartbeatPulsed(heartbeatPulsed bool) {
	c.heartbeatPulsedMutex.Lock()
	c.heartbeatPulsed = heartbeatPulsed
	c.heartbeatPulsedMutex.Unlock()
}

func (c *ServerSideClientConnection) isHeartbeatPulsed() bool {
	c.heartbeatPulsedMutex.RLock()
	heartbeatPulsed := c.heartbeatPulsed
	c.heartbeatPulsedMutex.RUnlock()
	return heartbeatPulsed
}

func (c *ServerSideClientConnection) Listen(t *TCPServer) error {
	if c.started {
		return fmt.Errorf("connection already listening")
	}
	defer t.wg.Done()
	defer c.conn.Close()
	defer func() {
		c.started = false
	}()

	c.started = true

	// Connection state
	go func() {
		for {
			select {
			case <-c.ctx.Done():
				log.Debug().
					Str("from", "heartbeat loop").
					Msg("connection is no longer alive, ending heartbeat loop.")
				return
			case <-time.After(time.Second * 5):
			}

			_heartbeatPulsed := c.isHeartbeatPulsed()
			if !_heartbeatPulsed {
				log.Debug().
					Str("from", "heartbeat loop").
					Msg("force closing connection.")
				c.cancel()
				break
			}

			c.setHeartbeatPulsed(false)
		}
	}()

	packetChan := make(chan *packet.RawPacket)
	readPacketLockMutex := sync.RWMutex{}
	readPacketLock := false
	for {
		go func() {
			readPacketLockMutex.Lock()
			_readPacketLock := readPacketLock
			if !_readPacketLock {
				readPacketLock = true
				defer func() {
					readPacketLockMutex.Lock()
					readPacketLock = false
					readPacketLockMutex.Unlock()
				}()
			}
			readPacketLockMutex.Unlock()
			if _readPacketLock {
				return
			}

			p, err := packet.ReadPacket(c.conn)
			if err != nil {
				if errors.Is(err, io.EOF) {
					log.Debug().
						AnErr("err", err).
						Msg("socket received EOF")
				} else {
					log.Error().
						AnErr("err", err).
						Msg("server error")
				}
				packetChan <- nil
				return
			}
			packetChan <- p
		}()

		select {
		case packet := <-packetChan:
			if packet != nil {
				egress, err := c.handlePacket(packet)
				if err != nil {
					log.Error().
						AnErr("err", err).
						Msg("error handling packet")
					c.cancel()
					return nil
				}
				if egress != nil {
					t.Broadcast(*egress)
				}
			} else {
				log.Debug().Msg("nil packet received, ending closing connection.")
				c.cancel()
				return nil
			}
		case <-c.ctx.Done():
			log.Debug().
				Str("from", "connection listener").
				Msg("termination signal received, ending closing connection.")
			return nil
		}
	}
}

func (t *TCPServer) Broadcast(packets []*packet.RawPacket) {
	all := make([]byte, 0)
	for _, p := range packets {
		all = append(all, p.Version, byte(p.PacketType), byte(len(p.Body)))
		all = append(all, p.Body...)
	}

	t.connectionsMutex.RLock()
	wg := sync.WaitGroup{}
	wg.Add(len(t.connections))
	for _, conn := range t.connections {
		// No point awaiting for the previous iteration to finish
		go func() {
			conn.conn.Write(all)
			wg.Done()
		}()
	}
	wg.Wait()
	t.connectionsMutex.RUnlock()
}

func (t *TCPServer) Start() {
	t.wg.Add(1)
	defer t.wg.Done()

	for {
		conn, err := t.listener.Accept()
		if err != nil {
			select {
			case <-t.quit:
				log.Debug().Msg("quit")
				return
			default:
				log.Error().
					AnErr("err", err).
					Msg("server error")
			}
		}

		t.wg.Add(1)
		go func() {
			_conn := NewConnection(conn)
			t.connectionsMutex.Lock()
			t.connections = append(t.connections, _conn)
			t.connectionsMutex.Unlock()

			_conn.Listen(t)
			defer func() {
				t.connectionsMutex.Lock()
				defer t.connectionsMutex.Unlock()

				foundId := -1
				for i, conn := range t.connections {
					if conn == _conn {
						foundId = i
						break
					}
				}
				if foundId != -1 {
					t.connections = append(t.connections[:foundId], t.connections[foundId+1:]...)
				}
			}()
		}()
	}
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	port := 2222

	tcp, err := NewTCPServer(port)
	if err != nil {
		log.Error().
			AnErr("err", err).
			Msg("server error")
		return
	}

	tcp.Start()
}
