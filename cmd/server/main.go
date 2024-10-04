package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type PacketType byte

const (
	Heartbeat PacketType = 0
	Move      PacketType = 1
)

type RawPacket struct {
	body       []byte
	Version    byte
	PacketType PacketType
}

type TCP struct {
	listener      net.Listener
	packetsEgress chan<- *RawPacket
	quit          chan interface{}
	wg            sync.WaitGroup
}

func NewTCPServer(port uint16, packetsEgress chan<- *RawPacket) (*TCP, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}

	return &TCP{
		listener:      listener,
		packetsEgress: packetsEgress,
		quit:          make(chan interface{}),
		wg:            sync.WaitGroup{},
	}, nil
}

func (t *TCP) Close() {
	close(t.quit)
	t.listener.Close()
	t.wg.Wait()
}

const CURRENT_VERSION = 0

var (
	ErrVersionMismatch   = fmt.Errorf("version mismatch")
	ErrInvalidPacketType = fmt.Errorf("invalid packet type")
)

func ReadPacket(connReader io.Reader) (*RawPacket, error) {
	// 8 bits version
	// + 8 bits packet type
	// + 8 bits packet size (n bytes, *Packet type should self truncate remainder bits.)
	// + 8*n bits (body) e.g. n=4 bits player id
	// min 3 bytes
	headerBuf := make([]byte, 3)

	if _, err := connReader.Read(headerBuf); err != nil {
		return nil, err
	}

	version := headerBuf[0]
	packetType := headerBuf[1]
	bodyNumberOfBytes := headerBuf[2]
	// fmt.Printf("%v %v\n", version, packetType)
	// playerId := binary.BigEndian.Uint32(headerBuf[2:6])

	if version != CURRENT_VERSION {
		return nil, ErrVersionMismatch
	}

	bodyBuf := make([]byte, bodyNumberOfBytes)
	if _, err := connReader.Read(bodyBuf); err != nil {
		return nil, err
	}

	packet := RawPacket{
		body:       bodyBuf,
		Version:    version,
		PacketType: PacketType(packetType),
	}

	// switch PacketType(packetType) {
	// case Heartbeat:
	// 	// fmt.Printf("heartbeat %v\n", playerId)
	// 	return &packet, nil
	// // case PacketTypeHeartbeat, PacketTypeDecrement, PacketTypeIncrement:
	// // 	p.PacketType = PacketType(packetType)

	// // 	packetData := make([]byte, 2)
	// // 	_, err := connReader.Read(packetData)
	// // 	if err != nil {
	// // 		return err
	// // 	}

	// // 	p.raw = packetData
	// // case PacketTypeGateStatus:
	// // 	p.PacketType = PacketTypeGateStatus

	// // 	gateStatusPacketData := make([]byte, 7)
	// // 	_, err := connReader.Read(gateStatusPacketData)
	// // 	if err != nil {
	// // 		return err
	// // 	}

	// // 	p.raw = gateStatusPacketData
	// default:
	// 	return nil, ErrInvalidPacketType
	// }
	return &packet, nil
}

type Connectio struct {
	// Connection state
	conn net.Conn

	// Simple heartbeat detection
	ctx    context.Context
	cancel context.CancelFunc

	heartbeatPulsedMutex sync.RWMutex
	heartbeatPulsed      bool

	started bool
}

func NewConnection(conn net.Conn) *Connectio {
	ctx, cancel := context.WithCancel(context.Background())
	return &Connectio{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,

		heartbeatPulsedMutex: sync.RWMutex{},
		heartbeatPulsed:      true,

		started: false,
	}
}

func (c *Connectio) handlePacket(packet *RawPacket) error {
	if packet == nil {
		return ErrInvalidPacketType
	}
	switch packet.PacketType {
	case Heartbeat:
		log.Debug().Msg("heartbeat received")
		c.setHeartbeatPulsed(true)
		return nil
	case Move:
		if len(packet.body) < 12 {
			return fmt.Errorf("packet body too short - requires 12 bytes, needs to conform to [4b playerId] [4b positionX] [4b positionY]")
		}
		playerId := binary.BigEndian.Uint32(packet.body[0:4])
		posX := int32(binary.BigEndian.Uint32(packet.body[4:8]))
		posY := int32(binary.BigEndian.Uint32(packet.body[8:12]))

		log.Debug().
			Uint32("playerId", playerId).
			Int32("x", posX).
			Int32("y", posY).
			Msg("player move")
		return nil
	}
	return ErrInvalidPacketType
}

func (c *Connectio) setHeartbeatPulsed(heartbeatPulsed bool) {
	c.heartbeatPulsedMutex.Lock()
	c.heartbeatPulsed = heartbeatPulsed
	c.heartbeatPulsedMutex.Unlock()
}

func (c *Connectio) isHeartbeatPulsed() bool {
	c.heartbeatPulsedMutex.RLock()
	heartbeatPulsed := c.heartbeatPulsed
	c.heartbeatPulsedMutex.RUnlock()
	return heartbeatPulsed
}

func (c *Connectio) listen(t *TCP) error {
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

	for {
		packetChan := make(chan *RawPacket)

		go func() {
			packet, err := ReadPacket(c.conn)
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
			packetChan <- packet
		}()

		select {
		case packet := <-packetChan:
			if packet != nil {
				if err := c.handlePacket(packet); err != nil {
					log.Error().
						AnErr("err", err).
						Msg("error handling packet")
					c.cancel()
					return nil
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

func (t *TCP) Start() {
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
			_conn.listen(t)
		}()
	}
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	packetsEgress := make(chan *RawPacket)
	quit := make(chan interface{})
	port := 2222

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Error().
			AnErr("err", err).
			Msg("server error")
		return
	}

	tcp := TCP{
		listener:      listener,
		packetsEgress: packetsEgress,
		quit:          quit,
		wg:            sync.WaitGroup{},
	}
	tcp.Start()
}
