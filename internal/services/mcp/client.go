package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const mcpCallTimeout = 60 * time.Second

type mcpClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	nextID atomic.Int64
}

func newMCPClient(cfg ServerConfig) (*mcpClient, error) {
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, fmt.Errorf("mcp server %q has no command", cfg.Name)
	}

	cmd := exec.Command(cfg.Command, cfg.Args...)
	baseEnv := os.Environ()
	for k, v := range cfg.Env {
		baseEnv = append(baseEnv, k+"="+v)
	}
	cmd.Env = baseEnv
	cmd.Stderr = io.Discard

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mcp server %q: %w", cfg.Name, err)
	}

	return &mcpClient{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}, nil
}

func (c *mcpClient) Close() {
	_ = c.stdin.Close()
	done := make(chan struct{})
	go func() {
		_ = c.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = c.cmd.Process.Kill()
	}
}

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *mcpClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	c.mu.Lock()
	defer c.mu.Unlock()

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal rpc request: %w", err)
	}
	b = append(b, '\n')
	if _, err := c.stdin.Write(b); err != nil {
		return nil, fmt.Errorf("mcp write: %w", err)
	}

	type readResult struct {
		line string
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		line, err := c.stdout.ReadString('\n')
		ch <- readResult{line, err}
	}()

	deadline := mcpCallTimeout
	if dl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(dl); remaining < deadline {
			deadline = remaining
		}
	}
	timer := time.NewTimer(deadline)
	defer timer.Stop()

	var line string
	select {
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("mcp read: %w", res.err)
		}
		line = res.line
	case <-timer.C:
		// 타임아웃 발생 시 프로세스 종료를 유도하여 블로킹된 고루틴 해제
		_ = c.cmd.Process.Kill()
		return nil, fmt.Errorf("mcp call %q timed out after %s (process killed to unblock)", method, deadline)
	case <-ctx.Done():
		_ = c.cmd.Process.Kill()
		return nil, ctx.Err()
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &resp); err != nil {
		return nil, fmt.Errorf("mcp parse response: %w (line: %s)", err, line)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

func (c *mcpClient) notify(method string, params any) {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	b, _ := json.Marshal(req)
	b = append(b, '\n')
	_, _ = c.stdin.Write(b)
}

func (c *mcpClient) Initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "argus",
			"version": "1.0",
		},
	}
	if _, err := c.call(ctx, "initialize", params); err != nil {
		return fmt.Errorf("mcp initialize: %w", err)
	}
	c.notify("notifications/initialized", map[string]any{})
	return nil
}

func (c *mcpClient) CallTool(ctx context.Context, toolName string, args map[string]any) (string, bool, error) {
	if args == nil {
		args = map[string]any{}
	}
	params := map[string]any{
		"name":      toolName,
		"arguments": args,
	}
	raw, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return "", true, err
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return string(raw), false, nil
	}

	var parts []string
	for _, c := range result.Content {
		if c.Type == "text" && c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n"), result.IsError, nil
}
