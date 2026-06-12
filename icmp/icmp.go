package main

import (
	"errors"
	"flag"
	"fmt"
	"net"

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
		fmt.Printf("ICMP Layer: %+v\n", icmpLayer)
	}
}
