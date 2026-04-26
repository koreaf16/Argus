// Package mcp
// File: stdio.go
// Description: MCP stdio 전송 — 서브프로세스와 JSON-RPC 2.0 통신
// Responsibility: 서브프로세스 기동, stdin/stdout으로 JSON-RPC 메시지 송수신

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

// StdioTransport는 서브프로세스와 JSON-RPC 2.0으로 통신하는 전송 계층이다.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	nextID atomic.Int64
}

// NewStdioTransport는 command와 args로 서브프로세스를 준비하는 StdioTransport를 생성한다.
// Start()를 호출해야 실제로 프로세스가 기동된다.
func NewStdioTransport(command string, args []string, env map[string]string) *StdioTransport {
	cmd := exec.Command(command, args...)

	// 환경변수 확장 적용
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+expandEnv(v))
	}

	return &StdioTransport{cmd: cmd}
}

// Start는 서브프로세스를 기동하고 stdin/stdout 파이프를 연결한다.
func (t *StdioTransport) Start() error {
	stdin, err := t.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %w", err)
	}

	stdout, err := t.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := t.cmd.Start(); err != nil {
		return fmt.Errorf("start mcp process: %w", err)
	}

	t.stdin = stdin
	t.stdout = bufio.NewReader(stdout)
	slog.Debug("mcp subprocess started", "pid", t.cmd.Process.Pid)
	return nil
}

// Send는 JSON-RPC 요청을 전송하고 응답을 반환한다.
func (t *StdioTransport) Send(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	id := t.nextID.Add(1)
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// 개행 구분자로 전송
	if _, err := fmt.Fprintf(t.stdin, "%s\n", data); err != nil {
		return nil, fmt.Errorf("write to stdin: %w", err)
	}

	// 컨텍스트 타임아웃을 고려한 응답 읽기
	type readResult struct {
		resp JSONRPCResponse
		err  error
	}
	ch := make(chan readResult, 1)

	go func() {
		line, err := t.stdout.ReadString('\n')
		if err != nil {
			ch <- readResult{err: fmt.Errorf("read from stdout: %w", err)}
			return
		}
		var resp JSONRPCResponse
		if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &resp); err != nil {
			ch <- readResult{err: fmt.Errorf("unmarshal response: %w", err)}
			return
		}
		ch <- readResult{resp: resp}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("request timed out: %w", ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		if r.resp.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", r.resp.Error.Code, r.resp.Error.Message)
		}
		return r.resp.Result, nil
	}
}

// Notify는 응답을 기다리지 않는 JSON-RPC 알림을 전송한다.
func (t *StdioTransport) Notify(method string, params interface{}) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal notify: %w", err)
	}
	if _, err := fmt.Fprintf(t.stdin, "%s\n", data); err != nil {
		return fmt.Errorf("write notify: %w", err)
	}
	return nil
}

// Close는 서브프로세스를 종료한다.
func (t *StdioTransport) Close() error {
	if t.stdin != nil {
		t.stdin.Close()
	}
	if t.cmd != nil && t.cmd.Process != nil {
		if err := t.cmd.Process.Kill(); err != nil {
			slog.Debug("kill mcp process", "err", err)
		}
		t.cmd.Wait() //nolint
	}
	return nil
}

// expandEnv는 문자열의 ${VAR} 패턴을 환경변수 값으로 치환한다.
func expandEnv(s string) string {
	return os.ExpandEnv(s)
}
