package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
)

type PacketType byte

const (
	Heartbeat PacketType = 0
)

type RawPacket struct {
	raw        []byte
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
	ErrVersionMismatch = fmt.Errorf("version mismatch")
)

// func getHeaders(b byte) {

// }

func ReadToPacket(connReader io.Reader) (*RawPacket, error) {
	// 8 bits version + 8 bits packet type + 32 bits player id
	// 6 bytes
	headerBuf := make([]byte, 6)

	_, err := connReader.Read(headerBuf)
	if err != nil {
		return nil, err
	}

	version := headerBuf[0]
	packetType := headerBuf[1]
	playerId := binary.BigEndian.Uint32(headerBuf[2:6])

	if version != CURRENT_VERSION {
		return nil, ErrVersionMismatch
	}

	packet := RawPacket{
		raw,
	}

	p.Version = version

	switch PacketType(packetType) {
	case PacketTypeHeartbeat, PacketTypeDecrement, PacketTypeIncrement:
		p.PacketType = PacketType(packetType)

		packetData := make([]byte, 2)
		_, err := connReader.Read(packetData)
		if err != nil {
			return err
		}

		p.raw = packetData
	case PacketTypeGateStatus:
		p.PacketType = PacketTypeGateStatus

		gateStatusPacketData := make([]byte, 7)
		_, err := connReader.Read(gateStatusPacketData)
		if err != nil {
			return err
		}

		p.raw = gateStatusPacketData
	default:
		return ErrInvalidPacketType
	}

	return nil
}

func (t *TCP) readConnection(conn net.Conn) {
	defer t.wg.Done()
	defer conn.Close()
	for {
		rawPacket := &RawPacket{}

		err := rawPacket.ReadPackets(conn)
		if err != nil {
			if errors.Is(err, io.EOF) {
				slog.Debug("socket received EOF", "error", err)
			} else {
				slog.Error("server error:", "error", err)
			}
			break
		}

		if t.packetsEgress != nil {
			t.packetsEgress <- rawPacket
		}
	}

	slog.Debug("finished reading from connection")
}

func (t *TCP) Start() {
	t.wg.Add(1)
	defer t.wg.Done()

	for {
		conn, err := t.listener.Accept()
		if err != nil {
			select {
			case <-t.quit:
				return
			default:
				slog.Error("server error:", "error", err)
			}
		}

		t.wg.Add(1)
		go t.readConnection(conn)
	}
}

func main() {
	fmt.Println("Hello, world!")
}
