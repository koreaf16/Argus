package aidebug

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/koreaf16/argus/internal/query"
)

// FilterMode는 Sink에 들어온 trace 레코드 중 어떤 종류를 stdout/stderr로 흘릴지
// 결정한다. 세션 파일(file)에는 항상 풀 레코드가 기록되어 사후 분석이 가능하다.
type FilterMode int

const (
	// FilterFull은 모든 레코드를 외부 writer로 전달한다 (기존 --aidebug 동작).
	FilterFull FilterMode = iota
	// FilterMirror는 token-by-token 델타류 레코드를 drop하고, 완성 이벤트와
	// 메타데이터만 전달한다. mirror 모드에서 NDJSON 노이즈를 줄이는 용도.
	FilterMirror
	// FilterDropAll은 외부 writer로 아무것도 보내지 않는다 (세션 파일에만 기록).
	// 기본 mirror 모드에서 stdout이 미러 ANSI 출력으로 사용될 때 NDJSON을 stdout과
	// 분리하기 위해 stderr 출력 자체를 끄고 싶을 때 사용한다.
	FilterDropAll
)

// noisyTraceTypes는 token-by-token 델타류 trace 타입들이다. mirror 모드에서는
// 이들을 외부 writer로 보내지 않는다 (세션 파일 기록은 유지).
var noisyTraceTypes = map[string]struct{}{
	"llm.provider_chunk": {},
	"llm.thinking":       {},
	"tool.call.output":   {},
}

type Sink struct {
	mu        sync.Mutex
	sendMu    sync.RWMutex
	out       io.Writer
	file      *os.File
	mode      FilterMode
	ch        chan query.TraceRecord
	done      chan struct{}
	closeOnce sync.Once
	closed    bool
	dropped   atomic.Int64
	lastDrop  atomic.Int64
}

func NewSink(out io.Writer, filePath string) (*Sink, error) {
	return NewSinkWithMode(out, filePath, FilterFull)
}

// NewSinkWithMode는 외부 writer 출력 정책을 명시해 Sink를 생성한다. 세션 파일은
// mode와 무관하게 항상 풀 NDJSON을 기록한다.
func NewSinkWithMode(out io.Writer, filePath string, mode FilterMode) (*Sink, error) {
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
		mode: mode,
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

	// Data 필드의 string 값에 대해 UTF-8 유효성 검증 — 한국어 등 멀티바이트 깨짐 방지
	sanitizeTraceData(&record)

	line, err := json.Marshal(record)
	if err != nil {
		return
	}
	line = append(line, '\n')
	if s.shouldEmitToWriter(record.Type) {
		_, _ = s.out.Write(line)
	}
	if s.file != nil {
		_, _ = s.file.Write(line)
	}
}

// sanitizeTraceData는 TraceRecord.Data 내부의 문자열 값들이 유효한 UTF-8인지
// 검증하고, 잘못된 바이트 시퀀스를 U+FFFD로 교체한다.
func sanitizeTraceData(record *query.TraceRecord) {
	if data, ok := record.Data.(map[string]any); ok {
		for k, v := range data {
			switch val := v.(type) {
			case string:
				if !utf8.ValidString(val) {
					data[k] = strings.ToValidUTF8(val, "\uFFFD")
				}
			case map[string]any:
				sanitizeMapUTF8(val)
			}
		}
	}
}

func sanitizeMapUTF8(m map[string]any) {
	for k, v := range m {
		switch val := v.(type) {
		case string:
			if !utf8.ValidString(val) {
				m[k] = strings.ToValidUTF8(val, "\uFFFD")
			}
		case map[string]any:
			sanitizeMapUTF8(val)
		}
	}
}

// shouldEmitToWriter는 외부 writer로 레코드를 보낼지 결정한다. 세션 파일 기록과
// 무관하게 stdout/stderr 노이즈만 제어한다.
func (s *Sink) shouldEmitToWriter(traceType string) bool {
	switch s.mode {
	case FilterDropAll:
		return false
	case FilterMirror:
		_, noisy := noisyTraceTypes[traceType]
		return !noisy
	default:
		return true
	}
}

func (s *Sink) Emit(record query.TraceRecord) {
	if s == nil {
		return
	}
	s.sendMu.RLock()
	defer s.sendMu.RUnlock()
	if s.closed {
		return
	}
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
		s.sendMu.Lock()
		s.closed = true
		close(s.ch)
		s.sendMu.Unlock()
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
