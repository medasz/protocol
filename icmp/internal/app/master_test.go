package app

import (
	"context"
	"testing"

	"protocol/icmp/internal/protocol"
)

type fakeMasterResponder struct {
	request protocol.Exchange
	reply   []byte
	err     error
}

func (f fakeMasterResponder) Serve(ctx context.Context, handler func(context.Context, protocol.Exchange) ([]byte, error)) error {
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
	data []byte
	err  error
}

func (f fakeCommandSource) NextCommand(context.Context) ([]byte, error) {
	return f.data, f.err
}

type fakeResultSink struct {
	got []byte
	err error
}

func (f *fakeResultSink) WriteResult(data []byte) error {
	f.got = append([]byte(nil), data...)
	return f.err
}

func TestMasterServiceRun(t *testing.T) {
	sink := &fakeResultSink{}
	service := MasterService{
		Responder: fakeMasterResponder{
			request: protocol.Exchange{Payload: []byte("result")},
			reply:   []byte("whoami"),
		},
		Commands: fakeCommandSource{data: []byte("whoami")},
		Results:  sink,
	}

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := string(sink.got), "result"; got != want {
		t.Fatalf("sink payload = %q, want %q", got, want)
	}
}

func TestMasterServiceRunWithoutCommandSource(t *testing.T) {
	service := MasterService{
		Responder: fakeMasterResponder{
			request: protocol.Exchange{Payload: []byte("result")},
			reply:   nil,
		},
		Results: &fakeResultSink{},
	}

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}
