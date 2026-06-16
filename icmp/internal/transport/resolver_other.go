//go:build !windows

package transport

import (
	"fmt"
	"net"
	"runtime"
)

type OSResolver struct{}

func unsupportedPlatformError() error {
	return fmt.Errorf("transport.OSResolver is not implemented on %s", runtime.GOOS)
}

func (OSResolver) ResolveSourceIP(dstIP string) (net.IP, error) {
	return nil, unsupportedPlatformError()
}

func (OSResolver) ResolveSourceMAC(srcIP net.IP) (net.HardwareAddr, error) {
	return nil, unsupportedPlatformError()
}

func (OSResolver) ResolveDeviceByIP(srcIP net.IP) (string, error) {
	return "", unsupportedPlatformError()
}

func (OSResolver) ResolveNextHopMAC(srcIP, dstIP net.IP) (net.HardwareAddr, error) {
	return nil, unsupportedPlatformError()
}
