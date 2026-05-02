package aidebug

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/koreaf16/argus/internal/query"
)

type Sink struct {
	mu        sync.Mutex
	out       io.Writer
	file      *os.File
	ch        chan query.TraceRecord
	done      chan struct{}
	closeOnce sync.Once
	dropped   atomic.Int64
	lastDrop  atomic.Int64
}

func NewSink(out io.Writer, filePath string) (*Sink, error) {
	if out == nil {
		out = io.Discard
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	s := &Sink{
		out:  out,
		file: f,
		ch:   make(chan query.TraceRecord, 1000),
		done: make(chan struct{}),
	}
	go s.process()
	return s, nil
}

func (s *Sink) process() {
	defer close(s.done)
	for record := range s.ch {
		s.writeRecord(record)
	}
}

func (s *Sink) writeRecord(record query.TraceRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	line, err := json.Marshal(record)
	if err != nil {
		return
	}
	line = append(line, '\n')
	_, _ = s.out.Write(line)
	if s.file != nil {
		_, _ = s.file.Write(line)
	}
}

func (s *Sink) Emit(record query.TraceRecord) {
	select {
	case s.ch <- record:
	default:
		count := s.dropped.Add(1)
		now := time.Now().UnixMilli()
		last := s.lastDrop.Load()
		// 1초마다 한 번만 sink.dropped 메타 trace를 발행해 drop 사실을 기록합니다.
		if now-last >= 1000 && s.lastDrop.CompareAndSwap(last, now) {
			drop := query.TraceRecord{
				TS:   time.Now().UTC().Format(time.RFC3339Nano),
				Type: "sink.dropped",
				Data: map[string]any{"count": count},
			}
			select {
			case s.ch <- drop:
			default:
			}
		}
	}
}

func (s *Sink) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		close(s.ch)
		<-s.done

		s.mu.Lock()
		defer s.mu.Unlock()
		if s.file != nil {
			closeErr = s.file.Close()
			s.file = nil
		}
	})
	return closeErr
}
