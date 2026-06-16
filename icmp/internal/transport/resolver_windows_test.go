package transport

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/google/gopacket/pcap"
)

type fakeConn struct {
	local net.Addr
}

func (f fakeConn) Read([]byte) (int, error)           { return 0, nil }
func (f fakeConn) Write([]byte) (int, error)          { return 0, nil }
func (f fakeConn) Close() error                       { return nil }
func (f fakeConn) LocalAddr() net.Addr                { return f.local }
func (f fakeConn) RemoteAddr() net.Addr               { return &net.UDPAddr{} }
func (f fakeConn) SetDeadline(t time.Time) error      { return nil }
func (f fakeConn) SetReadDeadline(t time.Time) error  { return nil }
func (f fakeConn) SetWriteDeadline(t time.Time) error { return nil }

func TestOSResolverResolveSourceIP(t *testing.T) {
	restoreDial := dialUDP
	defer func() { dialUDP = restoreDial }()

	dialUDP = func(network, address string) (net.Conn, error) {
		return fakeConn{local: &net.UDPAddr{IP: net.ParseIP("10.0.0.1").To4()}}, nil
	}

	ip, err := (OSResolver{}).ResolveSourceIP("10.0.0.2")
	if err != nil {
		t.Fatalf("ResolveSourceIP() error = %v", err)
	}
	if got, want := ip.String(), "10.0.0.1"; got != want {
		t.Fatalf("ip = %q, want %q", got, want)
	}
}

func TestOSResolverResolveSourceMAC(t *testing.T) {
	restoreInterfaces := interfaceList
	defer func() { interfaceList = restoreInterfaces }()

	interfaceList = func() ([]net.Interface, error) {
		return nil, errors.New("not available")
	}

	if _, err := (OSResolver{}).ResolveSourceMAC(net.ParseIP("10.0.0.1")); err == nil {
		t.Fatal("expected error")
	}
}

func TestOSResolverResolveDeviceByIP(t *testing.T) {
	restoreDevices := deviceList
	defer func() { deviceList = restoreDevices }()

	deviceList = func() ([]pcap.Interface, error) {
		return []pcap.Interface{{
			Name: "eth0",
			Addresses: []pcap.InterfaceAddress{
				{IP: net.ParseIP("10.0.0.1").To4()},
			},
		}}, nil
	}

	device, err := (OSResolver{}).ResolveDeviceByIP(net.ParseIP("10.0.0.1").To4())
	if err != nil {
		t.Fatalf("ResolveDeviceByIP() error = %v", err)
	}
	if device != "eth0" {
		t.Fatalf("device = %q, want %q", device, "eth0")
	}
}

func TestOSResolverResolveNextHopMAC(t *testing.T) {
	restoreInterfaces := interfaceList
	restoreRoute := routeOutput
	restoreARP := arpOutput
	restoreDial := dialUDP
	defer func() {
		interfaceList = restoreInterfaces
		routeOutput = restoreRoute
		arpOutput = restoreARP
		dialUDP = restoreDial
	}()

	interfaceList = func() ([]net.Interface, error) {
		return nil, errors.New("interfaces unavailable")
	}
	if _, err := (OSResolver{}).ResolveNextHopMAC(net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.2")); err == nil {
		t.Fatal("expected error")
	}

	interfaceList = func() ([]net.Interface, error) { return []net.Interface{}, nil }
	routeOutput = func() ([]byte, error) {
		return []byte("0.0.0.0          0.0.0.0      192.168.1.1      192.168.1.100"), nil
	}
	arpOutput = func() ([]byte, error) {
		return []byte("192.168.1.1           00-11-22-33-44-55     dynamic"), nil
	}
	dialUDP = func(network, address string) (net.Conn, error) {
		return fakeConn{local: &net.UDPAddr{IP: net.ParseIP("10.0.0.1").To4()}}, nil
	}

	mac, err := (OSResolver{}).ResolveNextHopMAC(net.ParseIP("10.0.0.1").To4(), net.ParseIP("8.8.8.8").To4())
	if err != nil {
		t.Fatalf("ResolveNextHopMAC() error = %v", err)
	}
	if got, want := mac.String(), "00:11:22:33:44:55"; got != want {
		t.Fatalf("mac = %q, want %q", got, want)
	}
}
