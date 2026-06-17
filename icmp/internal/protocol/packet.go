package protocol

import (
	"bytes"
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
			SrcMAC: bytes.Clone(ethLayer.SrcMAC),
			DstMAC: bytes.Clone(ethLayer.DstMAC),
			SrcIP:  bytes.Clone(ipLayer.SrcIP),
			DstIP:  bytes.Clone(ipLayer.DstIP),
		},
		Exchange: Exchange{
			ID:      icmpLayer.Id,
			Seq:     icmpLayer.Seq,
			Payload: bytes.Clone(icmpLayer.Payload),
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
		Payload: bytes.Clone(icmpLayer.Payload),
	}, nil
}

func BuildEchoReply(req RequestContext, payload []byte) ([]byte, error) {
	replyMeta := PacketMeta{
		SrcMAC: bytes.Clone(req.Meta.DstMAC),
		DstMAC: bytes.Clone(req.Meta.SrcMAC),
		SrcIP:  bytes.Clone(req.Meta.DstIP),
		DstIP:  bytes.Clone(req.Meta.SrcIP),
	}
	reply := Exchange{
		ID:      req.Exchange.ID,
		Seq:     req.Exchange.Seq,
		Payload: bytes.Clone(payload),
	}
	return buildPacket(replyMeta, layers.ICMPv4TypeEchoReply, reply)
}

func BuildEchoRequest(meta PacketMeta, msg Exchange) ([]byte, error) {
	return buildPacket(meta, layers.CreateICMPv4TypeCode(layers.ICMPv4TypeEchoRequest, 0), Exchange{
		ID:      msg.ID,
		Seq:     msg.Seq,
		Payload: bytes.Clone(msg.Payload),
	})
}

func buildPacket(meta PacketMeta, typeCode layers.ICMPv4TypeCode, msg Exchange) ([]byte, error) {
	buffer := gopacket.NewSerializeBuffer()
	options := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}

	eth := &layers.Ethernet{
		SrcMAC:       bytes.Clone(meta.SrcMAC),
		DstMAC:       bytes.Clone(meta.DstMAC),
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		SrcIP:    bytes.Clone(meta.SrcIP),
		DstIP:    bytes.Clone(meta.DstIP),
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

func getLayer[T any](packet gopacket.Packet, layerType gopacket.LayerType, errMissing error) (T, error) {
	var zero T
	layer := packet.Layer(layerType)
	if layer == nil {
		return zero, errMissing
	}
	tLayer, ok := layer.(T)
	if !ok {
		return zero, ErrInvalidLayer
	}
	return tLayer, nil
}

func extractLayers(packet gopacket.Packet) (*layers.Ethernet, *layers.IPv4, *layers.ICMPv4, error) {
	ethLayer, err := getLayer[*layers.Ethernet](packet, layers.LayerTypeEthernet, ErrMissingEthernet)
	if err != nil {
		return nil, nil, nil, err
	}

	ipLayer, err := getLayer[*layers.IPv4](packet, layers.LayerTypeIPv4, ErrMissingIPv4)
	if err != nil {
		return nil, nil, nil, err
	}

	icmpLayer, err := getLayer[*layers.ICMPv4](packet, layers.LayerTypeICMPv4, ErrMissingICMPv4)
	if err != nil {
		return nil, nil, nil, err
	}

	return ethLayer, ipLayer, icmpLayer, nil
}
