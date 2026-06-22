package app

import (
	"context"
	"net"
	"testing"

	"protocol/icmp/internal/protocol"
)

type fakeMasterResponder struct {
	request protocol.RequestContext
	reply   []byte
	err     error
}

func (f fakeMasterResponder) Serve(ctx context.Context, handler func(context.Context, protocol.RequestContext) ([]byte, error)) error {
	reply, err := handler(ctx, f.request)
	if err != nil {
		return err
	}
	if string(reply) != string(f.reply) {
		return nil
	}
	return f.err
}

type fakeCommandSource struct {
	dataByAgent map[string][]byte
	err         error
}

func (f fakeCommandSource) NextCommand(_ context.Context, agentIP string) ([]byte, error) {
	return f.dataByAgent[agentIP], f.err
}

type fakeResultSink struct {
	gotByAgent map[string][]byte
	err        error
}

func (f *fakeResultSink) WriteResult(agentIP string, data []byte) error {
	if f.gotByAgent == nil {
		f.gotByAgent = make(map[string][]byte)
	}
	f.gotByAgent[agentIP] = append([]byte(nil), data...)
	return f.err
}

func TestMasterServiceRun(t *testing.T) {
	sink := &fakeResultSink{}
	service := MasterService{
		Responder: fakeMasterResponder{
			request: protocol.RequestContext{
				Meta:     protocol.PacketMeta{SrcIP: mustIP(t, "10.0.0.2")},
				Exchange: protocol.Exchange{Payload: append([]byte{protocol.ProtocolShell}, []byte("result")...)},
			},
			reply: append([]byte{protocol.ProtocolShell}, []byte("whoami")...),
		},
		Commands: fakeCommandSource{dataByAgent: map[string][]byte{"10.0.0.2": []byte("whoami")}},
		Results:  sink,
	}

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := string(sink.gotByAgent["10.0.0.2"]), "result"; got != want {
		t.Fatalf("sink payload = %q, want %q", got, want)
	}
}

func TestMasterServiceRoutesCommandsAndResultsByAgentIP(t *testing.T) {
	sink := &fakeResultSink{}
	commands := fakeCommandSource{dataByAgent: map[string][]byte{
		"10.0.0.2": []byte("whoami"),
		"10.0.0.3": []byte("hostname"),
	}}

	for _, tc := range []struct {
		agentIP string
		result  string
		reply   string
	}{
		{agentIP: "10.0.0.2", result: "alice", reply: "whoami"},
		{agentIP: "10.0.0.3", result: "box-3", reply: "hostname"},
	} {
		service := MasterService{
			Responder: fakeMasterResponder{
				request: protocol.RequestContext{
					Meta:     protocol.PacketMeta{SrcIP: mustIP(t, tc.agentIP)},
					Exchange: protocol.Exchange{Payload: append([]byte{protocol.ProtocolShell}, []byte(tc.result)...)},
				},
				reply: append([]byte{protocol.ProtocolShell}, []byte(tc.reply)...),
			},
			Commands: commands,
			Results:  sink,
		}
		if err := service.Run(context.Background()); err != nil {
			t.Fatalf("Run(%s) error = %v", tc.agentIP, err)
		}
		if got := string(sink.gotByAgent[tc.agentIP]); got != tc.result {
			t.Fatalf("sink payload for %s = %q, want %q", tc.agentIP, got, tc.result)
		}
	}
}

func TestMasterServiceRunWithoutCommandSource(t *testing.T) {
	service := MasterService{
		Responder: fakeMasterResponder{
			request: protocol.RequestContext{
				Meta:     protocol.PacketMeta{SrcIP: mustIP(t, "10.0.0.2")},
				Exchange: protocol.Exchange{Payload: append([]byte{protocol.ProtocolShell}, []byte("result")...)},
			},
			reply: []byte{protocol.ProtocolShell},
		},
		Results: &fakeResultSink{},
	}

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func mustIP(t *testing.T, value string) net.IP {
	t.Helper()
	ip := net.ParseIP(value).To4()
	if ip == nil {
		t.Fatalf("ParseIP(%q) returned nil", value)
	}
	return ip
}
