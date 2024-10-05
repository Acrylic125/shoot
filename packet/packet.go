package packet

import (
	"fmt"
	"io"
)

const CURRENT_VERSION = 0

var (
	ErrVersionMismatch   = fmt.Errorf("version mismatch")
	ErrInvalidPacketType = fmt.Errorf("invalid packet type")
)

type PacketType byte

const (
	Heartbeat PacketType = 0
	Move      PacketType = 1
)

type RawPacket struct {
	Body       []byte
	Version    byte
	PacketType PacketType
}

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

	p := RawPacket{
		Body:       bodyBuf,
		Version:    version,
		PacketType: PacketType(packetType),
	}

	return &p, nil
}
