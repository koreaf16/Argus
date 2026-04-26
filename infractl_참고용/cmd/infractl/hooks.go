// Package main
// File: hooks.go
// Description: infractl hooks 서브커맨드 dispatcher 및 핸들러
// Responsibility: hooks list/test/validate/reload 서브커맨드 라우팅

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yourorg/infractl/internal/config"
	"github.com/yourorg/infractl/internal/hooks"
)

// runHooks 는 "infractl hooks <sub>" 를 처리한다.
func runHooks(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: infractl hooks {list|test|validate|reload}\n\n" +
			"  list                     현재 등록된 hook 목록 출력\n" +
			"  test --event E --tool T --input JSON   hook 실행 시뮬레이션\n" +
			"  validate <path>          hooks.yaml 유효성 검증\n" +
			"  reload                   hooks.yaml 강제 재로드")
	}

	cfgDir, err := config.DefaultConfigDir()
	if err != nil {
		return fmt.Errorf("get config dir: %w", err)
	}
	hooksYAML := filepath.Join(cfgDir, "hooks.yaml")

	switch args[0] {
	case "list":
		return runHooksList(hooksYAML)
	case "test":
		return runHooksTest(hooksYAML, args[1:])
	case "validate":
		path := hooksYAML
		if len(args) > 1 {
			path = args[1]
		}
		return runHooksValidate(path)
	case "reload":
		return runHooksReload(hooksYAML)
	default:
		return fmt.Errorf("알 수 없는 hooks 서브커맨드: %s", args[0])
	}
}

func runHooksList(hooksYAML string) error {
	cfg, err := hooks.LoadConfig(hooksYAML)
	if err != nil {
		return fmt.Errorf("load hooks.yaml: %w", err)
	}

	if len(cfg.Events) == 0 {
		fmt.Println("등록된 hook 없음.")
		return nil
	}

	for event, matchers := range cfg.Events {
		fmt.Printf("[%s]\n", event)
		for _, m := range matchers {
			fmt.Printf("  matcher: %s\n", m.Matcher)
			for _, h := range m.Hooks {
				timeout := ""
				if h.Timeout > 0 {
					timeout = fmt.Sprintf(" timeout=%.0fs", h.Timeout)
				}
				fmt.Printf("    - type=%s%s\n", h.Type, timeout)
				switch h.Type {
				case hooks.BackendCommand:
					fmt.Printf("      command=%s\n", h.Command)
				case hooks.BackendHTTP:
					fmt.Printf("      url=%s method=%s\n", h.URL, h.Method)
				case hooks.BackendPrompt:
					fmt.Printf("      prompt=%s\n", h.Prompt)
				case hooks.BackendAgent:
					fmt.Printf("      agent=%s\n", h.Agent)
				}
			}
		}
	}
	return nil
}

func runHooksTest(hooksYAML string, args []string) error {
	var event, tool, inputJSON string
	for i := 0; i < len(args)-1; i++ {
		switch args[i] {
		case "--event":
			event = args[i+1]
			i++
		case "--tool":
			tool = args[i+1]
			i++
		case "--input":
			inputJSON = args[i+1]
			i++
		}
	}

	if event == "" || tool == "" {
		return fmt.Errorf("usage: infractl hooks test --event PreToolUse --tool Bash --input '{...}'")
	}

	// hooks.yaml 로드 + 스냅샷 세팅
	if err := hooks.Reload(hooksYAML); err != nil {
		fmt.Fprintf(os.Stderr, "hooks.yaml 로드 실패 (기존 정책 사용): %v\n", err)
	}

	var inputMap map[string]any
	if inputJSON != "" {
		if err := json.Unmarshal([]byte(inputJSON), &inputMap); err != nil {
			return fmt.Errorf("--input JSON 파싱 실패: %w", err)
		}
	}

	hookIn := hooks.HookInput{
		Event: hooks.HookEvent(event),
		Tool:  tool,
		Input: inputMap,
	}

	runner := hooks.NewRunner(nil)
	var out hooks.HookOutput
	if strings.EqualFold(event, "PreToolUse") {
		out = runner.RunPreToolUse(context.Background(), hookIn)
	} else {
		fmt.Printf("event=%s 는 test 에서 PostToolUse 로 실행됩니다\n", event)
		runner.RunPostToolUse(context.Background(), hookIn)
		fmt.Println("PostToolUse: fire-and-forget 완료")
		return nil
	}

	fmt.Printf("decision : %s\n", out.Decision)
	if out.Reason != "" {
		fmt.Printf("reason   : %s\n", out.Reason)
	}
	if out.SystemMessage != "" {
		fmt.Printf("message  : %s\n", out.SystemMessage)
	}
	return nil
}

func runHooksValidate(path string) error {
	cfg, err := hooks.LoadConfig(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "유효성 검증 실패: %v\n", err)
		os.Exit(1)
	}

	// 기본 검증: 이벤트 이름 확인
	validEvents := map[string]bool{
		"PreToolUse": true, "PostToolUse": true,
		"SessionStart": true, "SessionEnd": true,
	}
	var errs []string
	for event := range cfg.Events {
		if !validEvents[event] {
			errs = append(errs, fmt.Sprintf("알 수 없는 이벤트: %s", event))
		}
	}
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
		os.Exit(1)
	}

	fmt.Printf("OK: %s (이벤트 %d개)\n", path, len(cfg.Events))
	return nil
}

func runHooksReload(hooksYAML string) error {
	if err := hooks.Reload(hooksYAML); err != nil {
		return fmt.Errorf("reload 실패: %w", err)
	}
	fmt.Println("hooks.yaml 재로드 완료")
	return nil
}
