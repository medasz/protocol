package app

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

type fakePollClient struct {
	reply []byte
	err   error
	sent  [][]byte
}

func (f *fakePollClient) Exchange(_ context.Context, payload []byte) ([]byte, error) {
	f.sent = append(f.sent, append([]byte(nil), payload...))
	return append([]byte(nil), f.reply...), f.err
}

type fakeExecutor struct {
	outputs  [][]byte
	executed [][]byte
	index    int
}

func (f *fakeExecutor) ReadOutput(context.Context) ([]byte, error) {
	if f.index >= len(f.outputs) {
		return nil, nil
	}
	out := append([]byte(nil), f.outputs[f.index]...)
	f.index++
	return out, nil
}

func (f *fakeExecutor) Execute(command []byte) error {
	f.executed = append(f.executed, append([]byte(nil), command...))
	return nil
}

func (f *fakeExecutor) Close() error { return nil }

func TestSlaveServiceRunTestMode(t *testing.T) {
	var log bytes.Buffer
	client := &fakePollClient{reply: []byte("\x01whoami")}
	executor := &fakeExecutor{outputs: [][]byte{[]byte("hostname")}}
	service := SlaveService{
		Config: SlaveConfig{
			Delay:    time.Millisecond,
			TestMode: true,
			Logger:   &log,
		},
		Client:   client,
		Executor: executor,
	}

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(client.sent) != 1 || string(client.sent[0]) != "\x01hostname" {
		t.Fatalf("unexpected sent payloads: %#v", client.sent)
	}
	if len(executor.executed) != 1 || string(executor.executed[0]) != "whoami" {
		t.Fatalf("unexpected executed commands: %#v", executor.executed)
	}
	if log.Len() == 0 {
		t.Fatal("expected log output")
	}
}

func TestSlaveServiceRunMissingDependencies(t *testing.T) {
	if err := (SlaveService{}).Run(context.Background()); err == nil {
		t.Fatal("expected executor error")
	}
	if err := (SlaveService{Executor: &fakeExecutor{}}).Run(context.Background()); err == nil {
		t.Fatal("expected client error")
	}
}

func TestSleepContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepContext(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("sleepContext() error = %v", err)
	}
}
