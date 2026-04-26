// Package hooks
// File: backend_http.go
// Description: HTTP POST hook backend 구현
// Responsibility: hook input JSON 을 외부 HTTP 엔드포인트로 전송하고 응답을 HookOutput 으로 파싱

// Ported from: claude_cli/src/utils/hooks/execHttpHook.ts:49-58 (allowedEnvVars 화이트리스트 패턴)
// 핵심 패턴: headers 의 $VAR 보간은 allowedEnvVars 화이트리스트만 허용 (크리덴셜 노출 방지).

package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// httpBackend 는 외부 HTTP 엔드포인트를 호출하는 hook backend 이다.
type httpBackend struct {
	client *http.Client
}

func newHTTPBackend() *httpBackend {
	return &httpBackend{client: &http.Client{Timeout: 10 * time.Minute}}
}

// Run 은 hook input JSON 을 HTTP POST 로 전송하고 응답을 HookOutput 으로 반환한다.
func (b *httpBackend) Run(ctx context.Context, def HookDef, input HookInput) (HookOutput, error) {
	if def.URL == "" {
		return HookOutput{}, fmt.Errorf("http hook: empty url")
	}

	jsonBytes, err := json.Marshal(input)
	if err != nil {
		return HookOutput{}, fmt.Errorf("marshal hook input: %w", err)
	}

	method := strings.ToUpper(def.Method)
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequestWithContext(ctx, method, def.URL, bytes.NewReader(jsonBytes))
	if err != nil {
		return HookOutput{}, fmt.Errorf("create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	for k, v := range def.Headers {
		req.Header.Set(k, interpolateEnvVars(v, def.AllowedEnvVars))
	}

	slog.Debug("http hook request", "url", def.URL, "method", method)

	resp, err := b.client.Do(req)
	if err != nil {
		return HookOutput{}, fmt.Errorf("http hook request: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Debug("http hook: close response body", "err", cerr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return HookOutput{}, fmt.Errorf("read http hook response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return HookOutput{}, fmt.Errorf("http hook: status %d, body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if len(bytes.TrimSpace(body)) == 0 {
		return HookOutput{Decision: DecisionAllow}, nil
	}

	var result HookOutput
	if err := json.Unmarshal(body, &result); err != nil {
		return HookOutput{}, fmt.Errorf("parse http hook response JSON: %w, raw: %s", err, string(body))
	}
	return result, nil
}

// interpolateEnvVars 는 allowedEnvVars 화이트리스트 변수만 $VAR 치환한다.
func interpolateEnvVars(value string, allowedEnvVars []string) string {
	result := value
	for _, k := range allowedEnvVars {
		result = strings.ReplaceAll(result, "$"+k, os.Getenv(k))
		result = strings.ReplaceAll(result, "${"+k+"}", os.Getenv(k))
	}
	return result
}
