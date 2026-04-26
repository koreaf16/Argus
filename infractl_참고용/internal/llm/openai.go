package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// defaultStreamIdleTimeout은 스트림에서 새 데이터가 올 때까지 기다리는 최대 시간이다.
	defaultStreamIdleTimeout = 180 * time.Second

	// maxStreamIdleTimeout은 동적 조절 시 허용하는 idle timeout 상한이다.
	maxStreamIdleTimeout = 300 * time.Second

	// idleTimeoutPerKB는 요청 크기 1KB당 추가되는 idle timeout이다.
	idleTimeoutPerKB = 500 * time.Millisecond
)

type OpenAIClient struct {
	endpoint           string
	model              string
	apiKey             string
	httpClient         *http.Client
	streamClient       *http.Client
	useInlineToolCalls bool          // true면 본문 내 <tool_call> 태그를 가로채서 파싱함 (Qwen 27B 등 특정 모델용)
	streamIdleTimeout  time.Duration // SSE 스트림 청크 간 최대 유휴 시간 (0이면 defaultStreamIdleTimeout 사용)
	temperature        *float64      // nil이면 서버 기본값(1.0) 사용
}

// SetTemperature는 모든 요청에 적용할 temperature를 설정한다.
// nil을 전달하면 서버 기본값(1.0)이 사용된다.
func (c *OpenAIClient) SetTemperature(t *float64) { c.temperature = t }

func NewOpenAIClient(endpoint, model, apiKey string, timeout time.Duration) *OpenAIClient {
	lowerModel := strings.ToLower(model)
	useInline := strings.Contains(lowerModel, "qwen") || strings.Contains(lowerModel, "gemma")

	return &OpenAIClient{
		endpoint:           strings.TrimRight(endpoint, "/"),
		model:              model,
		apiKey:             apiKey,
		useInlineToolCalls: useInline,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		streamClient: &http.Client{
			Timeout: 0, // 전체 스트림 타임아웃은 context로 제어
			Transport: &http.Transport{
				ResponseHeaderTimeout: timeout,
				IdleConnTimeout:       90 * time.Second,
			},
		},
		streamIdleTimeout: defaultStreamIdleTimeout,
	}
}

func (c *OpenAIClient) Chat(ctx context.Context, messages []Message, tools []ToolDef, toolChoice interface{}, opts ...CallOption) (Response, error) {
	reqMessages := messages
	reqTools := tools

	reqToolChoice := toolChoice
	if c.useInlineToolCalls {
		reqMessages = c.transformMessagesForInlineTools(messages)
		reqTools = nil      // Tool API 비활성화
		reqToolChoice = nil // tools 없으면 tool_choice도 제외해야 함 (Qwen API 400 방지)
	}

	callOpts := &CallOptions{}
	for _, opt := range opts {
		opt(callOpts)
	}

	reqBody := chatRequest{
		Model:       c.model,
		Messages:    reqMessages,
		Tools:       reqTools,
		ToolChoice:  reqToolChoice,
		Stream:      false,
		Temperature: c.temperature,
		MaxTokens:   callOpts.MaxTokens,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return Response{}, fmt.Errorf("marshal request: %w", err)
	}

	LogRequestJSON(c.model, data)
	LogRawRequest(c.model, data)

	resp, err := c.doRequest(ctx, data)
	if err != nil {
		logToFile(c.model, "ERROR", err.Error())
		return Response{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return Response{}, fmt.Errorf("read response: %w", err)
	}

	LogRawResponse(c.model, body)

	// vLLM --reasoning-parser는 reasoning 필드를 별도로 반환하므로 인라인 구조체로 파싱한다.
	var raw struct {
		Choices []struct {
			Message struct {
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls"`
				Reasoning string     `json:"reasoning"` // vLLM --reasoning-parser 전용
			} `json:"message"`
		} `json:"choices"`
		Usage *usageInfo `json:"usage"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Response{}, fmt.Errorf("parse response: %w", err)
	}
	if len(raw.Choices) == 0 {
		return Response{}, fmt.Errorf("empty choices in response")
	}

	msg := raw.Choices[0].Message
	content := sanitizeAssistantContent(msg.Content)
	toolCalls := msg.ToolCalls

	// Fallback: sanitizeAssistantContent가 tags를 strip하기 전에 원본에서 inline tool call을 체크한다.
	// sanitize 후 content에는 이미 tag가 없으므로 msg.Content(원본)를 사용해야 한다.
	if len(toolCalls) == 0 && hasInlineToolCallCandidate(msg.Content) {
		if parsed, cleaned := extractInlineToolCalls(msg.Content); len(parsed) > 0 {
			toolCalls = parsed
			content = sanitizeAssistantContent(cleaned)
		}
	}

	result := Response{
		Content:   content,
		Thinking:  msg.Reasoning,
		ToolCalls: toolCalls,
	}
	if raw.Usage != nil {
		result.InputTokens = raw.Usage.PromptTokens
		result.OutputTokens = raw.Usage.CompletionTokens
	}

	logToFile(c.model, "RESPONSE", result.Content)
	if len(result.ToolCalls) > 0 {
		logToolCalls(c.model, result.ToolCalls, "")
	}
	return result, nil
}

func (c *OpenAIClient) ChatStream(ctx context.Context, messages []Message, tools []ToolDef, toolChoice interface{}, onThinkingToken func(string), onToken func(string), opts ...CallOption) (Response, error) {
	reqMessages := messages
	reqTools := tools

	reqToolChoice := toolChoice
	if c.useInlineToolCalls {
		reqMessages = c.transformMessagesForInlineTools(messages)
		reqTools = nil      // Tool API 비활성화
		reqToolChoice = nil // tools 없으면 tool_choice도 제외해야 함 (Qwen API 400 방지)
	}

	callOpts := &CallOptions{}
	for _, opt := range opts {
		opt(callOpts)
	}

	reqBody := chatRequest{
		Model:       c.model,
		Messages:    reqMessages,
		Tools:       reqTools,
		ToolChoice:  reqToolChoice,
		Stream:      true,
		Temperature: c.temperature,
		MaxTokens:   callOpts.MaxTokens,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return Response{}, fmt.Errorf("marshal request: %w", err)
	}

	LogRequestJSON(c.model, data)
	LogRawRequest(c.model, data)

	// idle timeout: 서버가 스트림 중간에 멈춰도 무한 블록되지 않도록 ctx 래핑
	// 요청 크기에 따라 동적 조절: 큰 프롬프트는 LLM 응답 생성이 느릴 수 있다
	idleTimeout := c.streamIdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = adaptiveIdleTimeout(len(data))
	}
	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()

	resp, err := c.doRequestWith(streamCtx, data, c.streamClient)
	if err != nil {
		logToFile(c.model, "ERROR", err.Error())
		return Response{}, err
	}
	defer resp.Body.Close()

	var bodyReader io.Reader = resp.Body
	var rawBuf bytes.Buffer
	if rawLogEnabled {
		bodyReader = io.TeeReader(resp.Body, &rawBuf)
	}

	// 청크 수신마다 타이머를 리셋하는 idle reader로 Body를 래핑한다.
	ir := newIdleReader(bodyReader, idleTimeout, cancelStream)
	defer ir.stop()

	res, err := c.parseStream(c.model, ir, onThinkingToken, onToken)
	if err == nil && rawLogEnabled {
		LogRawResponse(c.model, rawBuf.Bytes())
	}
	return res, err
}

// adaptiveIdleTimeout은 요청 본문 크기에 비례하여 idle timeout을 계산한다.
// 큰 프롬프트(히스토리가 긴 경우)는 LLM이 첫 토큰을 생성하는 데 시간이 더 걸리므로
// timeout을 늘려 불필요한 stream 중단을 방지한다.
func adaptiveIdleTimeout(requestBodyBytes int) time.Duration {
	if requestBodyBytes <= 0 {
		return defaultStreamIdleTimeout
	}

	kb := requestBodyBytes / 1024
	if kb <= 100 {
		total := 90*time.Second + time.Duration(kb)*idleTimeoutPerKB
		if total > maxStreamIdleTimeout {
			return maxStreamIdleTimeout
		}
		return total
	}

	if kb >= 200 {
		return maxStreamIdleTimeout
	}

	base := 90 * time.Second
	softCap := base + 100*time.Duration(idleTimeoutPerKB)
	extra := time.Duration(kb-100) * (maxStreamIdleTimeout - softCap) / 100
	total := softCap + extra
	if total > maxStreamIdleTimeout {
		return maxStreamIdleTimeout
	}
	return total
}

func (c *OpenAIClient) doRequest(ctx context.Context, body []byte) (*http.Response, error) {
	return c.doRequestWith(ctx, body, c.httpClient)
}

func (c *OpenAIClient) doRequestWith(ctx context.Context, body []byte, client *http.Client) (*http.Response, error) {
	url := c.endpoint + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	// 2xx가 아닌 응답: 구조화된 APIError로 변환하여 재시도 판단을 가능하게 한다
	defer resp.Body.Close()
	return nil, parseHTTPError(resp)
}
