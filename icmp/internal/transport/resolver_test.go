package transport

import (
	"net"
	"testing"

	"github.com/google/gopacket/pcap"
)

func TestBuildFilters(t *testing.T) {
	if got, want := BuildMasterFilter(nil, "10.0.0.1"), "icmp and dst host 10.0.0.1 and icmp[0] == 8"; got != want {
		t.Fatalf("BuildMasterFilter(nil) = %q, want %q", got, want)
	}
	if got, want := BuildMasterFilter([]string{"10.0.0.2"}, "10.0.0.1"), "icmp and dst host 10.0.0.1 and icmp[0] == 8 and (src host 10.0.0.2)"; got != want {
		t.Fatalf("BuildMasterFilter(single) = %q, want %q", got, want)
	}
	if got, want := BuildMasterFilter([]string{"10.0.0.2", "10.0.0.3"}, "10.0.0.1"), "icmp and dst host 10.0.0.1 and icmp[0] == 8 and (src host 10.0.0.2 or src host 10.0.0.3)"; got != want {
		t.Fatalf("BuildMasterFilter(multiple) = %q, want %q", got, want)
	}
	if got, want := BuildSlaveFilter("10.0.0.2"), "icmp and host 10.0.0.2 and icmp[0] == 0"; got != want {
		t.Fatalf("BuildSlaveFilter() = %q, want %q", got, want)
	}
}

func TestParseDefaultGateway(t *testing.T) {
	out := "Active Routes:\n0.0.0.0          0.0.0.0      192.168.1.1      192.168.1.100"
	got, err := ParseDefaultGateway(out)
	if err != nil {
		t.Fatalf("ParseDefaultGateway() error = %v", err)
	}
	if want := "192.168.1.1"; got != want {
		t.Fatalf("gateway = %q, want %q", got, want)
	}
}

func TestParseARPTable(t *testing.T) {
	out := "Interface: 192.168.1.100 --- 0x6\n  Internet Address      Physical Address      Type\n  192.168.1.1           00-11-22-33-44-55     dynamic"
	got, err := ParseARPTable(out, "192.168.1.1")
	if err != nil {
		t.Fatalf("ParseARPTable() error = %v", err)
	}
	if want := "00:11:22:33:44:55"; got.String() != want {
		t.Fatalf("mac = %q, want %q", got.String(), want)
	}
}

func TestFindDeviceByIP(t *testing.T) {
	device, err := FindDeviceByIP([]pcap.Interface{
		{
			Name: "eth0",
			Addresses: []pcap.InterfaceAddress{
				{IP: net.ParseIP("10.0.0.1").To4()},
			},
		},
	}, net.ParseIP("10.0.0.1").To4())
	if err != nil {
		t.Fatalf("FindDeviceByIP() error = %v", err)
	}
	if device != "eth0" {
		t.Fatalf("device = %q, want %q", device, "eth0")
	}
}
