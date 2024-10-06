/**
 * Packet: FireProjectile - Signals to the server that the player fired off a projectile.
 * Used for client -> server
 *
 * Format: [1b version=0] [1b packetType=3] [1b bodySize=13] [4b playerId] [4b positionX] [4b positionY] [1b direction]
 * Direction is encoded in 8 bits, 2 bits for X and 2 bits for Y, 2 bit signed representation for X and Y. 4 unused (Could be used for other things later)
 *
 * Example:
 * 0x00 0x01 0x0C (Head)
 * 0xff 0xff 0xff 0xff (Player ID - 0xffffffff)
 * 0x00 0x00 0x00 0x00 (Position X - 0)
 * 0x00 0x00 0x00 0x00 (Position Y - 0)
 * 0b1101_0000 (Direction (x=-1, y=1))
 **/

package packet

import (
	"encoding/binary"
	"fmt"
)

const FireProjectile PacketType = 3

type FireProjectilePacket struct {
	ShooterId  uint32
	PositionX  int32
	PositionY  int32
	DirectionX int8
	DirectionY int8
}

func (p *FireProjectilePacket) ToBytes() []byte {
	buf := GenPacket(FireProjectile, 13)
	offset := len(buf)

	binary.BigEndian.PutUint32(buf[offset+0:offset+4], p.ShooterId)
	binary.BigEndian.PutUint32(buf[offset+4:offset+8], uint32(p.PositionX))
	binary.BigEndian.PutUint32(buf[offset+8:offset+12], uint32(p.PositionY))

	var directionByte byte = 0
	// Encode X direction
	if p.DirectionX < 0 {
		directionByte |= 0b11
	} else if p.DirectionX > 0 {
		directionByte |= 0b01
	}
	directionByte = directionByte << 2 // Make room for Y direction
	// Encode Y direction
	if p.DirectionY < 0 {
		directionByte |= 0b11
	} else if p.DirectionY > 0 {
		directionByte |= 0b01
	}

	// Pad remaining 4 bits with 0s
	buf[offset+12] = directionByte << 4

	return buf
}

func ParseFireProjectilePacket(p *RawPacket) (*FireProjectilePacket, error) {
	if len(p.Body) < 13 {
		return nil, fmt.Errorf("packet body too short - requires 13 bytes, needs to conform to [4b shooterId] [4b positionX] [4b positionY] [1b direction] - received %d bytes", len(p.Body))
	}
	if p.PacketType != Move {
		return nil, ErrInvalidPacketType
	}
	playerId := binary.BigEndian.Uint32(p.Body[0:4])
	posX := int32(binary.BigEndian.Uint32(p.Body[4:8]))
	posY := int32(binary.BigEndian.Uint32(p.Body[8:12]))

	direction := (p.Body[12])
	directionX := int8(direction&0b1100_0000) >> 6 // First 2 bits are X
	directionXSign := direction & 0b10
	directionX = directionX & 0b1
	if directionXSign == 0b10 {
		directionX = -directionX
	}
	directionY := int8(direction&0b0011_0000) >> 4 // Next 2 bits are Y
	directionYSign := direction & 0b10
	directionY = directionY & 0b1
	if directionYSign == 0b10 {
		directionY = -directionY
	}

	return &FireProjectilePacket{
		ShooterId:  playerId,
		PositionX:  posX,
		PositionY:  posY,
		DirectionX: directionX,
		DirectionY: directionY,
	}, nil
}
