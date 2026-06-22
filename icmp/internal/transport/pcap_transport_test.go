package transport

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"protocol/icmp/internal/protocol"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

type fakeResolver struct {
	sourceIP  net.IP
	sourceMAC net.HardwareAddr
	device    string
	nextHop   net.HardwareAddr
	err       error
}

func (f fakeResolver) ResolveSourceIP(string) (net.IP, error)            { return f.sourceIP, f.err }
func (f fakeResolver) ResolveSourceMAC(net.IP) (net.HardwareAddr, error) { return f.sourceMAC, f.err }
func (f fakeResolver) ResolveDeviceByIP(net.IP) (string, error)          { return f.device, f.err }
func (f fakeResolver) ResolveNextHopMAC(net.IP, net.IP) (net.HardwareAddr, error) {
	return f.nextHop, f.err
}

type fakeHandle struct {
	filter      string
	writes      [][]byte
	reads       [][]byte
	readErr     error
	packetsChan chan gopacket.Packet
}

func (f *fakeHandle) SetBPFFilter(filter string) error {
	f.filter = filter
	return nil
}

func (f *fakeHandle) WritePacketData(data []byte) error {
	f.writes = append(f.writes, append([]byte(nil), data...))
	return nil
}

func (f *fakeHandle) ReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	if len(f.reads) == 0 {
		return nil, gopacket.CaptureInfo{}, f.readErr
	}
	data := f.reads[0]
	f.reads = f.reads[1:]
	return append([]byte(nil), data...), gopacket.CaptureInfo{}, nil
}

func (f *fakeHandle) Close() {}

func (f *fakeHandle) Packets() <-chan gopacket.Packet {
	return f.packetsChan
}

func TestPcapMasterResponderServe(t *testing.T) {
	restore := openLiveHandle
	defer func() { openLiveHandle = restore }()

	fh := &fakeHandle{packetsChan: make(chan gopacket.Packet, 1)}
	openLiveHandle = func(string, int32, bool, time.Duration) (packetHandle, error) {
		return fh, nil
	}

	reqBytes, err := protocol.BuildEchoRequest(protocol.PacketMeta{
		SrcMAC: mustMAC(t, "00:11:22:33:44:55"),
		DstMAC: mustMAC(t, "66:77:88:99:aa:bb"),
		SrcIP:  net.ParseIP("10.0.0.2").To4(),
		DstIP:  net.ParseIP("10.0.0.1").To4(),
	}, protocol.Exchange{ID: 1, Seq: 2, Payload: []byte("result")})
	if err != nil {
		t.Fatalf("BuildEchoRequest() error = %v", err)
	}
	fh.packetsChan <- gopacket.NewPacket(reqBytes, layers.LayerTypeEthernet, gopacket.Default)
	close(fh.packetsChan)

	responder := PcapMasterResponder{
		SrcIP:         "10.0.0.1",
		AllowedDstIPs: []string{"10.0.0.2"},
		Resolver:      fakeResolver{device: "eth0"},
	}
	if err := responder.Serve(context.Background(), func(_ context.Context, req protocol.RequestContext) ([]byte, error) {
		if got, want := req.Meta.SrcIP.String(), "10.0.0.2"; got != want {
			t.Fatalf("request source IP = %q, want %q", got, want)
		}
		return []byte("whoami"), nil
	}); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if got, want := fh.filter, BuildMasterFilter([]string{"10.0.0.2"}, "10.0.0.1"); got != want {
		t.Fatalf("filter = %q, want %q", got, want)
	}
	if len(fh.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(fh.writes))
	}
}

func TestPcapPollClientExchange(t *testing.T) {
	restore := openLiveHandle
	defer func() { openLiveHandle = restore }()

	replyBytes, err := protocol.BuildEchoRequest(protocol.PacketMeta{
		SrcMAC: mustMAC(t, "00:11:22:33:44:55"),
		DstMAC: mustMAC(t, "66:77:88:99:aa:bb"),
		SrcIP:  net.ParseIP("10.0.0.2").To4(),
		DstIP:  net.ParseIP("10.0.0.1").To4(),
	}, protocol.Exchange{ID: 9, Seq: 10, Payload: append([]byte{0x55, protocol.ProtocolShell}, []byte("ok")...)})
	if err != nil {
		t.Fatalf("BuildEchoRequest() error = %v", err)
	}

	fh := &fakeHandle{reads: [][]byte{replyBytes}}
	openLiveHandle = func(string, int32, bool, time.Duration) (packetHandle, error) {
		return fh, nil
	}

	client := PcapPollClient{
		TargetIP: "10.0.0.2",
		Timeout:  50 * time.Millisecond,
		Resolver: fakeResolver{
			sourceIP:  net.ParseIP("10.0.0.1").To4(),
			sourceMAC: mustMAC(t, "66:77:88:99:aa:bb"),
			nextHop:   mustMAC(t, "00:11:22:33:44:55"),
			device:    "eth0",
		},
		ID:  9,
		Seq: 10,
	}

	reply, err := client.Exchange(context.Background(), []byte("hostname"))
	if err != nil {
		t.Fatalf("Exchange() error = %v", err)
	}
	if got, want := string(reply), "\x01ok"; got != want {
		t.Fatalf("reply = %q, want %q", got, want)
	}
	if got, want := fh.filter, BuildSlaveFilter("10.0.0.2"); got != want {
		t.Fatalf("filter = %q, want %q", got, want)
	}
	if len(fh.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(fh.writes))
	}
}

func TestPcapPollClientExchangeTimeout(t *testing.T) {
	restore := openLiveHandle
	defer func() { openLiveHandle = restore }()

	fh := &fakeHandle{readErr: errors.New("no packet")}
	openLiveHandle = func(string, int32, bool, time.Duration) (packetHandle, error) {
		return fh, nil
	}

	client := PcapPollClient{
		TargetIP: "10.0.0.2",
		Timeout:  time.Millisecond,
		Resolver: fakeResolver{
			sourceIP:  net.ParseIP("10.0.0.1").To4(),
			sourceMAC: mustMAC(t, "66:77:88:99:aa:bb"),
			nextHop:   mustMAC(t, "00:11:22:33:44:55"),
			device:    "eth0",
		},
		ID:  1,
		Seq: 1,
	}

	if _, err := client.Exchange(context.Background(), nil); err == nil {
		t.Fatal("expected timeout error")
	}
}

func mustMAC(t *testing.T, value string) net.HardwareAddr {
	t.Helper()
	mac, err := net.ParseMAC(value)
	if err != nil {
		t.Fatalf("ParseMAC(%q) error = %v", value, err)
	}
	return mac
}
