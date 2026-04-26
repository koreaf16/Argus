package agent

import (
	"context"

	"github.com/yourorg/infractl/internal/llm"
)

type sequenceLLMClient struct {
	responses []llm.Response
	calls     [][]llm.Message
}

func (c *sequenceLLMClient) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolDef, _ interface{}, opts ...llm.CallOption) (llm.Response, error) {
	if len(c.responses) == 0 {
		return llm.Response{}, nil
	}
	resp := c.responses[0]
	c.responses = c.responses[1:]
	return resp, nil
}

func (c *sequenceLLMClient) ChatStream(
	_ context.Context,
	messages []llm.Message,
	_ []llm.ToolDef,
	_ interface{},
	_ func(string),
	_ func(string),
	opts ...llm.CallOption,
) (llm.Response, error) {
	snapshot := append([]llm.Message(nil), messages...)
	c.calls = append(c.calls, snapshot)
	resp := c.responses[0]
	c.responses = c.responses[1:]
	return resp, nil
}

type captureResponseHandler struct {
	noopAgentEventHandler
	responses []string
}

func (h *captureResponseHandler) OnResponse(content string) {
	h.responses = append(h.responses, content)
}


