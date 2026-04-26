// 파일 역할: Google Gemini 모델과의 통신을 처리하는 LLM 서비스 구현체.
// 포함 모듈: llm, google.golang.org/genai 등.
// 호출/사용 방식: llm.Registry를 통해 서비스 인터페이스를 구현하여 사용.
// 연결: internal/services/llm/llm.go 기반.

package llm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type geminiLLM struct {
	entry      ModelEntry
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewGemini(entry ModelEntry, apiKey string, httpClient *http.Client) LLM {
	baseURL := strings.TrimRight(entry.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &geminiLLM{
		entry:      entry,
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

func (g *geminiLLM) Provider() string { return string(ProviderGemini) }

func (g *geminiLLM) Capabilities() Caps { return g.entry.Caps }

func (g *geminiLLM) CountTokens(ctx context.Context, req Request) (int, error) {
	_ = ctx
	return ApproximateTokenCount(req), nil
}

func (g *geminiLLM) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	if strings.TrimSpace(g.apiKey) == "" {
		return nil, fmt.Errorf("missing gemini api key")
	}
	model := defaultIfEmpty(req.Model, g.entry.ModelID)
	payload := map[string]any{
		"contents": toGeminiContents(req.Messages),
	}
	if len(req.System) > 0 {
		payload["systemInstruction"] = map[string]any{
			"role": "system",
			"parts": []map[string]any{
				{"text": systemToText(req.System)},
			},
		}
	}
	if len(req.Tools) > 0 {
		payload["tools"] = toGeminiTools(req.Tools)
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if req.TraceHook != nil {
		reqCopy := req
		reqCopy.TraceHook = nil
		req.TraceHook(TraceEvent{
			Kind:     TraceKindRequest,
			Provider: g.Provider(),
			Model:    model,
			Raw:      copyRaw(b),
			Request:  &reqCopy,
		})
	}

	endpoint := fmt.Sprintf(
		"%s/models/%s:streamGenerateContent?alt=sse&key=%s",
		strings.TrimRight(g.baseURL, "/"),
		url.PathEscape(model),
		url.QueryEscape(g.apiKey),
	)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("accept", "text/event-stream")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		msg, _ := bufio.NewReader(resp.Body).ReadBytes(0)
		return nil, fmt.Errorf("gemini stream failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	out := make(chan Event, 64)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			var evt geminiStreamEvent
			if err := json.Unmarshal([]byte(data), &evt); err != nil {
				if req.TraceHook != nil {
					req.TraceHook(TraceEvent{
						Kind:       TraceKindProviderChunk,
						Provider:   g.Provider(),
						Model:      model,
						Raw:        copyRaw([]byte(data)),
						MappedKind: EventError,
					})
				}
				out <- Event{Kind: EventError, Err: err}
				continue
			}
			if len(evt.Candidates) == 0 {
				continue
			}
			parts := evt.Candidates[0].Content.Parts
			toolUses := make([]ToolUseStart, 0, len(parts))
			for _, p := range parts {
				if p.Text != "" {
					if req.TraceHook != nil {
						req.TraceHook(TraceEvent{
							Kind:        TraceKindProviderChunk,
							Provider:    g.Provider(),
							Model:       model,
							Raw:         copyRaw([]byte(data)),
							MappedKind:  EventTextDelta,
							MappedDelta: p.Text,
						})
					}
					out <- Event{Kind: EventTextDelta, Delta: p.Text}
				}
				if p.FunctionCall.Name != "" {
					args, _ := json.Marshal(p.FunctionCall.Args)
					toolUses = append(toolUses, ToolUseStart{
						ID:    newGeminiCallID(),
						Name:  p.FunctionCall.Name,
						Input: args,
					})
				}
			}
			if len(toolUses) > 0 {
				for _, tu := range toolUses {
					if req.TraceHook != nil {
						req.TraceHook(TraceEvent{
							Kind:           TraceKindProviderChunk,
							Provider:       g.Provider(),
							Model:          model,
							Raw:            copyRaw([]byte(data)),
							MappedKind:     EventToolUseStart,
							MappedToolName: tu.Name,
						})
					}
					tuCopy := tu
					out <- Event{Kind: EventToolUseStart, ToolUse: &tuCopy}
				}
				stop := StopReasonToolUse
				if req.TraceHook != nil {
					req.TraceHook(TraceEvent{
						Kind:       TraceKindProviderChunk,
						Provider:   g.Provider(),
						Model:      model,
						Raw:        copyRaw([]byte(data)),
						MappedKind: EventStop,
						MappedStop: &stop,
					})
				}
				out <- Event{Kind: EventStop, Stop: &stop}
				return
			}
			if evt.Candidates[0].FinishReason != "" {
				fr := strings.ToUpper(strings.TrimSpace(evt.Candidates[0].FinishReason))
				if fr == "SAFETY" || fr == "RECITATION" {
					out <- Event{Kind: EventError, Err: fmt.Errorf("gemini: response blocked (%s)", fr)}
					return
				}
				stop := StopReasonEndTurn
				if fr == "MAX_TOKENS" {
					stop = StopReasonMaxTokens
				}
				if req.TraceHook != nil {
					req.TraceHook(TraceEvent{
						Kind:       TraceKindProviderChunk,
						Provider:   g.Provider(),
						Model:      model,
						Raw:        copyRaw([]byte(data)),
						MappedKind: EventStop,
						MappedStop: &stop,
					})
				}
				out <- Event{Kind: EventStop, Stop: &stop}
				return
			}
		}
		if err := scanner.Err(); err != nil {
			out <- Event{Kind: EventError, Err: err}
			return
		}
		stop := StopReasonEndTurn
		out <- Event{Kind: EventStop, Stop: &stop}
	}()
	return out, nil
}

type geminiStreamEvent struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text         string `json:"text"`
				FunctionCall struct {
					Name string         `json:"name"`
					Args map[string]any `json:"args"`
				} `json:"functionCall"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
}

func toGeminiContents(msgs []Message) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		role := "user"
		if m.Role == RoleAssistant {
			role = "model"
		}
		parts := make([]map[string]any, 0, len(m.Content))
		for _, b := range m.Content {
			switch b.Type {
			case ContentText:
				if b.Text != "" {
					parts = append(parts, map[string]any{"text": b.Text})
				}
			case ContentToolUse:
				var args map[string]any
				_ = json.Unmarshal(b.Input, &args)
				parts = append(parts, map[string]any{
					"functionCall": map[string]any{
						"name": b.Name,
						"args": args,
					},
				})
			case ContentToolResult:
				parts = append(parts, map[string]any{
					"functionResponse": map[string]any{
						"name": b.Name,
						"response": map[string]any{
							"tool_use_id": b.ToolUseID,
							"text":        b.Text,
							"is_error":    b.IsError,
						},
					},
				})
			}
		}
		if len(parts) == 0 {
			continue
		}
		out = append(out, map[string]any{
			"role":  role,
			"parts": parts,
		})
	}
	return out
}

func toGeminiTools(specs []ToolSpec) []map[string]any {
	funcDecls := make([]map[string]any, 0, len(specs))
	for _, s := range specs {
		if s.Name == "" {
			continue
		}
		funcDecls = append(funcDecls, map[string]any{
			"name":        s.Name,
			"description": s.Description,
			"parameters":  NormalizeSchemaForGemini(s.InputSchema),
		})
	}
	if len(funcDecls) == 0 {
		return nil
	}
	return []map[string]any{
		{"functionDeclarations": funcDecls},
	}
}

func systemToText(blocks []SystemBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func newGeminiCallID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "gemini_" + hex.EncodeToString(b)
}
