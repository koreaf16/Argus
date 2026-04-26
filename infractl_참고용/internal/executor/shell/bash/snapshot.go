// Package bash
// File: snapshot.go
// Description: bash 셸 환경 스냅샷 캡처 및 저장
// Responsibility: CreateAndSave — env/cwd 캡처 → 임시 소스 스크립트 생성

// Ported from: claude_cli/src/utils/shell/ShellSnapshot.ts:createAndSaveSnapshot

package bash

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Snapshot 은 캡처된 셸 환경 상태를 담는다.
// Ported from: claude_cli/src/utils/shell/ShellSnapshot.ts:ShellSnapshot
type Snapshot struct {
	// Path 는 source 할 수 있는 임시 쉘 스크립트 경로
	Path string
	// Env 는 캡처된 환경 변수 맵
	Env map[string]string
	// Cwd 는 캡처된 현재 작업 디렉토리
	Cwd string
	// Umask 는 현재 umask 값 (예: "0022")
	Umask string
	// Aliases 는 캡처된 alias 목록 (alias 명령 출력)
	Aliases []string
	// Shopt 는 캡처된 shopt 상태 (shopt 명령 출력)
	Shopt []string
}

// CreateAndSave 는 현재 셸 환경을 캡처하고 임시 파일에 저장한다.
// 반환된 Snapshot 의 Cleanup 을 반드시 호출해 임시 파일을 정리해야 한다.
// Ported from: claude_cli/src/utils/shell/ShellSnapshot.ts:createAndSaveSnapshot
func CreateAndSave(ctx context.Context) (*Snapshot, error) {
	s := &Snapshot{Env: make(map[string]string)}

	// 현재 프로세스 환경 변수 캡처
	for _, env := range os.Environ() {
		k, v, _ := strings.Cut(env, "=")
		if k != "" {
			s.Env[k] = v
		}
	}

	// 현재 작업 디렉토리 캡처
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("snapshot cwd: %w", err)
	}
	s.Cwd = cwd

	// umask / alias / shopt 캡처 (bash 필요)
	if out, err := runBashCapture(ctx, "umask; echo '---ALIAS---'; alias; echo '---SHOPT---'; shopt"); err == nil {
		parts := strings.SplitN(out, "---ALIAS---", 2)
		if len(parts) >= 1 {
			s.Umask = strings.TrimSpace(parts[0])
		}
		if len(parts) >= 2 {
			aliasParts := strings.SplitN(parts[1], "---SHOPT---", 2)
			s.Aliases = parseLines(aliasParts[0])
			if len(aliasParts) >= 2 {
				s.Shopt = parseLines(aliasParts[1])
			}
		}
	}
	// bash 없이도 동작 (umask/alias/shopt 는 선택)

	// 스냅샷 스크립트 파일 생성
	f, err := os.CreateTemp("", "infractl-snap-*.sh")
	if err != nil {
		return nil, fmt.Errorf("snapshot tempfile: %w", err)
	}
	content := buildScript(s)
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, fmt.Errorf("snapshot write: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return nil, fmt.Errorf("snapshot close: %w", err)
	}
	s.Path = f.Name()
	return s, nil
}

// Cleanup 은 스냅샷 임시 파일을 삭제한다.
func (s *Snapshot) Cleanup() error {
	if s.Path == "" {
		return nil
	}
	if err := os.Remove(s.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("snapshot cleanup: %w", err)
	}
	return nil
}

// buildScript 는 Snapshot 을 bash source 가능한 스크립트 문자열로 변환한다.
func buildScript(s *Snapshot) string {
	var sb strings.Builder
	sb.WriteString("#!/usr/bin/env bash\n")

	if s.Cwd != "" {
		sb.WriteString("cd " + Quote(s.Cwd) + " || true\n")
	}
	if s.Umask != "" {
		sb.WriteString("umask " + s.Umask + "\n")
	}
	for k, v := range s.Env {
		if isExportableEnvKey(k) {
			sb.WriteString("export " + k + "=" + Quote(v) + "\n")
		}
	}
	// 에이전트 실행은 항상 비대화형 — pager가 stdin을 점유하지 않도록 강제 덮어쓰기
	sb.WriteString("export PAGER=cat\n")
	sb.WriteString("export SYSTEMD_PAGER=cat\n")
	sb.WriteString("export GIT_PAGER=cat\n")
	sb.WriteString("export LESS=\n")
	sb.WriteString("export MORE=\n")
	for _, a := range s.Aliases {
		if strings.TrimSpace(a) != "" {
			sb.WriteString(a + "\n")
		}
	}
	for _, opt := range s.Shopt {
		if strings.TrimSpace(opt) != "" {
			sb.WriteString("shopt -s " + strings.Fields(opt)[0] + " 2>/dev/null || true\n")
		}
	}
	return sb.String()
}

// isExportableEnvKey 는 스크립트에 안전하게 export 할 수 있는 변수 이름인지 확인한다.
func isExportableEnvKey(k string) bool {
	if k == "" {
		return false
	}
	for _, ch := range k {
		if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') ||
			(ch >= '0' && ch <= '9') || ch == '_') {
			return false
		}
	}
	return true
}

// runBashCapture 는 bash -c cmd 를 실행해 stdout 을 반환한다.
func runBashCapture(ctx context.Context, cmd string) (string, error) {
	out, err := exec.CommandContext(ctx, "bash", "-c", cmd).Output()
	if err != nil {
		return "", fmt.Errorf("bash capture: %w", err)
	}
	return string(out), nil
}

// parseLines 는 여러 줄 문자열을 비어 있지 않은 줄 목록으로 분리한다.
func parseLines(s string) []string {
	var result []string
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			result = append(result, t)
		}
	}
	return result
}
