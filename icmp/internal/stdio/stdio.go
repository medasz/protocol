package stdio

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"sync"
)

type NonBlockingCommandSource struct {
	ch chan []byte
}

func NewNonBlockingCommandSource(r io.Reader) *NonBlockingCommandSource {
	src := &NonBlockingCommandSource{ch: make(chan []byte, 16)}
	go src.scan(r)
	return src
}

func (s *NonBlockingCommandSource) NextCommand(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case data := <-s.ch:
		return bytes.Clone(data), nil
	default:
		return nil, nil
	}
}

func (s *NonBlockingCommandSource) scan(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		s.ch <- bytes.Clone(scanner.Bytes())
	}
}

type WriterResultSink struct {
	mu sync.Mutex
	w  io.Writer
}

func NewWriterResultSink(w io.Writer) *WriterResultSink {
	return &WriterResultSink{w: w}
}

func (s *WriterResultSink) WriteResult(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.w.Write(payload)
	return err
}

