package transport

import (
	"context"
	"net"

	"protocol/icmp/internal/protocol"
)

type MasterResponder interface {
	Serve(context.Context, func(context.Context, protocol.RequestContext) ([]byte, error)) error
	SendAsync(req protocol.RequestContext, payload []byte) error
}

type PollClient interface {
	Exchange(context.Context, []byte) ([]byte, error)
}

type AddressResolver interface {
	ResolveSourceIP(dstIP string) (net.IP, error)
	ResolveSourceMAC(srcIP net.IP) (net.HardwareAddr, error)
	ResolveDeviceByIP(srcIP net.IP) (string, error)
	ResolveNextHopMAC(srcIP, dstIP net.IP) (net.HardwareAddr, error)
}
