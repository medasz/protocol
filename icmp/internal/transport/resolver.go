package transport

import (
	"errors"
	"net"
	"strings"

	"github.com/google/gopacket/pcap"
)

func BuildMasterFilter(srcHost, dstHost string) string {
	return "icmp and src host " + srcHost + " and dst host " + dstHost + " and icmp[0] == 8"
}

func BuildSlaveFilter(host string) string {
	return "icmp and host " + host + " and icmp[0] == 0"
}

func FindDeviceByIP(ifs []pcap.Interface, srcIP net.IP) (string, error) {
	for _, iface := range ifs {
		for _, addr := range iface.Addresses {
			if addr.IP.Equal(srcIP) {
				return iface.Name, nil
			}
		}
	}
	return "", errors.New("interface not found")
}

func ParseDefaultGateway(routeOutput string) (string, error) {
	lines := strings.Split(routeOutput, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "0.0.0.0" {
			return fields[2], nil
		}
	}
	return "", errors.New("gateway not found")
}

func ParseARPTable(arpOutput, targetIP string) (net.HardwareAddr, error) {
	lines := strings.Split(arpOutput, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != targetIP {
			continue
		}
		mac, err := net.ParseMAC(strings.ReplaceAll(fields[1], "-", ":"))
		if err == nil {
			return mac, nil
		}
	}
	return nil, errors.New("mac not found in arp cache for " + targetIP)
}

func FindSourceMAC(srcIP net.IP, ifaces []net.Interface) (net.HardwareAddr, error) {
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.Equal(srcIP) {
				return cloneHardwareAddr(iface.HardwareAddr), nil
			}
		}
	}
	return nil, errors.New("source mac not found")
}

func IsLocalDestination(srcIP, dstIP net.IP, ifaces []net.Interface) bool {
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipnet.IP.Equal(srcIP) && ipnet.Contains(dstIP) {
					return true
				}
			}
		}
	}
	return false
}

func cloneHardwareAddr(addr net.HardwareAddr) net.HardwareAddr {
	if len(addr) == 0 {
		return nil
	}
	cloned := make(net.HardwareAddr, len(addr))
	copy(cloned, addr)
	return cloned
}
