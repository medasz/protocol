package shell

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const DefaultBufferSize = 64

type Executor interface {
	ReadOutput(context.Context) ([]byte, error)
	Execute([]byte) error
	Close() error
}

type CmdShell struct {
	stdin         io.WriteCloser
	stdout        io.ReadCloser
	commandWriter io.Writer
	outChan       chan []byte
}

func NewCmdShell(bufferSize int) (*CmdShell, error) {
	cmd := exec.Command("cmd.exe")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout

	stdin, err := cmd.StdinPipe()
	if err != nil {
		stdout.Close()
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, err
	}

	sh := &CmdShell{
		stdin:         stdin,
		stdout:        stdout,
		commandWriter: newCmdInputWriter(stdin),
		outChan:       make(chan []byte, 100),
	}
	go pumpOutput(newCmdOutputReader(stdout), sh.outChan, bufferSize)
	return sh, nil
}

func (s *CmdShell) ReadOutput(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case data, ok := <-s.outChan:
		if !ok {
			return nil, io.EOF
		}
		return bytes.Clone(data), nil
	default:
		return nil, nil
	}
}

func (s *CmdShell) Execute(command []byte) error {
	if len(command) == 0 {
		return nil
	}
	if _, err := s.commandWriter.Write(command); err != nil {
		return err
	}
	_, err := s.commandWriter.Write([]byte("\r\n"))
	return err
}

func (s *CmdShell) Close() error {
	var errs []error
	if s.stdin != nil {
		errs = append(errs, s.stdin.Close())
	}
	if s.stdout != nil {
		errs = append(errs, s.stdout.Close())
	}
	return errors.Join(errs...)
}

func pumpOutput(r io.Reader, out chan<- []byte, bufferSize int) {
	if bufferSize <= 0 {
		bufferSize = DefaultBufferSize
	}
	buf := make([]byte, bufferSize)
	var pending []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			pending = flushUTF8Chunks(out, pending, bufferSize, false)
		}
		if err != nil {
			if len(pending) > 0 {
				flushUTF8Chunks(out, pending, bufferSize, true)
			}
			close(out)
			return
		}
	}
}

func newCmdOutputReader(r io.Reader) io.Reader {
	return transform.NewReader(r, simplifiedchinese.GB18030.NewDecoder())
}

func newCmdInputWriter(w io.Writer) io.Writer {
	return transform.NewWriter(w, simplifiedchinese.GB18030.NewEncoder())
}

func flushUTF8Chunks(out chan<- []byte, pending []byte, chunkSize int, flushAll bool) []byte {
	for len(pending) > 0 {
		limit := chunkSize
		if limit > len(pending) {
			limit = len(pending)
		}

		sendLen := lastValidUTF8Prefix(pending[:limit])
		if sendLen == 0 {
			if flushAll && utf8.Valid(pending) {
				out <- bytes.Clone(pending)
				return nil
			}
			break
		}

		out <- bytes.Clone(pending[:sendLen])
		pending = pending[sendLen:]
		if !flushAll && len(pending) < chunkSize && !utf8.FullRune(pending) {
			break
		}
	}
	return pending
}

func lastValidUTF8Prefix(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	if utf8.Valid(data) {
		return len(data)
	}
	for i := len(data) - 1; i > 0; i-- {
		if utf8.Valid(data[:i]) {
			return i
		}
	}
	return 0
}

