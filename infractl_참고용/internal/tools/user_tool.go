// Package tools
// File: user_tool.go
// Description: 사용자 정의 도구 래퍼 — 사용자가 생성한 스크립트를 Tool 인터페이스로 노출
// Responsibility: UserToolEntry를 Tool 인터페이스로 구현, INFRACTL_ARGS_B64로 인자 전달

package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/store"
)

// UserDefinedTool은 사용자가 생성한 스크립트를 Tool 인터페이스로 구현한다.
type UserDefinedTool struct {
	entry store.UserToolEntry
}

// NewUserDefinedTool은 UserToolEntry로부터 UserDefinedTool을 생성한다.
func NewUserDefinedTool(entry store.UserToolEntry) *UserDefinedTool {
	return &UserDefinedTool{entry: entry}
}

func (t *UserDefinedTool) Name() string         { return t.entry.Name }
func (t *UserDefinedTool) Description() string  { return t.entry.Description }
func (t *UserDefinedTool) IsReadOnly() bool     { return false }

// IsEnabled는 스크립트 파일이 존재할 때만 활성화한다.
func (t *UserDefinedTool) IsEnabled() bool {
	_, err := os.Stat(t.entry.ScriptPath)
	return err == nil
}

// Parameters는 저장된 JSON 스키마를 반환한다.
// 저장된 값이 없거나 파싱 실패 시 빈 스키마를 반환한다.
func (t *UserDefinedTool) Parameters() map[string]interface{} {
	if t.entry.Parameters == "" || t.entry.Parameters == "{}" {
		return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	}
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(t.entry.Parameters), &schema); err != nil {
		return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	}
	return schema
}

// Execute는 스크립트를 실행한다.
// LLM 인자는 INFRACTL_ARGS_B64 환경변수로 base64-encoded JSON 형태로 전달되며,
// 사용자 입력이 명령 문자열에 직접 삽입되지 않으므로 셸 인젝션 위험이 없다.
func (t *UserDefinedTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return ToolOutcome{}, fmt.Errorf("marshal args: %w", err)
	}
	argsB64 := base64.StdEncoding.EncodeToString(argsJSON)
	cmd := fmt.Sprintf("INFRACTL_ARGS_B64=%s bash %s", argsB64, t.entry.ScriptPath)

	result, err := exec.Execute(ctx, cmd)
	if err != nil {
		msg := fmt.Sprintf("script execution failed: %s", err)
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}

	output := result.Stdout
	if result.Stderr != "" {
		output += fmt.Sprintf("\n[stderr]\n%s", result.Stderr)
	}
	if result.ExitCode != 0 {
		output = fmt.Sprintf("[Exit Code: %d]\n%s", result.ExitCode, output)
	}
	return ToolOutcome{Content: output, Success: result.ExitCode == 0, ExitCode: result.ExitCode}, nil
}
