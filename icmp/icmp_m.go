//go:build master

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

var (
	src string
	dst string
)

func init() {
	flag.StringVar(&src, "src", "", "Source IP address")
	flag.StringVar(&dst, "dst", "", "Destination IP address")
	flag.Parse()
}

func getIf(ifs []pcap.Interface) (pcap.Interface, error) {
	srcIp := net.ParseIP(src).To4()
	if srcIp == nil {
		return pcap.Interface{}, errors.New("invalid ip")
	}
	for _, iface := range ifs {
		for _, addr := range iface.Addresses {
			if addr.IP.Equal(srcIp) {
				return iface, nil
			}
		}
	}
	return pcap.Interface{}, errors.New("interface not found")
}

func startInputReader(inChan chan []byte) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		copied := make([]byte, len(line))
		copy(copied, line)
		inChan <- copied
	}
}
func main() {
	inChan := make(chan []byte)
	go startInputReader(inChan)
	fmt.Printf("src: %s, dst: %s\n", src, dst)
	ifs, err := pcap.FindAllDevs()
	if err != nil {
		panic(err)
	}
	curIf, err := getIf(ifs)
	if err != nil {
		panic(err)
	}
	hDrive, err := pcap.OpenLive(curIf.Name, 65536, true, pcap.BlockForever)
	if err != nil {
		panic(err)
	}
	defer hDrive.Close()

	filterRule := fmt.Sprintf("icmp and src host %s and dst host %s and icmp[0] == 8", dst, src)
	if err := hDrive.SetBPFFilter(filterRule); err != nil {
		panic(err)
	}
	packetSource := gopacket.NewPacketSource(hDrive, hDrive.LinkType())
	for packet := range packetSource.Packets() {
		printRequestPayload(packet)
		replyBytes, err := createIcmpPacket(packet, inChan)
		if err != nil {
			panic(err)
		}

		err = hDrive.WritePacketData(replyBytes)
		if err != nil {
			panic(err)
		}

	}
}

func printRequestPayload(reqPacket gopacket.Packet) {
	icmpPacket := reqPacket.Layer(layers.LayerTypeICMPv4)
	if icmpPacket == nil {
		return
	}
	icmpLayer, ok := icmpPacket.(*layers.ICMPv4)
	if !ok || len(icmpLayer.Payload) == 0 {
		return
	}
	fmt.Print(string(icmpLayer.Payload))
}

func createIcmpPacket(reqPacket gopacket.Packet, inChan chan []byte) ([]byte, error) {
	ethPacket := reqPacket.Layer(layers.LayerTypeEthernet)
	if ethPacket == nil {
		return nil, errors.New("missing ethernet layer")
	}
	ethLayer, ok := ethPacket.(*layers.Ethernet)
	if !ok {
		return nil, errors.New("invalid ethernet layer")
	}
	ipPacket := reqPacket.Layer(layers.LayerTypeIPv4)
	if ipPacket == nil {
		return nil, errors.New("missing ipv4 layer")
	}
	ipLayer, ok := ipPacket.(*layers.IPv4)
	if !ok {
		return nil, errors.New("invalid ipv4 layer")
	}
	icmpPacket := reqPacket.Layer(layers.LayerTypeICMPv4)
	if icmpPacket == nil {
		return nil, errors.New("missing icmpv4 layer")
	}
	icmpLayer, ok := icmpPacket.(*layers.ICMPv4)
	if !ok {
		return nil, errors.New("invalid icmpv4 layer")
	}
	buffer := gopacket.NewSerializeBuffer()
	options := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	replyEth := &layers.Ethernet{
		SrcMAC:       ethLayer.DstMAC,
		DstMAC:       ethLayer.SrcMAC,
		EthernetType: layers.EthernetTypeIPv4,
	}
	replyIp := &layers.IPv4{
		SrcIP:    ipLayer.DstIP,
		DstIP:    ipLayer.SrcIP,
		Protocol: layers.IPProtocolICMPv4,
		Version:  4,
		TTL:      64,
	}
	replyIcmp := &layers.ICMPv4{
		TypeCode: layers.ICMPv4TypeEchoReply,
		Id:       icmpLayer.Id,
		Seq:      icmpLayer.Seq,
	}
	inputData, err := userInput(inChan)
	if err != nil {
		return nil, err
	}
	customData := gopacket.Payload(inputData)
	err = gopacket.SerializeLayers(buffer, options, replyEth, replyIp, replyIcmp, customData)
	return buffer.Bytes(), err
}

func userInput(inChan chan []byte) ([]byte, error) {
	select {
	case input := <-inChan:
		return input, nil
	default:
		return []byte{}, nil
	}
}
