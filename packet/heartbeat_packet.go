/**
 * Packet: Heartbeat - To indicate to the server that the client is still alive.
 * Requires an update minimally every 5 seconds.
 * Used for client -> server
 *
 * Format: [1b version=0] [1b packetType=0] [1b bodySize=0]
 * Example:
 * 0x00 0x00 0x00 (Head)
 **/

package packet

const Heartbeat PacketType = 0

type HeartbeatPacket struct {
}

func (p *HeartbeatPacket) ToBytes() []byte {
	buf := GenPacket(Heartbeat, 0)
	return buf
}

func ParseHeartbeatPacket(p *RawPacket) (*HeartbeatPacket, error) {
	if p.PacketType != Move {
		return nil, ErrInvalidPacketType
	}

	return &HeartbeatPacket{}, nil
}
