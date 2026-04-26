//go:build tools
// +build tools

// Package main
// File: oracle_full_audit.go
// Description: REPL --json 모드를 사용하여 Oracle 설치 시나리오를 자동 완수하고 로그를 수집하는 드라이버
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/config"
	"github.com/yourorg/infractl/internal/llm"
)

func main() {
	ctx := context.Background()
	
	// 1. Setup Tester LLM (to act as the 'User')
	cfg, _ := config.Load()
	llmCfg := cfg.GeneralLLM()
	testerClient := llm.NewOpenAIClient(llmCfg.Endpoint, llmCfg.Model, llmCfg.APIKey, 60*time.Second)

	// 2. Start infractl repl --json as a subprocess
	cmd := exec.Command("./bin/infractl.exe", "repl", "--json")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start infractl: %v", err)
	}

	logFile, _ := os.Create("scratch/oracle_audit_full.log")
	defer logFile.Close()
	multiLog := io.MultiWriter(os.Stdout, logFile)

	// Background stderr reader for internal logs (slog output)
	go func() {
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			fmt.Fprintln(multiLog, "[INTERNAL]", s.Text())
		}
	}()

	fmt.Fprintln(multiLog, "=== Oracle 19c Audit Started ===")

	reader := bufio.NewScanner(stdout)
	const maxCapacity = 10 * 1024 * 1024 // 10MB buffer for large JSON responses
	buf := make([]byte, maxCapacity)
	reader.Buffer(buf, maxCapacity)
	
	// Initial Prompt
	initialPrompt := "이 서버에 oracle19c install할거야. applyRU로 사전에 패치하고 작업하는 방식으로 할거고 c:\\users\\jhkwa\\downloads안에 설치파일 있어. 설치는 oracle 계정 홈에 할거야."
	fmt.Fprintln(multiLog, ">>> Sending Initial Prompt: ", initialPrompt)
	io.WriteString(stdin, initialPrompt+"\n")

	// Event Loop
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "너는 인프라 에이전트인 infractl의 동작을 검증하는 테스터야. 에이전트가 보낸 JSON 로그를 보고, 작업이 막히거나 질문이 오면 적절히 응답해서 Oracle 설치를 끝까지 완수해. 에러가 나면 수정 방향을 지시해. 절대로 포기하지 마."},
	}

	turnCount := 0
	maxTurns := 1000 // 사실상 무제한
for turnCount < maxTurns {
	var turnEvents []string
	var accumulatedResponses []string
	var stopLoop bool
	var turnStatus string
	var statusPrinted bool

	// Read events until we get a 'terminal' or 'ui_interaction'
	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		if line == "" {
			continue
		}

		if !statusPrinted {
			fmt.Fprintf(multiLog, "\n[TURN %d] Agent is working... (Target: Oracle 19c Install Completion)\n", turnCount+1)
			statusPrinted = true
		}

		if !strings.HasPrefix(line, "{") {
			continue
		}

		// 즉시 로그 출력 (Flushing 효과)
		fmt.Fprintln(multiLog, "[AGENT EVENT]", line)

			turnEvents = append(turnEvents, line)

			var msg map[string]any
			json.Unmarshal([]byte(line), &msg)

			event, _ := msg["event"].(string)
			data, _ := msg["data"].(map[string]any)

			// 도구 실행 결과 본문 수집
			if event == "tool_end" {
				if out, ok := data["result"].(string); ok && out != "" {
					accumulatedResponses = append(accumulatedResponses, fmt.Sprintf("[TOOL OUTPUT] %s", out))
				}
			}

			// Accumulate response text
			if event == "response" {
				if text, ok := msg["data"].(string); ok && text != "" {
					accumulatedResponses = append(accumulatedResponses, text)
				}
			}

			// Stop only on terminal or UI interaction
			if event == "terminal" {
				stopLoop = true
				turnStatus = "completed"
				break
			}
			if event == "ui_question" || event == "ui_form" || event == "ui_idle_input" {
				stopLoop = true
				turnStatus = "interaction_required"
				break
			}
			if event == "error" {
				fmt.Fprintln(multiLog, "!!! Engine Error detected !!!")
				stopLoop = true
				turnStatus = "error"
				break
			}
		}

		if err := reader.Err(); err != nil {
			fmt.Fprintln(multiLog, "Error scanning stdout:", err)
			break
		}

		if stopLoop {
			turnCount++
			fmt.Fprintf(multiLog, "\n--- Turn %d Analysis (Status: %s) ---\n", turnCount, turnStatus)

			// [개선] 컨텍스트 관리: 리셋 주기를 3턴으로 단축 (서버 폭발 방지)
			if len(history) > 6 {
				history = []llm.Message{
					history[0], // System message
					{Role: llm.RoleUser, Content: "에이전트가 작업을 수행 중이야. 이전 상세 내역은 잊고 최신 로그만 보고 짧게 명령해."},
				}
				fmt.Fprintln(multiLog, ">>> EMERGENCY CONTEXT RESET: Lightening the load...")
			}

			allResponses := strings.Join(accumulatedResponses, "\n\n")
			// [데이터 다이어트] 테스터에게는 핵심만 요약해서 전달
			if len(allResponses) > 1000 {
				allResponses = allResponses[:500] + "\n... (Long response hidden from Tester to prevent crash) ...\n" + allResponses[len(allResponses)-500:]
			}

			fmt.Fprintln(multiLog, ">>> Tester LLM is deciding next step...")

			// [핵심 개선] 테스터 지침: 텍스트 무시, 로그 기반 명령
			var testerInstruction string
			if turnStatus == "interaction_required" {
				testerInstruction = `질문택일: 선택지 번호나 버튼 라벨만 딱 한 글자/단어로 대답해. 
				만약 'Privilege Password Required' (비밀번호 요청) 질문이 오면, 무조건 'sandbox'라고 답변해. 부연 설명 금지.`
			} else {
				testerInstruction = `로그(AGENT EVENT)만 봐. 텍스트 응답은 엔진이 삭제했으니 무시해.
				1. 도구 실행이 안 됐다면: "당장 [도구명] 실행해" 라고 명령.
				2. 도구가 성공했다면: "이제 [다음도구명] 실행해" 라고 명령.
				안부, 설명, 인사 금지. 오직 '에이전트에게 내릴 명령'만 한 줄로 생성.`
			}

			testerPrompt := fmt.Sprintf("이벤트 스트림:\n%s\n\n에이전트 텍스트:\n%s\n\n지침: %s", strings.Join(turnEvents, "\n"), allResponses, testerInstruction)
			
			history = append(history, llm.Message{Role: llm.RoleUser, Content: testerPrompt})
			resp, err := testerClient.Chat(ctx, history, nil, nil)
			if err != nil {
				fmt.Fprintln(multiLog, "Tester LLM Error:", err)
				break
			}
			
			nextInput := strings.TrimSpace(resp.Content)
			fmt.Fprintln(multiLog, ">>> Tester Decision:", nextInput)
			
			if strings.ToLower(nextInput) == "exit" || strings.Contains(strings.ToLower(allResponses), "설치가 완료") {
				fmt.Fprintln(multiLog, "=== Audit Completed ===")
				io.WriteString(stdin, "exit\n")
				break
			}

			io.WriteString(stdin, nextInput+"\n")
			history = append(history, llm.Message{Role: llm.RoleAssistant, Content: nextInput})
		}
	}

	cmd.Process.Kill()
	fmt.Fprintln(multiLog, "=== Audit Terminated ===")
}
