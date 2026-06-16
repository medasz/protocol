//go:build !windows

package transport

import (
	"net"
	"strings"
	"testing"
)

func TestOSResolverUnsupportedPlatform(t *testing.T) {
	resolver := OSResolver{}

	if _, err := resolver.ResolveSourceIP("1.1.1.1"); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("ResolveSourceIP() error = %v", err)
	}
	if _, err := resolver.ResolveSourceMAC(net.ParseIP("127.0.0.1")); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("ResolveSourceMAC() error = %v", err)
	}
	if _, err := resolver.ResolveDeviceByIP(net.ParseIP("127.0.0.1")); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("ResolveDeviceByIP() error = %v", err)
	}
	if _, err := resolver.ResolveNextHopMAC(net.ParseIP("127.0.0.1"), net.ParseIP("8.8.8.8")); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("ResolveNextHopMAC() error = %v", err)
	}
}
