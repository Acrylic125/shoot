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
