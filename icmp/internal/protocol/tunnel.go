package protocol

import (
	"encoding/binary"
	"errors"
)

// Protocol Multiplexing IDs
const (
	ProtocolShell  uint8 = 1
	ProtocolTunnel uint8 = 2
)

// Tunnel packet types
const (
	TunnelTypeSYN  uint8 = 1 // 建立连接
	TunnelTypeDATA uint8 = 2 // 传输数据
	TunnelTypeACK  uint8 = 3 // 纯确认包
	TunnelTypeFIN  uint8 = 4 // 断开连接
)

// TunnelHeaderSize is the total size of the serialized TunnelHeader.
// 4 (SessionID) + 1 (Type) + 4 (Seq) + 4 (Ack) + 2 (Length) + 1 (Padding) = 16 bytes
const TunnelHeaderSize = 16

var ErrBufferTooSmall = errors.New("buffer too small for tunnel header")

// TunnelHeader represents the header of a reliable ICMP tunnel packet.
type TunnelHeader struct {
	SessionID uint32 // 会话 ID
	Type      uint8  // 数据包类型
	Seq       uint32 // 序列号
	Ack       uint32 // 确认号
	Length    uint16 // 实际数据的长度
}

// Marshal serializes the TunnelHeader into a byte slice.
func (h *TunnelHeader) Marshal(b []byte) error {
	if len(b) < TunnelHeaderSize {
		return ErrBufferTooSmall
	}
	binary.BigEndian.PutUint32(b[0:4], h.SessionID)
	b[4] = h.Type
	binary.BigEndian.PutUint32(b[5:9], h.Seq)
	binary.BigEndian.PutUint32(b[9:13], h.Ack)
	binary.BigEndian.PutUint16(b[13:15], h.Length)
	b[15] = 0 // Padding byte for 16-byte alignment
	return nil
}

// UnmarshalTunnelHeader deserializes a byte slice into a TunnelHeader.
func UnmarshalTunnelHeader(b []byte) (TunnelHeader, error) {
	if len(b) < TunnelHeaderSize {
		return TunnelHeader{}, ErrBufferTooSmall
	}
	return TunnelHeader{
		SessionID: binary.BigEndian.Uint32(b[0:4]),
		Type:      b[4],
		Seq:       binary.BigEndian.Uint32(b[5:9]),
		Ack:       binary.BigEndian.Uint32(b[9:13]),
		Length:    binary.BigEndian.Uint16(b[13:15]),
	}, nil
}
