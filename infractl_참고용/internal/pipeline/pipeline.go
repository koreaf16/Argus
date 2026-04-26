// Package pipeline
// File: pipeline.go
// Description: 명령 출력 스트리밍 파이프라인 오케스트레이터
// Responsibility: io.Reader를 라인 단위로 읽고 ProcessorFunc 체인을 적용하여 결과를 반환

package pipeline

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"strings"
	"time"
)

// ProgressInfo는 파이프라인 처리 진행 상황이다.
type ProgressInfo struct {
	LinesRead   int
	BytesRead   int64
	ElapsedTime time.Duration
	Done        bool
}

// OnProgress는 진행 상황 콜백 함수 타입이다.
type OnProgress func(ProgressInfo)

// Option은 Pipeline 생성 옵션 함수이다.
type Option func(*Pipeline)

// Pipeline은 io.Reader를 라인 단위로 읽고 전처리기 체인을 적용하는 스트리밍 파이프라인이다.
type Pipeline struct {
	maxBytes    int
	processors  []ProcessorFunc
	onProgress  OnProgress
	progressInt time.Duration // 프로그레스 콜백 최소 간격
}

// New는 지정된 옵션으로 Pipeline을 생성한다.
func New(opts ...Option) *Pipeline {
	p := &Pipeline{
		progressInt: 500 * time.Millisecond,
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// WithMaxBytes는 파이프라인 출력 최대 바이트 수를 설정한다.
func WithMaxBytes(n int) Option {
	return func(p *Pipeline) { p.maxBytes = n }
}

// WithProcessor는 출력 라인에 적용할 ProcessorFunc를 추가한다.
// 여러 번 호출하면 순서대로 체인이 구성된다.
func WithProcessor(fn ProcessorFunc) Option {
	return func(p *Pipeline) { p.processors = append(p.processors, fn) }
}

// WithProgress는 진행 상황 콜백과 최소 보고 간격을 설정한다.
func WithProgress(fn OnProgress, interval time.Duration) Option {
	return func(p *Pipeline) {
		p.onProgress = fn
		if interval > 0 {
			p.progressInt = interval
		}
	}
}

// Process는 r에서 라인 단위로 읽고 ProcessorFunc 체인을 적용하여 라인 목록을 반환한다.
// ctx가 취소되면 즉시 반환한다. maxBytes가 설정된 경우 초과 시 중단한다.
func (p *Pipeline) Process(ctx context.Context, r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	var lines []string
	var totalBytes int64
	start := time.Now()
	lastProgress := start

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			slog.Debug("pipeline context cancelled", "lines_read", len(lines))
			return lines, ctx.Err()
		default:
		}

		line := scanner.Text()
		totalBytes += int64(len(line)) + 1 // +1 for newline

		// 전처리기 체인 적용
		for _, proc := range p.processors {
			line = proc(line)
			if line == "" {
				break // 빈 문자열이면 해당 라인 건너뜀
			}
		}

		if line != "" {
			lines = append(lines, line)
		}

		// 최대 바이트 제한
		if p.maxBytes > 0 && int(totalBytes) >= p.maxBytes {
			slog.Debug("pipeline max bytes reached", "bytes", totalBytes, "limit", p.maxBytes)
			break
		}

		// 진행 상황 보고
		if p.onProgress != nil && time.Since(lastProgress) >= p.progressInt {
			p.onProgress(ProgressInfo{
				LinesRead:   len(lines),
				BytesRead:   totalBytes,
				ElapsedTime: time.Since(start),
			})
			lastProgress = time.Now()
		}
	}

	if err := scanner.Err(); err != nil {
		return lines, err
	}

	// 완료 콜백
	if p.onProgress != nil {
		p.onProgress(ProgressInfo{
			LinesRead:   len(lines),
			BytesRead:   totalBytes,
			ElapsedTime: time.Since(start),
			Done:        true,
		})
	}

	return lines, nil
}

// ProcessString은 문자열 전체를 파이프라인으로 처리한다.
func (p *Pipeline) ProcessString(ctx context.Context, s string) ([]string, error) {
	return p.Process(ctx, strings.NewReader(s))
}
