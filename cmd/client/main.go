package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
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

func (c *ClientConnection) genPacketHeader(packetType packet.PacketType, bodySize byte) []byte {
	packetHeader := make([]byte, 3)
	packetHeader[0] = 0
	packetHeader[1] = byte(packetType)
	packetHeader[2] = bodySize
	return packetHeader
}

func (c *ClientConnection) handlePacket(p *packet.RawPacket) error {
	if p == nil {
		return packet.ErrInvalidPacketType
	}
	switch p.PacketType {
	case packet.Move:
		if len(p.Body) < 12 {
			return fmt.Errorf("packet body too short - requires 12 bytes, needs to conform to [4b playerId] [4b positionX] [4b positionY]")
		}
		playerId := binary.BigEndian.Uint32(p.Body[0:4])
		posX := int32(binary.BigEndian.Uint32(p.Body[4:8]))
		posY := int32(binary.BigEndian.Uint32(p.Body[8:12]))

		log.Debug().
			Uint32("playerId", playerId).
			Int32("x", posX).
			Int32("y", posY).
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
				if _, err := c.conn.Write(c.genPacketHeader(packet.Heartbeat, 0)); err != nil {
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
	for {
		packetChan := make(chan *packet.RawPacket)

		go func() {
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

func NewClientConnection(conn net.Conn) *ClientConnection {
	ctx, cancel := context.WithCancel(context.Background())
	return &ClientConnection{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}
}

func main() {
	conn, err := net.Dial("tcp", "localhost:2222")
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}
	// defer conn.Close()

	client := NewClientConnection(conn)
	client.Start()

	// go func() {
	// 	for {
	// 		if _, err := conn.Read([]byte{
	// 			0, 0, 0,
	// 		}); err != nil {
	// 			fmt.Println("Error writing:", err)
	// 			return
	// 		}

	// 	}
	// }()

	// for {
	// 	if _, err := conn.Write([]byte{
	// 		0, 0, 0,
	// 	}); err != nil {
	// 		fmt.Println("Error writing:", err)
	// 		return
	// 	}
	// 	if _, err := conn.Write([]byte{
	// 		0, 1, 12,
	// 		0xf, 0, 0, 1,
	// 		0xff, 0xff, 0xff, 0,
	// 		0, 0, 0, 0xff,
	// 	}); err != nil {
	// 		fmt.Println("Error writing:", err)
	// 		return
	// 	}
	// 	time.Sleep(1 * time.Second)
	// }
}
