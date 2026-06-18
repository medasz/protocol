package stdio

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestNonBlockingCommandSource(t *testing.T) {
	source := NewNonBlockingCommandSource(strings.NewReader("whoami\nhostname\n"))

	waitForData(t)
	cmd, err := source.NextCommand(context.Background(), "10.0.0.2")
	if err != nil {
		t.Fatalf("NextCommand() error = %v", err)
	}
	if got, want := string(cmd), "whoami"; got != want {
		t.Fatalf("first command = %q, want %q", got, want)
	}
}

func TestWriterResultSink(t *testing.T) {
	var buf bytes.Buffer
	sink := NewWriterResultSink(&buf)

	if err := sink.WriteResult("10.0.0.2", []byte("ok")); err != nil {
		t.Fatalf("WriteResult() error = %v", err)
	}
	if got, want := buf.String(), "ok"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
}

func waitForData(t *testing.T) {
	t.Helper()
	time.Sleep(20 * time.Millisecond)
}
