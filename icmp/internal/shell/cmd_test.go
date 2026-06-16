package shell

import (
	"bytes"
	"context"
	"io"
	"testing"
)

type nopWriteCloser struct {
	bytes.Buffer
}

func (n *nopWriteCloser) Close() error { return nil }

type nopReadCloser struct {
	io.Reader
}

func (n nopReadCloser) Close() error { return nil }

func TestCmdShellExecuteAndRead(t *testing.T) {
	stdin := &nopWriteCloser{}
	outChan := make(chan []byte, 1)
	outChan <- []byte("hello")
	close(outChan)

	sh := &CmdShell{
		stdin:   stdin,
		stdout:  nopReadCloser{Reader: bytes.NewReader(nil)},
		outChan: outChan,
	}

	data, err := sh.ReadOutput(context.Background())
	if err != nil {
		t.Fatalf("ReadOutput() error = %v", err)
	}
	if got, want := string(data), "hello"; got != want {
		t.Fatalf("data = %q, want %q", got, want)
	}
	if err := sh.Execute([]byte("whoami")); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := stdin.String(), "whoami\r\n"; got != want {
		t.Fatalf("stdin = %q, want %q", got, want)
	}
}

func TestPumpOutput(t *testing.T) {
	out := make(chan []byte, 2)
	go pumpOutput(bytes.NewBufferString("abcdef"), out, 3)

	got1 := <-out
	got2 := <-out
	if string(got1) != "abc" || string(got2) != "def" {
		t.Fatalf("unexpected chunks: %q %q", got1, got2)
	}
}

func TestReadOutputContextCanceled(t *testing.T) {
	sh := &CmdShell{outChan: make(chan []byte)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := sh.ReadOutput(ctx)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestClose(t *testing.T) {
	sh := &CmdShell{
		stdin:   &nopWriteCloser{},
		stdout:  nopReadCloser{Reader: bytes.NewReader(nil)},
		outChan: make(chan []byte),
	}
	if err := sh.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestCloneBytes(t *testing.T) {
	src := []byte("abc")
	dst := cloneBytes(src)
	src[0] = 'z'
	if string(dst) != "abc" {
		t.Fatalf("cloneBytes() = %q, want %q", dst, "abc")
	}
}
