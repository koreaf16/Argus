// Package executor
// File: parallel_test.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package executor

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// parallelMockExecutor??병렬 ?�스?�용 ?�순 mock executor?�다.
type parallelMockExecutor struct {
	target string
	out    string
	err    error
	delay  time.Duration
}

func (m *parallelMockExecutor) Execute(_ context.Context, _ string) (ExecResult, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return ExecResult{Stdout: m.out}, m.err
}
func (m *parallelMockExecutor) Target() string { return m.target }
func (m *parallelMockExecutor) Host() string   { return m.target }

func newTestManager(servers map[string]string) *Manager {
	mgr := NewManager(&parallelMockExecutor{target: "localhost", out: "local"})
	for name, out := range servers {
		mgr.Register(name, &parallelMockExecutor{target: name, out: out})
	}
	return mgr
}

func TestFanOut_AllSuccess(t *testing.T) {
	mgr := newTestManager(map[string]string{
		"web1": "web1-result",
		"web2": "web2-result",
		"web3": "web3-result",
	})
	targets := []string{"web1", "web2", "web3"}

	results := FanOut(context.Background(), mgr, targets, "echo test", DefaultParallelConfig)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("server %s: unexpected error %v", r.Target, r.Err)
		}
	}
}

func TestFanOut_PartialFailure(t *testing.T) {
	mgr := NewManager(&parallelMockExecutor{target: "localhost", out: "ok"})
	mgr.Register("good", &parallelMockExecutor{target: "good", out: "ok"})
	// "bad" ???�록?��? ?�음 ??manager.Get() ?�패

	targets := []string{"good", "bad-missing"}
	results := FanOut(context.Background(), mgr, targets, "ls", DefaultParallelConfig)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	byTarget := make(map[string]ParallelResult)
	for _, r := range results {
		byTarget[r.Target] = r
	}

	if byTarget["good"].Err != nil {
		t.Errorf("good server should succeed, got err: %v", byTarget["good"].Err)
	}
	if byTarget["bad-missing"].Err == nil {
		t.Error("bad-missing server should fail")
	}
}

func TestFanOut_ResultOrder(t *testing.T) {
	// 결과가 targets ?�서?� ?�일?��? ?�인
	mgr := newTestManager(map[string]string{
		"s1": "out1",
		"s2": "out2",
		"s3": "out3",
	})
	targets := []string{"s1", "s2", "s3"}
	results := FanOut(context.Background(), mgr, targets, "echo", DefaultParallelConfig)

	for i, r := range results {
		if r.Target != targets[i] {
			t.Errorf("result[%d].Target = %s; want %s", i, r.Target, targets[i])
		}
	}
}

func TestFanOut_ConcurrencyLimit(t *testing.T) {
	// ?�시 ?�행 goroutine??Concurrency�?초과?��? ?�는지 ?�인
	var concurrent int64
	var peak int64

	mgr := NewManager(&parallelMockExecutor{target: "localhost"})
	for i := range 10 {
		name := "srv" + string(rune('0'+i))
		mgr.Register(name, &parallelMockExecutor{target: name, out: "ok"})
	}
	targets := make([]string, 10)
	for i := range targets {
		targets[i] = "srv" + string(rune('0'+i))
	}

	cfg := ParallelConfig{Concurrency: 3}
	FanOutFunc(context.Background(), mgr, targets, func(ctx context.Context, exec Executor) (ExecResult, error) {
		cur := atomic.AddInt64(&concurrent, 1)
		for {
			p := atomic.LoadInt64(&peak)
			if cur <= p || atomic.CompareAndSwapInt64(&peak, p, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt64(&concurrent, -1)
		return ExecResult{Stdout: "ok"}, nil
	}, cfg)

	if peak > 3 {
		t.Errorf("peak concurrency = %d; want <= 3", peak)
	}
}

func TestFanOut_EmptyTargets(t *testing.T) {
	mgr := NewManager(&parallelMockExecutor{target: "localhost"})
	results := FanOut(context.Background(), mgr, nil, "echo", DefaultParallelConfig)
	if len(results) != 0 {
		t.Errorf("empty targets should return empty results, got %d", len(results))
	}
}

func TestFanOut_ContextCancellation(t *testing.T) {
	mgr := newTestManager(map[string]string{"s1": "ok", "s2": "ok"})
	targets := []string{"s1", "s2"}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// �??�버가 100ms ?�기하?�록
	mgr.Register("slow1", &parallelMockExecutor{target: "slow1", delay: 100 * time.Millisecond})
	mgr.Register("slow2", &parallelMockExecutor{target: "slow2", delay: 100 * time.Millisecond})
	targets = []string{"slow1", "slow2"}

	results := FanOut(ctx, mgr, targets, "sleep 1", DefaultParallelConfig)
	_ = results
	// 컨텍?�트 취소 ?�후 반환?�면 ?�과
}

// --- FormatParallelResults ?�스??---

func TestFormatParallelResults_AllSuccess(t *testing.T) {
	results := []ParallelResult{
		{Target: "web1", Result: ExecResult{Stdout: "hello"}, Duration: 100 * time.Millisecond},
		{Target: "web2", Result: ExecResult{Stdout: "world"}, Duration: 200 * time.Millisecond},
	}
	out := FormatParallelResults(results)

	if !strings.Contains(out, "web1") || !strings.Contains(out, "hello") {
		t.Error("web1 result missing from output")
	}
	if !strings.Contains(out, "web2") || !strings.Contains(out, "world") {
		t.Error("web2 result missing from output")
	}
}

func TestFormatParallelResults_WithError(t *testing.T) {
	results := []ParallelResult{
		{Target: "good", Result: ExecResult{Stdout: "ok"}, Duration: 100 * time.Millisecond},
		{Target: "bad", Err: errors.New("connection refused")},
	}
	out := FormatParallelResults(results)

	if !strings.Contains(out, "bad") {
		t.Error("bad server should appear in output")
	}
	if !strings.Contains(out, "connection refused") {
		t.Error("error message should appear in output")
	}
}

func TestFormatParallelResults_Empty(t *testing.T) {
	out := FormatParallelResults(nil)
	if out == "" {
		t.Error("empty results should return non-empty placeholder")
	}
}

