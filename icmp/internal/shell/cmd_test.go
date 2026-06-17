package shell

import (
	"bytes"
	"context"
	"io"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
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
		stdin:         stdin,
		stdout:        nopReadCloser{Reader: bytes.NewReader(nil)},
		commandWriter: stdin,
		outChan:       outChan,
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
		stdin:         &nopWriteCloser{},
		stdout:        nopReadCloser{Reader: bytes.NewReader(nil)},
		commandWriter: &bytes.Buffer{},
		outChan:       make(chan []byte),
	}
	if err := sh.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}


func TestCmdEncodingRoundTripChinese(t *testing.T) {
	rawGBK, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte("中文输出"))
	if err != nil {
		t.Fatalf("encode error = %v", err)
	}

	decoded, err := io.ReadAll(newCmdOutputReader(bytes.NewReader(rawGBK)))
	if err != nil {
		t.Fatalf("decode error = %v", err)
	}
	if got, want := string(decoded), "中文输出"; got != want {
		t.Fatalf("decoded = %q, want %q", got, want)
	}

	var encoded bytes.Buffer
	writer := newCmdInputWriter(&encoded)
	if _, err := writer.Write([]byte("中文命令")); err != nil {
		t.Fatalf("writer.Write() error = %v", err)
	}
	roundTrip, err := simplifiedchinese.GB18030.NewDecoder().Bytes(encoded.Bytes())
	if err != nil {
		t.Fatalf("round trip decode error = %v", err)
	}
	if got, want := string(roundTrip), "中文命令"; got != want {
		t.Fatalf("round trip = %q, want %q", got, want)
	}
}

func TestPumpOutputKeepsUTF8Boundaries(t *testing.T) {
	out := make(chan []byte, 8)
	go pumpOutput(bytes.NewBufferString("中文abc"), out, 4)

	var chunks [][]byte
	for chunk := range out {
		chunks = append(chunks, chunk)
	}

	var merged []byte
	for _, chunk := range chunks {
		if !bytes.Equal(chunk, []byte(string(chunk))) {
			t.Fatalf("chunk is not valid utf-8: %v", chunk)
		}
		merged = append(merged, chunk...)
	}
	if got, want := string(merged), "中文abc"; got != want {
		t.Fatalf("merged = %q, want %q", got, want)
	}
}
