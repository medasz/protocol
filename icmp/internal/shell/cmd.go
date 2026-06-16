package shell

import (
	"context"
	"io"
	"os/exec"
)

const DefaultBufferSize = 64

type Executor interface {
	ReadOutput(context.Context) ([]byte, error)
	Execute([]byte) error
	Close() error
}

type CmdShell struct {
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	outChan chan []byte
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
		stdin:   stdin,
		stdout:  stdout,
		outChan: make(chan []byte, 100),
	}
	go pumpOutput(stdout, sh.outChan, bufferSize)
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
		return cloneBytes(data), nil
	default:
		return nil, nil
	}
}

func (s *CmdShell) Execute(command []byte) error {
	if len(command) == 0 {
		return nil
	}
	if _, err := s.stdin.Write(command); err != nil {
		return err
	}
	_, err := s.stdin.Write([]byte("\r\n"))
	return err
}

func (s *CmdShell) Close() error {
	var firstErr error
	if s.stdin != nil {
		firstErr = s.stdin.Close()
	}
	if s.stdout != nil {
		if err := s.stdout.Close(); firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func pumpOutput(r io.Reader, out chan<- []byte, bufferSize int) {
	if bufferSize <= 0 {
		bufferSize = DefaultBufferSize
	}
	buf := make([]byte, bufferSize)
	for {
		n, err := r.Read(buf)
		if err != nil {
			close(out)
			return
		}
		if n > 0 {
			out <- cloneBytes(buf[:n])
		}
	}
}

func cloneBytes(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	cloned := make([]byte, len(data))
	copy(cloned, data)
	return cloned
}
