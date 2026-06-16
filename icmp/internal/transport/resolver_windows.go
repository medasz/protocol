package transport

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/google/gopacket/pcap"
)

type OSResolver struct{}

var (
	dialUDP       = net.Dial
	interfaceList = net.Interfaces
	deviceList    = pcap.FindAllDevs
	routeOutput   = func() ([]byte, error) {
		return execCommand("cmd", "/c", "route print 0.0.0.0")
	}
	arpOutput = func() ([]byte, error) {
		return execCommand("arp", "-a")
	}
)

func (OSResolver) ResolveSourceIP(dstIP string) (net.IP, error) {
	conn, err := dialUDP("udp", dstIP+":80")
	if err != nil {
		return nil, fmt.Errorf("获取源IP失败: %v", err)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP, nil
}

func (OSResolver) ResolveSourceMAC(srcIP net.IP) (net.HardwareAddr, error) {
	ifaces, err := interfaceList()
	if err != nil {
		return nil, err
	}
	mac, err := FindSourceMAC(srcIP, ifaces)
	if err != nil {
		return nil, fmt.Errorf("无法找到源 IP %s 对应的 MAC 地址", srcIP)
	}
	return mac, nil
}

func (OSResolver) ResolveDeviceByIP(srcIP net.IP) (string, error) {
	ifs, err := deviceList()
	if err != nil {
		return "", err
	}
	device, err := FindDeviceByIP(ifs, srcIP)
	if err != nil {
		return "", fmt.Errorf("无法找到源 IP %s 对应的抓包设备", srcIP)
	}
	return device, nil
}

func (OSResolver) ResolveNextHopMAC(srcIP, dstIP net.IP) (net.HardwareAddr, error) {
	targetIP := dstIP.String()
	ifaces, err := interfaceList()
	if err != nil {
		return nil, err
	}
	if !IsLocalDestination(srcIP, dstIP, ifaces) {
		gw, err := readDefaultGateway()
		if err != nil {
			return nil, err
		}
		targetIP = gw
	}
	if conn, err := dialUDP("udp", targetIP+":53"); err == nil {
		conn.Close()
	}
	return readARPTable(targetIP)
}

func readDefaultGateway() (string, error) {
	out, err := routeOutput()
	if err != nil {
		return "", err
	}
	return ParseDefaultGateway(string(out))
}

func readARPTable(targetIP string) (net.HardwareAddr, error) {
	out, err := arpOutput()
	if err != nil {
		return nil, err
	}
	return ParseARPTable(strings.ReplaceAll(string(out), "\r\n", "\n"), targetIP)
}

func execCommand(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}
