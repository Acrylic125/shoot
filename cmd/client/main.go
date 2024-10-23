package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"shoot.me/shoot/packet"
)

type ClientConnection struct {
	conn net.Conn

	// Simple heartbeat detection
	ctx    context.Context
	cancel context.CancelFunc

	started bool
}

func (c *ClientConnection) handlePacket(p *packet.RawPacket) error {
	if p == nil {
		return packet.ErrInvalidPacketType
	}
	switch p.PacketType {
	case packet.Move:
		movePacket, err := packet.ParseMovePacket(p)
		if err != nil {
			return err
		}

		log.Debug().
			Uint32("playerId", movePacket.PlayerId).
			Int32("x", movePacket.PositionX).
			Int32("y", movePacket.PositionY).
			Msg("player move")
		return nil
	}
	return packet.ErrInvalidPacketType
}

func (c *ClientConnection) heartbeat() {
	go func() {
		for {
			select {
			case <-c.ctx.Done():
				log.Debug().Msg("heartbeat loop terminated")
				return
			case <-time.After(time.Second * 1):
				p := packet.HeartbeatPacket{}
				if _, err := c.conn.Write(p.ToBytes()); err != nil {
					log.Error().
						AnErr("err", err).
						Msg("error writing heartbeat")
					c.cancel()
					return
				}
			}
		}
	}()
}

func (c *ClientConnection) Start() error {
	if c.started {
		return fmt.Errorf("connection already started")
	}

	defer c.conn.Close()
	defer func() {
		c.started = false
	}()

	c.started = true
	c.heartbeat()

	packetChan := make(chan *packet.RawPacket)
	readPacketLockMutex := sync.RWMutex{}
	readPacketLock := false
	for {
		// Lock mechanism is required to prevent a race condition.
		// For example, if the select statement gets resolved but it was not resolved from
		// a packet read, this function will be called again causing a race condition.
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
		case p := <-packetChan:
			if p == nil {
				log.Debug().Msg("nil packet received, ending closing connection.")
				c.cancel()
				return nil
			}
			c.handlePacket(p)
		case <-c.ctx.Done():
			log.Debug().
				Str("from", "connection listener").
				Msg("termination signal received, ending closing connection.")
			return nil
		case <-time.After(time.Second * 1):
			log.Debug().Msg("sending packet")
			if _, err := c.conn.Write([]byte{
				0, 1, 12,
				0xf, 0, 0, 1,
				0xff, 0xff, 0xff, 0,
				0, 0, 0, 0xff,
			}); err != nil {
				fmt.Println("Error writing:", err)
				return nil
			}
		}
	}
}

func NewClientConnection(address string) (*ClientConnection, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ClientConnection{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func main() {
	client, err := NewClientConnection("localhost:2222")
	if err != nil {
		log.Error().
			AnErr("err", err).
			Msg("client error")
		return
	}
	client.Start()
}
