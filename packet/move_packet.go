package packet

import (
	"encoding/binary"
	"fmt"

	"github.com/rs/zerolog/log"
)

const Move PacketType = 1

type MovePacket struct {
	PlayerId  uint32
	PositionX int32
	PositionY int32
}

func (p *MovePacket) ToBytes() []byte {
	buf := GenPacket(Move, 12)
	offset := len(buf)

	binary.BigEndian.PutUint32(buf[offset+0:offset+4], p.PlayerId)
	binary.BigEndian.PutUint32(buf[offset+4:offset+8], uint32(p.PositionX))
	binary.BigEndian.PutUint32(buf[offset+8:offset+12], uint32(p.PositionY))
	return buf
}

func ParseMovePacket(p *RawPacket) (*MovePacket, error) {
	if len(p.Body) < 12 {
		log.Error().Bytes("body", p.Body).Msg("packet body too short")
		return nil, fmt.Errorf("packet body too short - requires 12 bytes, needs to conform to [4b playerId] [4b positionX] [4b positionY] - received %d bytes", len(p.Body))
	}
	if p.PacketType != Move {
		return nil, ErrInvalidPacketType
	}
	playerId := binary.BigEndian.Uint32(p.Body[0:4])
	posX := int32(binary.BigEndian.Uint32(p.Body[4:8]))
	posY := int32(binary.BigEndian.Uint32(p.Body[8:12]))

	return &MovePacket{
		PlayerId:  playerId,
		PositionX: posX,
		PositionY: posY,
	}, nil
}
