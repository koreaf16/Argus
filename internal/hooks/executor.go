package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/utils"
)

const defaultHookTimeoutSec = 60
const defaultPromptTimeoutSec = 30

// Executor 는 command 타입 훅을 실행한다.
type Executor struct {
	subQueryFn SubQueryFn // prompt 타입 훅에 사용; nil 이면 prompt 훅 비활성
}

// Run 은 단일 HookCommand 를 실행하고 결과를 반환한다.
func (e *Executor) Run(ctx context.Context, cmd HookCommand, input HookInput) HookExecResult {
	switch cmd.Type {
	case HookCommandTypeHTTP:
		return e.runHTTPHook(ctx, cmd, input)
	case HookCommandTypePrompt:
		return e.runPromptHook(ctx, cmd, input)
	}

	shellPath, args, err := resolveShell(cmd)
	if err != nil {
		return HookExecResult{
			ExitCode: 1,
			Stderr:   "hook: " + err.Error(),
			Continue: true,
		}
	}

	timeout := time.Duration(cmd.Timeout) * time.Second
	if timeout <= 0 {
		timeout = defaultHookTimeoutSec * time.Second
	}

	sc := utils.RunCommand(ctx, shellPath, args, buildEnv(input), input.WorkingDir, timeout, false)
	execResult := <-sc.Result
	sc.Cleanup()

	result := HookExecResult{
		ExitCode: execResult.Code,
		Stdout:   execResult.Stdout,
		Stderr:   execResult.Stderr,
		Continue: true,
	}

	// exit code 2: 차단
	if execResult.Code == 2 {
		result.Continue = false
		result.StopReason = strings.TrimSpace(execResult.Stderr)
	}

	// stdout에서 JSON 파싱 (exit code 0/2 무관하게 적용)
	parseJSONOutput(execResult.Stdout, &result)

	return result
}

// resolveShell 은 cmd.Shell 설정에 따라 셸 경로와 인수를 결정한다.
func resolveShell(cmd HookCommand) (path string, args []string, err error) {
	shell := strings.ToLower(strings.TrimSpace(cmd.Shell))

	switch shell {
	case "powershell", "pwsh":
		pwsh, e := resolvePowerShell()
		if e != nil {
			return "", nil, e
		}
		return pwsh, []string{"-NonInteractive", "-NoProfile", "-Command", cmd.Command}, nil
	default:
		// bash / 기본
		sh, e := utils.FindSuitableShell()
		if e != nil {
			return "", nil, e
		}
		return sh, []string{"-c", cmd.Command}, nil
	}
}

// resolvePowerShell 은 pwsh.exe → powershell.exe 순서로 탐색한다.
func resolvePowerShell() (string, error) {
	for _, name := range []string{"pwsh", "pwsh.exe", "powershell", "powershell.exe"} {
		if p, err := findExecutable(name); err == nil {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

// findExecutable 은 PATH에서 실행 파일을 찾는다.
func findExecutable(name string) (string, error) {
	// os/exec.LookPath 와 동일하지만 직접 구현해 import 없이 처리
	return lookupPath(name)
}

// lookupPath 는 os/exec.LookPath 를 사용하지 않기 위해 PATH 탐색을 직접 수행한다.
func lookupPath(name string) (string, error) {
	pathEnv := os.Getenv("PATH")
	sep := string(os.PathListSeparator)
	for _, dir := range strings.Split(pathEnv, sep) {
		if dir == "" {
			continue
		}
		full := dir + string(os.PathSeparator) + name
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return full, nil
		}
	}
	return "", os.ErrNotExist
}

// buildEnv 는 훅 실행 시 주입할 환경변수 맵을 반환한다.
func buildEnv(input HookInput) map[string]string {
	env := map[string]string{
		// Argus 네이티브
		"ARGUS_SESSION_ID":      input.SessionID,
		"ARGUS_PROJECT_ROOT":    input.WorkingDir,
		"ARGUS_TOOL_NAME":       input.ToolName,
		"ARGUS_TOOL_INPUT_JSON": input.ToolInputJSON,
		"ARGUS_USER_PROMPT":     input.UserPrompt,
		// Claude CLI 호환 별칭
		"CLAUDE_SESSION_ID":      input.SessionID,
		"CLAUDE_TOOL_NAME":       input.ToolName,
		"CLAUDE_TOOL_INPUT_JSON": input.ToolInputJSON,
	}
	return env
}

// hookJSONOutput 은 훅 stdout의 JSON 출력 스키마다.
type hookJSONOutput struct {
	Continue     *bool          `json:"continue"`
	StopReason   string         `json:"stopReason"`
	Decision     string         `json:"decision"` // "approve" | "block"
	Reason       string         `json:"reason"`
	UpdatedInput map[string]any `json:"updatedInput"`
}

// runHTTPHook 은 http 타입 훅을 실행한다. HookInput 을 JSON body 로 POST 한다.
func (e *Executor) runHTTPHook(ctx context.Context, cmd HookCommand, input HookInput) HookExecResult {
	timeout := time.Duration(cmd.Timeout) * time.Second
	if timeout <= 0 {
		timeout = defaultHookTimeoutSec * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(map[string]any{
		"session_id":    input.SessionID,
		"working_dir":   input.WorkingDir,
		"tool_name":     input.ToolName,
		"tool_input":    json.RawMessage(nullableJSON(input.ToolInputJSON)),
		"tool_result":   input.ToolResult,
		"is_tool_error": input.IsToolError,
		"user_prompt":   input.UserPrompt,
	})
	if err != nil {
		return HookExecResult{ExitCode: 1, Stderr: "hook http: marshal input: " + err.Error(), Continue: true}
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, cmd.URL, bytes.NewReader(body))
	if err != nil {
		return HookExecResult{ExitCode: 1, Stderr: "hook http: build request: " + err.Error(), Continue: true}
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range cmd.Headers {
		req.Header.Set(k, interpolateEnvVars(v, cmd.AllowedEnvVars))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return HookExecResult{ExitCode: 1, Stderr: "hook http: request failed: " + err.Error(), Continue: true}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	result := HookExecResult{Continue: true, ExitCode: 0}

	if resp.StatusCode >= 400 {
		result.ExitCode = 1
		result.Stderr = strings.TrimSpace(string(respBody))
		return result
	}

	parseJSONOutput(string(respBody), &result)
	return result
}

// nullableJSON 은 빈 문자열을 "null" 로 변환해 JSON 직렬화 시 null 이 되도록 한다.
func nullableJSON(s string) string {
	if strings.TrimSpace(s) == "" {
		return "null"
	}
	return s
}

// envVarPattern 은 $VAR 또는 ${VAR} 패턴을 찾는다.
var envVarPattern = regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?`)

// interpolateEnvVars 는 value 내의 환경변수 참조를 실제 값으로 치환한다.
// allowed == nil: 모든 변수 허용. allowed != nil: 목록에 있는 변수만 치환 (빈 목록이면 전부 차단).
func interpolateEnvVars(value string, allowed []string) string {
	allowSet := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowSet[name] = true
	}
	return envVarPattern.ReplaceAllStringFunc(value, func(match string) string {
		sub := envVarPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		name := sub[1]
		if allowed != nil && !allowSet[name] {
			return ""
		}
		return os.Getenv(name)
	})
}

// promptResponseJSON 은 prompt 타입 훅 LLM 응답 스키마다.
type promptResponseJSON struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason"`
}

// runPromptHook 은 prompt 타입 훅을 실행한다.
// LLM에 단일 턴 쿼리를 보내고, {"ok":false} 응답 시 실행을 차단한다.
func (e *Executor) runPromptHook(ctx context.Context, cmd HookCommand, input HookInput) HookExecResult {
	if e.subQueryFn == nil {
		return HookExecResult{
			Continue: true,
			Stderr:   "hook prompt: subQueryFn not configured",
			ExitCode: 1,
		}
	}

	timeout := time.Duration(cmd.Timeout) * time.Second
	if timeout <= 0 {
		timeout = defaultPromptTimeoutSec * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// $ARGUMENTS를 HookInput JSON으로 치환
	argsJSON, _ := json.Marshal(input)
	userPrompt := strings.ReplaceAll(cmd.Prompt, "$ARGUMENTS", string(argsJSON))

	const systemPrompt = `You are a security policy enforcer. Respond ONLY with valid JSON: {"ok":true} to allow, or {"ok":false,"reason":"<brief reason>"} to block.`

	resp, err := e.subQueryFn(reqCtx, systemPrompt, userPrompt)
	if err != nil {
		return HookExecResult{Continue: true, Stderr: "hook prompt: llm error: " + err.Error(), ExitCode: 1}
	}

	// 응답에서 첫 번째 JSON 객체 파싱
	var pr promptResponseJSON
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		if err := json.Unmarshal([]byte(line), &pr); err == nil {
			break
		}
	}

	if !pr.OK {
		reason := pr.Reason
		if reason == "" {
			reason = "prompt hook blocked execution"
		}
		return HookExecResult{
			ExitCode:   2,
			Continue:   false,
			StopReason: reason,
			Stderr:     reason,
		}
	}
	return HookExecResult{Continue: true, ExitCode: 0}
}

// parseJSONOutput 은 stdout 에서 첫 번째 JSON 라인을 찾아 result 에 적용한다.
func parseJSONOutput(stdout string, result *HookExecResult) {
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var out hookJSONOutput
		if err := json.Unmarshal([]byte(line), &out); err != nil {
			continue
		}
		if out.Continue != nil && !*out.Continue {
			result.Continue = false
		}
		if out.StopReason != "" {
			result.StopReason = out.StopReason
		}
		if out.Decision != "" {
			result.Decision = out.Decision
			if out.Decision == "block" {
				result.Continue = false
				if result.StopReason == "" {
					result.StopReason = out.Reason
				}
			}
		}
		if out.Reason != "" {
			result.Reason = out.Reason
		}
		if out.UpdatedInput != nil {
			result.UpdatedInput = out.UpdatedInput
		}
		return
	}
}
