package bash

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/koreaf16/argus/internal/services/workspace"
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/shellsignal"
	"github.com/koreaf16/argus/internal/types"
	"github.com/koreaf16/argus/internal/utils"
	ibash "github.com/koreaf16/argus/internal/utils/bash"
	"github.com/koreaf16/argus/internal/utils/shell"
)

// replPromptRe: 대표적인 인터랙티브 REPL 프롬프트 패턴 (줄 끝에 위치)
var replPromptRe = regexp.MustCompile(`(?m)(SQL>\s*$|mysql>\s*$|MariaDB\s*\[.*?\]>\s*$|sqlite>\s*$|postgres[=#>]+\s*$|>>>\s*$|>\s+$)`)

type BashTool struct {
	provider     shell.ShellProvider
	providerOnce sync.Once
	providerErr  error
}

func NewBashTool() *BashTool {
	return &BashTool{}
}

func (t *BashTool) ensureProvider(ctx context.Context) error {
	t.providerOnce.Do(func() {
		shellPath, err := utils.FindSuitableShell()
		if err != nil {
			t.providerErr = err
			return
		}
		p, err := shell.CreateBashShellProvider(ctx, shellPath, ibash.CreateAndSaveSnapshot)
		if err != nil {
			t.providerErr = err
			return
		}
		t.provider = p
	})
	return t.providerErr
}

func (t *BashTool) Name() string { return "bash" }
func (t *BashTool) Description(ctx tool.Context) string { return "Execute bash shell commands" }
func (t *BashTool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"command":    map[string]any{"type": "string"},
			"server":     map[string]any{"type": "string"},
			"password":   map[string]any{"type": "string"},
			"background": map[string]any{"type": "boolean"},
		},
		"required": []string{"command"},
	}
}

func (t *BashTool) IsReadOnly() bool               { return false }
func (t *BashTool) MaxResultSizeChars() int        { return 100000 }
func (t *BashTool) IsConcurrencySafe(json.RawMessage) bool { return false }

func (t *BashTool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	events := make(chan tool.ToolEvent, 2)
	var req struct {
		Command    string `json:"command"`
		Server     string `json:"server"`
		Password   string `json:"password"`
		Background bool   `json:"background"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	go func() {
		defer close(events)
		targetAlias, aliasErr := tool.ResolveValidatedWorkspaceAlias(ctx, req.Server)
		if aliasErr != nil {
			events <- tool.NewErrorEvent(aliasErr)
			return
		}

		// OS 인식 기반 셸 호환성 검증. 대상이 Windows이면서 bash가 설치되어 있지 않은 경우
		// 무지성으로 bash로 실행하면 명령이 알 수 없는 방식으로 실패하므로 사전에 차단한다.
		targetInfo, infoErr := tool.ResolveShellTargetInfo(ctx, req.Server, true)
		if infoErr != nil {
			events <- tool.NewErrorEvent(infoErr)
			return
		}
		if err := tool.ValidateShellCompatibility("bash", targetInfo); err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		if req.Password != "" && ctx.Workspace != nil {
			ctx.Workspace.SetPassword(targetAlias, "ssh", req.Password)
			ctx.Workspace.SetPassword(targetAlias, "sudo", req.Password)
		}

		// Windows + Git Bash 같은 케이스를 위해 ExecOptions.Shell을 환경에 맞게 선택한다.
		shellKind := "bash"
		if targetInfo.Platform == workspace.PlatformWindows && targetInfo.BashAvailable {
			shellKind = "git-bash"
		}

		finalCommand := req.Command
		if req.Background && tool.IsRemoteWorkspace(ctx, targetAlias) {
			jobID := time.Now().Unix()
			finalCommand = fmt.Sprintf("{ (%s); echo $? > /tmp/argus_%d.exit; } > /tmp/argus_%d.log 2>&1 & echo \"[PID] $! [EXIT] /tmp/argus_%d.exit\"", req.Command, jobID, jobID, jobID)
		}

		handle, err := ctx.Workspace.StartExecWithOptions(ctx.Context, targetAlias, finalCommand, workspace.ExecOptions{Shell: shellKind}, nil)
		if err != nil {
			events <- tool.NewErrorEvent(err)
			return
		}

		// UI 패널과 stdin을 연결하는 채널을 즉시 생성해 전달
		inputCh := make(chan string, 64)
		events <- tool.ToolEvent{Kind: tool.ToolEventChunk, Output: "", InputResponse: inputCh}

		var (
			outputBuffer    string
			lastAnalyzedPos int
			idleDuration    = 600 * time.Millisecond // 출력 중단 대기 시간
			idleTimer       = time.NewTimer(idleDuration)
		)
		if !idleTimer.Stop() {
			<-idleTimer.C
		}

		for {
			select {
			case <-ctx.Context.Done():
				return
			case input := <-inputCh:
				if shellsignal.IsBackgroundRequest(input) {
					handle.Kill()
				} else {
					_ = handle.Write(input)
				}
			case chunk, ok := <-handle.Stream:
				if !ok {
					goto FINISH
				}
				outputBuffer += chunk
				events <- tool.NewChunkEvent(chunk)
				idleTimer.Stop()
				idleTimer.Reset(idleDuration)

			case <-idleTimer.C:
				// [지능형 분석 로직 작동] 출력이 멈췄을 때 상황 분석
				if len(outputBuffer) > lastAnalyzedPos {
					currentTail := outputBuffer[lastAnalyzedPos:]
					if len(currentTail) > 1000 {
						currentTail = currentTail[len(currentTail)-1000:]
					}
					lastAnalyzedPos = len(outputBuffer)

					// 빠른 경로: LLM 없이 일반적인 패스워드/프롬프트 패턴 감지
					tailLower := strings.ToLower(currentTail)

					// REPL 프롬프트 감지: SQL>, mysql>, >>> 등 → exit 자동 전송
					if replPromptRe.MatchString(currentTail) {
						_ = handle.Write("exit\n")
						idleTimer.Reset(idleDuration)
						continue
					}

					isPasswordPrompt := strings.Contains(tailLower, "password:") ||
						strings.Contains(tailLower, "암호:") ||
						strings.Contains(tailLower, "[sudo]") ||
						strings.Contains(tailLower, "passphrase") ||
						strings.Contains(tailLower, "enter password")
					isChoicePrompt := strings.Contains(tailLower, "replace?") ||
						strings.Contains(tailLower, "[y]es") ||
						strings.Contains(tailLower, "[y/n]") ||
						strings.Contains(tailLower, "[a]ll")

					if isPasswordPrompt {
						pw := ctx.Workspace.GetPassword(targetAlias, "ssh")
						if pw != "" {
							_ = handle.Write(pw + "\n")
							idleTimer.Reset(idleDuration)
							continue
						}
					} else if isChoicePrompt {
						if strings.Contains(tailLower, "[a]ll") {
							_ = handle.Write("A\n")
						} else {
							_ = handle.Write("y\n")
						}
						idleTimer.Reset(idleDuration)
						continue
					}

					// 느린 경로: LLM으로 복잡한 상황 분석
					kind, prompt, ok := analyzeInteractionSituation(ctx, currentTail)
					if ok {
						kindUpper := strings.ToUpper(strings.TrimSpace(kind))

						if kindUpper == "PASSWORD" || kindUpper == "SUDO" || kindUpper == "SSH" {
							pw := ctx.Workspace.GetPassword(targetAlias, "ssh")
							if pw != "" {
								_ = handle.Write(pw + "\n")
								idleTimer.Reset(idleDuration)
							}
						} else if kindUpper == "CHOICE" || kindUpper == "YES_NO" || kindUpper == "TEXT" {
							if strings.Contains(strings.ToLower(prompt), "replace") || strings.Contains(strings.ToLower(prompt), "[y/n]") {
								if strings.Contains(strings.ToLower(prompt), "[a]ll") {
									_ = handle.Write("A\n")
								} else {
									_ = handle.Write("y\n")
								}
								idleTimer.Reset(idleDuration)
							}
						}
					}
				}

			case res, ok := <-handle.Result:
				idleTimer.Stop()
				if !ok {
					events <- tool.NewErrorEvent(fmt.Errorf("remote channel closed unexpectedly"))
					return
				}
				resJSON, _ := json.Marshal(res)
				events <- tool.NewOutputEvent(string(resJSON))
				return
			}
		}
	FINISH:
		events <- tool.NewDoneEvent()
	}()

	return events, nil
}

func analyzeInteractionSituation(ctx tool.Context, tail string) (kind, prompt string, ok bool) {
	if ctx.ExecuteSubQuery == nil {
		return "", "", false
	}

	systemPrompt := `Analyze terminal output. If waiting for user input, respond JSON: {"is_waiting":true, "type":"PASSWORD|YES_NO|CHOICE", "prompt_label":"..."}. Else: {"is_waiting":false}.`
	userPrompt := fmt.Sprintf("Output:\n---\n%s\n---", tail)
	
	resp, err := ctx.ExecuteSubQuery(ctx.Context, systemPrompt, userPrompt)
	if err != nil {
		return "", "", false
	}

	var result struct {
		IsWaiting   bool   `json:"is_waiting"`
		Type        string `json:"type"`
		PromptLabel string `json:"prompt_label"`
	}
	
	// JSON 파싱 (마크다운 가드 제거 로직 포함)
	jsonStr := resp
	if start := strings.Index(resp, "{"); start != -1 {
		if end := strings.LastIndex(resp, "}"); end != -1 {
			jsonStr = resp[start : end+1]
		}
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return "", "", false
	}

	return result.Type, result.PromptLabel, result.IsWaiting
}

func (t *BashTool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	rawServer := tool.ExtractStringInput(input, "server")
	targetInfo, err := tool.ResolveShellTargetInfo(ctx, rawServer, false)
	if err != nil {
		return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
	}
	if targetInfo.Platform != workspace.PlatformUnknown {
		if err := tool.ValidateShellCompatibility("bash", targetInfo); err != nil {
			return tool.PermissionResult{Behavior: types.BehaviorDeny, Message: err.Error()}, nil
		}
	}
	return types.PermissionResult{Behavior: types.BehaviorAllow}, nil
}
