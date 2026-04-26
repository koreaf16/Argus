// Package main
// File: plan.go
// Description: infractl plan 서브커맨드 dispatcher 및 핸들러
// Responsibility: plan enter/exit/status 서브커맨드 라우팅

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/config"
)

// planStateFile 은 TUI 프로세스가 Plan Mode 상태를 기록하는 JSON 파일 경로를 반환한다.
func planStateFile() (string, error) {
	cfgDir, err := config.DefaultConfigDir()
	if err != nil {
		return "", fmt.Errorf("get config dir: %w", err)
	}
	return filepath.Join(cfgDir, "plan_state.json"), nil
}

// PlanStateJSON 은 plan_state.json 의 구조체다.
type PlanStateJSON struct {
	Active   bool              `json:"active"`
	Pending  []PendingCmdJSON  `json:"pending,omitempty"`
	UpdatedAt string           `json:"updated_at"`
}

// PendingCmdJSON 은 직렬화된 보류 명령이다.
type PendingCmdJSON struct {
	ID   string `json:"id"`
	Tool string `json:"tool"`
	At   string `json:"at"`
}

// runPlan 은 "infractl plan <sub>" 를 처리한다.
func runPlan(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: infractl plan {enter|exit|status}\n\n" +
			"  enter                    Plan Mode 진입 (TUI 세션에서 반영)\n" +
			"  exit [--approve IDs | --reject-all]   Plan Mode 종료 + 승인\n" +
			"  status                   현재 Plan Mode 상태 + 보류 명령 목록")
	}

	switch args[0] {
	case "enter":
		return runPlanEnter()
	case "exit":
		return runPlanExit(args[1:])
	case "status":
		return runPlanStatus()
	default:
		return fmt.Errorf("알 수 없는 plan 서브커맨드: %s", args[0])
	}
}

func runPlanEnter() error {
	path, err := planStateFile()
	if err != nil {
		return err
	}
	state := PlanStateJSON{
		Active:    true,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	return writePlanState(path, state)
}

func runPlanExit(args []string) error {
	path, err := planStateFile()
	if err != nil {
		return err
	}

	state, err := readPlanState(path)
	if err != nil {
		fmt.Println("Plan Mode 상태 파일 없음 — 이미 비활성화 상태입니다.")
		return nil
	}

	if !state.Active {
		fmt.Println("Plan Mode 가 활성화되어 있지 않습니다.")
		return nil
	}

	var rejectAll bool
	var approveIDs []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--reject-all":
			rejectAll = true
		case "--approve":
			if i+1 < len(args) {
				approveIDs = strings.Split(args[i+1], ",")
				i++
			}
		}
	}

	if rejectAll {
		fmt.Printf("Plan Mode 종료: %d 개 명령 모두 거부됨\n", len(state.Pending))
	} else if len(approveIDs) > 0 {
		fmt.Printf("Plan Mode 종료: %d 개 명령 중 %d 개 승인됨\n",
			len(state.Pending), len(approveIDs))
	} else {
		// 승인 옵션 없으면 모두 거부
		fmt.Printf("Plan Mode 종료: 보류 명령 %d 개 거부됨 (--approve 옵션 없음)\n",
			len(state.Pending))
	}

	state.Active = false
	state.Pending = nil
	state.UpdatedAt = time.Now().Format(time.RFC3339)
	return writePlanState(path, state)
}

func runPlanStatus() error {
	path, err := planStateFile()
	if err != nil {
		return err
	}

	state, err := readPlanState(path)
	if err != nil {
		fmt.Println("Plan Mode: 비활성 (상태 파일 없음)")
		return nil
	}

	if !state.Active {
		fmt.Println("Plan Mode: 비활성")
		return nil
	}

	fmt.Printf("Plan Mode: 활성 (갱신: %s)\n", state.UpdatedAt)
	if len(state.Pending) == 0 {
		fmt.Println("보류 명령: 없음")
		return nil
	}

	fmt.Printf("보류 명령: %d 개\n", len(state.Pending))
	for i, p := range state.Pending {
		fmt.Printf("  [%d] id=%s tool=%s at=%s\n", i+1, p.ID, p.Tool, p.At)
	}
	return nil
}

func readPlanState(path string) (PlanStateJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PlanStateJSON{}, fmt.Errorf("read plan state: %w", err)
	}
	var s PlanStateJSON
	if err := json.Unmarshal(data, &s); err != nil {
		return PlanStateJSON{}, fmt.Errorf("parse plan state: %w", err)
	}
	return s, nil
}

func writePlanState(path string, s PlanStateJSON) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plan state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write plan state: %w", err)
	}
	fmt.Printf("plan_state.json 갱신: %s\n", path)
	return nil
}
