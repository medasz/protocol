package protocol

import (
	"errors"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

var (
	ErrMissingEthernet = errors.New("missing ethernet layer")
	ErrMissingIPv4     = errors.New("missing ipv4 layer")
	ErrMissingICMPv4   = errors.New("missing icmpv4 layer")
	ErrInvalidLayer    = errors.New("invalid packet layer type")
)

type PacketMeta struct {
	SrcMAC net.HardwareAddr
	DstMAC net.HardwareAddr
	SrcIP  net.IP
	DstIP  net.IP
}

type RequestContext struct {
	Meta     PacketMeta
	Exchange Exchange
}

func ParseEchoRequest(packet gopacket.Packet) (RequestContext, error) {
	ethLayer, ipLayer, icmpLayer, err := extractLayers(packet)
	if err != nil {
		return RequestContext{}, err
	}

	return RequestContext{
		Meta: PacketMeta{
			SrcMAC: cloneHardwareAddr(ethLayer.SrcMAC),
			DstMAC: cloneHardwareAddr(ethLayer.DstMAC),
			SrcIP:  cloneIP(ipLayer.SrcIP),
			DstIP:  cloneIP(ipLayer.DstIP),
		},
		Exchange: Exchange{
			ID:      icmpLayer.Id,
			Seq:     icmpLayer.Seq,
			Payload: ClonePayload(icmpLayer.Payload),
		},
	}, nil
}

func ParseEchoReply(data []byte) (Exchange, error) {
	packet := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default)
	_, _, icmpLayer, err := extractLayers(packet)
	if err != nil {
		return Exchange{}, err
	}
	return Exchange{
		ID:      icmpLayer.Id,
		Seq:     icmpLayer.Seq,
		Payload: ClonePayload(icmpLayer.Payload),
	}, nil
}

func BuildEchoReply(req RequestContext, payload []byte) ([]byte, error) {
	replyMeta := PacketMeta{
		SrcMAC: cloneHardwareAddr(req.Meta.DstMAC),
		DstMAC: cloneHardwareAddr(req.Meta.SrcMAC),
		SrcIP:  cloneIP(req.Meta.DstIP),
		DstIP:  cloneIP(req.Meta.SrcIP),
	}
	reply := Exchange{
		ID:      req.Exchange.ID,
		Seq:     req.Exchange.Seq,
		Payload: ClonePayload(payload),
	}
	return buildPacket(replyMeta, layers.ICMPv4TypeEchoReply, reply)
}

func BuildEchoRequest(meta PacketMeta, msg Exchange) ([]byte, error) {
	return buildPacket(meta, layers.CreateICMPv4TypeCode(layers.ICMPv4TypeEchoRequest, 0), Exchange{
		ID:      msg.ID,
		Seq:     msg.Seq,
		Payload: ClonePayload(msg.Payload),
	})
}

func buildPacket(meta PacketMeta, typeCode layers.ICMPv4TypeCode, msg Exchange) ([]byte, error) {
	buffer := gopacket.NewSerializeBuffer()
	options := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}

	eth := &layers.Ethernet{
		SrcMAC:       cloneHardwareAddr(meta.SrcMAC),
		DstMAC:       cloneHardwareAddr(meta.DstMAC),
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		SrcIP:    cloneIP(meta.SrcIP),
		DstIP:    cloneIP(meta.DstIP),
		Protocol: layers.IPProtocolICMPv4,
		Version:  4,
		TTL:      64,
	}
	icmp := &layers.ICMPv4{
		TypeCode: typeCode,
		Id:       msg.ID,
		Seq:      msg.Seq,
	}

	if err := gopacket.SerializeLayers(buffer, options, eth, ip, icmp, gopacket.Payload(msg.Payload)); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func extractLayers(packet gopacket.Packet) (*layers.Ethernet, *layers.IPv4, *layers.ICMPv4, error) {
	ethPacket := packet.Layer(layers.LayerTypeEthernet)
	if ethPacket == nil {
		return nil, nil, nil, ErrMissingEthernet
	}
	ethLayer, ok := ethPacket.(*layers.Ethernet)
	if !ok {
		return nil, nil, nil, ErrInvalidLayer
	}

	ipPacket := packet.Layer(layers.LayerTypeIPv4)
	if ipPacket == nil {
		return nil, nil, nil, ErrMissingIPv4
	}
	ipLayer, ok := ipPacket.(*layers.IPv4)
	if !ok {
		return nil, nil, nil, ErrInvalidLayer
	}

	icmpPacket := packet.Layer(layers.LayerTypeICMPv4)
	if icmpPacket == nil {
		return nil, nil, nil, ErrMissingICMPv4
	}
	icmpLayer, ok := icmpPacket.(*layers.ICMPv4)
	if !ok {
		return nil, nil, nil, ErrInvalidLayer
	}

	return ethLayer, ipLayer, icmpLayer, nil
}

func cloneHardwareAddr(addr net.HardwareAddr) net.HardwareAddr {
	if len(addr) == 0 {
		return nil
	}
	cloned := make(net.HardwareAddr, len(addr))
	copy(cloned, addr)
	return cloned
}

func cloneIP(ip net.IP) net.IP {
	if len(ip) == 0 {
		return nil
	}
	cloned := make(net.IP, len(ip))
	copy(cloned, ip)
	return cloned
}
