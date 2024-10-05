package packet

import (
	"fmt"
	"io"

	"github.com/rs/zerolog/log"
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

// Theres no guarantee that the first size bytes are received. We will
// await for them to be received, therefor making it "safer".
func safeReadStream(connReader io.Reader, size int) ([]byte, error) {
	buf := make([]byte, size)
	remainingBytes, indexAcc := size, 0
	for {
		if remainingBytes <= 0 {
			log.Debug().
				Int("remainingBytes", remainingBytes).
				Int("indexAcc", indexAcc).
				Int("size", size).
				Msg("safeReadStream")
			break
		}
		_buf := make([]byte, remainingBytes)
		// readNumberOfBytes will NEVER exceed remainingBytes
		readNumberOfBytes, err := connReader.Read(_buf)
		if err != nil {
			return nil, err
		}
		// Allocate _buf to headerBuf
		for i := 0; i < readNumberOfBytes; i++ {
			buf[indexAcc] = _buf[i]
			indexAcc++
		}
		remainingBytes -= readNumberOfBytes
	}
	return buf, nil
}

func ReadPacket(connReader io.Reader) (*RawPacket, error) {
	// 8 bits version
	// + 8 bits packet type
	// + 8 bits packet size (n bytes, *Packet type should self truncate remainder bits.)
	// + 8*n bits (body) e.g. n=4 bits player id
	// min 3 bytes
	log.Debug().
		Msg("calling safestream on header")
	headerBuf, err := safeReadStream(connReader, 3)
	if err != nil {
		return nil, err
	}

	version := headerBuf[0]
	packetType := headerBuf[1]
	bodyNumberOfBytes := headerBuf[2]
	// fmt.Printf("%v %v\n", version, packetType)
	// playerId := binary.BigEndian.Uint32(headerBuf[2:6])
	log.Debug().
		Int8("version", int8(version)).
		Int8("packet", int8(packetType)).
		Int8("body", int8(bodyNumberOfBytes)).
		Msg("packet header")

	if version != CURRENT_VERSION {
		return nil, ErrVersionMismatch
	}
	log.Debug().
		Int("bodyNumberOfBytes", int(bodyNumberOfBytes)).
		Msg("calling safestream on body")

	// bodyBuf := make([]byte, bodyNumberOfBytes)
	// n, err := connReader.Read(bodyBuf)
	bodyBuf, err := safeReadStream(connReader, int(bodyNumberOfBytes))
	// log.Debug().Int("n", n).Int("bodyNumberOfBytes", int(bodyNumberOfBytes)).Msg("read body")
	if err != nil {
		return nil, err
	}

	p := RawPacket{
		Body:       bodyBuf,
		Version:    version,
		PacketType: PacketType(packetType),
	}

	return &p, nil
}
