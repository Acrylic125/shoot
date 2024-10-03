package main

import (
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
	ErrVersionMismatch   = fmt.Errorf("version mismatch")
	ErrInvalidPacketType = fmt.Errorf("invalid packet type")
)

func ReadToPacket(connReader io.Reader) (*RawPacket, error) {
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
	fmt.Println(bodyBuf)

	packet := RawPacket{
		raw:        bodyBuf,
		Version:    version,
		PacketType: PacketType(packetType),
	}

	switch PacketType(packetType) {
	case Heartbeat:
		// fmt.Printf("heartbeat %v\n", playerId)
		return &packet, nil
	// case PacketTypeHeartbeat, PacketTypeDecrement, PacketTypeIncrement:
	// 	p.PacketType = PacketType(packetType)

	// 	packetData := make([]byte, 2)
	// 	_, err := connReader.Read(packetData)
	// 	if err != nil {
	// 		return err
	// 	}

	// 	p.raw = packetData
	// case PacketTypeGateStatus:
	// 	p.PacketType = PacketTypeGateStatus

	// 	gateStatusPacketData := make([]byte, 7)
	// 	_, err := connReader.Read(gateStatusPacketData)
	// 	if err != nil {
	// 		return err
	// 	}

	// 	p.raw = gateStatusPacketData
	default:
		return nil, ErrInvalidPacketType
	}
}

func (t *TCP) listenConnection(conn net.Conn) {
	defer t.wg.Done()
	defer conn.Close()
	for {
		packet, err := ReadToPacket(conn)

		if err != nil {
			if errors.Is(err, io.EOF) {
				// fmt.Println("EOF")
				slog.Debug("socket received EOF", "error", err)
			} else {
				slog.Error("server error:", "error", err)
			}
			break
		}

		if t.packetsEgress != nil {
			t.packetsEgress <- packet
		}
	}

	// fmt.Println("TTT")
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
				fmt.Println("Quit!")
				return
			default:
				slog.Error("server error:", "error", err)
			}
		}

		fmt.Println("New connection!")
		t.wg.Add(1)
		go t.listenConnection(conn)
	}
}

func main() {
	packetsEgress := make(chan *RawPacket)
	quit := make(chan interface{})
	port := 2222

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		slog.Error("server error:", "error", err)
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
