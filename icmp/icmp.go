package main

import (
	"bufio"
	"encoding/hex"
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

func main() {
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
		icmpLayer := packet.Layer(layers.LayerTypeICMPv4)
		icmp := icmpLayer.(*layers.ICMPv4)
		fmt.Println(icmp.Id)
		fmt.Println(icmp.Seq)
		fmt.Println(hex.Dump(icmp.Payload))
		replyBytes, err := createIcmpPacket(packet)
		if err != nil {
			panic(err)
		}
		fmt.Println("-------------------------1")
		err = hDrive.WritePacketData(replyBytes)
		if err != nil {
			panic(err)
		}
		fmt.Println("-------------------------2")
	}
}

func createIcmpPacket(reqPacket gopacket.Packet) ([]byte, error) {
	ethPacket := reqPacket.Layer(layers.LayerTypeEthernet)
	ethLayer := ethPacket.(*layers.Ethernet)
	ipPacket := reqPacket.Layer(layers.LayerTypeIPv4)
	ipLayer := ipPacket.(*layers.IPv4)
	icmpPacket := reqPacket.Layer(layers.LayerTypeICMPv4)
	icmpLayer := icmpPacket.(*layers.ICMPv4)
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
	inputData, err := userInput()
	if err != nil {
		return nil, err
	}
	customData := gopacket.Payload(inputData)
	err = gopacket.SerializeLayers(buffer, options, replyEth, replyIp, replyIcmp, customData)
	return buffer.Bytes(), err
}

func userInput() ([]byte, error) {
	fmt.Printf("userInput:")
	reader := bufio.NewReader(os.Stdin)
	line, _, err := reader.ReadLine()
	return line, err
}
