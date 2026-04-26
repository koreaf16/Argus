// Package powershell
// File: snapshot.go
// Description: PowerShell 환경 스냅샷 — $env:* + 현재 경로 캡처
// Responsibility: PSSnapshot — env/cwd 상태를 저장하고 임시 스크립트로 직렬화

// Ported from: claude_cli/src/utils/shell/powershellProvider.ts (snapshot pattern)

package powershell

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// PSSnapshot 은 캡처된 PowerShell 환경 상태를 담는다.
type PSSnapshot struct {
	// Path 는 임시 .ps1 스크립트 경로 (source 용)
	Path string
	// Env 는 캡처된 환경 변수 맵
	Env map[string]string
	// Cwd 는 캡처된 현재 작업 디렉토리
	Cwd string
}

// CreateAndSave 는 현재 환경을 캡처해 임시 .ps1 파일에 저장한다.
// Ported from: claude_cli/src/utils/shell/powershellProvider.ts
func CreateAndSave(ctx context.Context) (*PSSnapshot, error) {
	_ = ctx
	s := &PSSnapshot{Env: make(map[string]string)}

	for _, env := range os.Environ() {
		k, v, _ := strings.Cut(env, "=")
		if k != "" {
			s.Env[k] = v
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("ps snapshot cwd: %w", err)
	}
	s.Cwd = cwd

	f, err := os.CreateTemp("", "infractl-pssnap-*.ps1")
	if err != nil {
		return nil, fmt.Errorf("ps snapshot tempfile: %w", err)
	}
	if _, err := f.WriteString(buildPSScript(s)); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, fmt.Errorf("ps snapshot write: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return nil, fmt.Errorf("ps snapshot close: %w", err)
	}
	s.Path = f.Name()
	return s, nil
}

// Cleanup 은 스냅샷 임시 파일을 삭제한다.
func (s *PSSnapshot) Cleanup() error {
	if s.Path == "" {
		return nil
	}
	if err := os.Remove(s.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ps snapshot cleanup: %w", err)
	}
	return nil
}

func buildPSScript(s *PSSnapshot) string {
	var sb strings.Builder
	if s.Cwd != "" {
		sb.WriteString("Set-Location " + Quote(s.Cwd) + "\n")
	}
	for k, v := range s.Env {
		if isExportableKey(k) {
			sb.WriteString("$env:" + k + " = " + Quote(v) + "\n")
		}
	}
	return sb.String()
}

// isExportableKey 는 PowerShell 환경 변수 이름으로 안전한지 확인한다.
func isExportableKey(k string) bool {
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
