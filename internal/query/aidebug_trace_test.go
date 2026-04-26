package query

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/state"
)

type traceCaptureEmitter struct {
	mu      sync.Mutex
	records []TraceRecord
}

func (e *traceCaptureEmitter) Emit(record TraceRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.records = append(e.records, record)
}

func (e *traceCaptureEmitter) list() []TraceRecord {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]TraceRecord, len(e.records))
	copy(out, e.records)
	return out
}

type fakeTraceLLM struct{}

func (f *fakeTraceLLM) Stream(_ context.Context, req llm.Request) (<-chan llm.Event, error) {
	if req.TraceHook != nil {
		reqCopy := req
		reqCopy.TraceHook = nil
		req.TraceHook(llm.TraceEvent{
			Kind:     llm.TraceKindRequest,
			Provider: "fake",
			Model:    "fake-model",
			Raw:      json.RawMessage(`{"request":"raw"}`),
			Request:  &reqCopy,
		})
		req.TraceHook(llm.TraceEvent{
			Kind:        llm.TraceKindProviderChunk,
			Provider:    "fake",
			Model:       "fake-model",
			Raw:         json.RawMessage(`{"chunk":"raw"}`),
			MappedKind:  llm.EventTextDelta,
			MappedDelta: "ok",
		})
	}
	stop := llm.StopReasonEndTurn
	ch := make(chan llm.Event, 2)
	ch <- llm.Event{Kind: llm.EventTextDelta, Delta: "done"}
	ch <- llm.Event{Kind: llm.EventStop, Stop: &stop}
	close(ch)
	return ch, nil
}

func (f *fakeTraceLLM) CountTokens(context.Context, llm.Request) (int, error) {
	return 16, nil
}

func (f *fakeTraceLLM) Capabilities() llm.Caps {
	return llm.Caps{}
}

func (f *fakeTraceLLM) Provider() string {
	return "fake"
}

func TestAIDebugProviderChunkRawAlwaysIncluded(t *testing.T) {
	emitter := &traceCaptureEmitter{}
	engine := NewEngine(&fakeTraceLLM{}, nil, state.NewAppState(), nil)
	engine.SetDeps(context.Background(), Deps{
		WorkingDir: "C:\\repo",
		SessionID:  "s1",
		AIDebug: AIDebugConfig{
			Enabled: true,
			Emitter: emitter,
		},
	})

	ch, err := engine.SubmitMessage(context.Background(), "hello")
	if err != nil {
		t.Fatalf("submit message: %v", err)
	}
	for range ch {
	}

	foundProviderChunk := false
	for _, rec := range emitter.list() {
		if rec.Type != "llm.provider_chunk" {
			continue
		}
		foundProviderChunk = true
		data, ok := rec.Data.(map[string]any)
		if !ok {
			t.Fatalf("unexpected provider chunk data type: %T", rec.Data)
		}
		if _, exists := data["raw"]; !exists {
			t.Fatalf("provider chunk should always include raw payload: %+v", data)
		}
	}
	if !foundProviderChunk {
		t.Fatalf("expected provider chunk trace record")
	}
}
