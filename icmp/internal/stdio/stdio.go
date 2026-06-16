package stdio

import (
	"bufio"
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
		return cloneBytes(data), nil
	default:
		return nil, nil
	}
}

func (s *NonBlockingCommandSource) scan(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		s.ch <- cloneBytes(scanner.Bytes())
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

func cloneBytes(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	cloned := make([]byte, len(data))
	copy(cloned, data)
	return cloned
}
