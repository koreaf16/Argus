// Package powershell
// File: provider.go
// Description: PowerShell ShellProvider 구현
// Responsibility: PowerShell 명령어 Wrap / Prepare / Analyze

// Ported from: claude_cli/src/utils/shell/powershellProvider.ts:27-124

package powershell

import (
	"context"
	"strings"

	"github.com/yourorg/infractl/internal/executor/shell"
	"github.com/yourorg/infractl/internal/executor/shell/bash/danger"
	"github.com/yourorg/infractl/internal/executor/shell/powershell/specs"
)

// Provider 는 PowerShell ShellProvider 구현체이다.
type Provider struct{}

func init() { shell.Register(&Provider{}) }

// New 는 PowerShell Provider 를 생성하고 전역 레지스트리에 등록한다.
func New() *Provider {
	p := &Provider{}
	shell.Register(p)
	return p
}

// Name 은 "powershell" 을 반환한다.
func (p *Provider) Name() string { return "powershell" }

// Wrap 은 Prepare 를 호출하고 argv 를 공백으로 합쳐 반환한다.
func (p *Provider) Wrap(ctx context.Context, command string) (string, error) {
	prepared, err := p.Prepare(ctx, command)
	if err != nil {
		return "", err
	}
	return strings.Join(prepared.Argv, " "), nil
}

// Prepare 는 PowerShell 스크립트를 -EncodedCommand UTF-16LE base64 로 변환한다.
// 한글·특수문자·따옴표 포함 스크립트를 안전하게 전달한다.
// Ported from: claude_cli/src/utils/shell/powershellProvider.ts:encodePowerShellCommand
func (p *Provider) Prepare(ctx context.Context, command string) (shell.PreparedCmd, error) {
	lvl, reason := specs.CheckDangerous(command)
	a := shell.Analysis{
		RiskLevel:  lvl,
		RawCommand: command,
		ShellName:  "powershell",
	}
	if lvl > shell.LevelLow {
		a.Findings = append(a.Findings, shell.Finding{
			Node:   "PSPattern",
			Reason: reason,
			Level:  lvl,
			Ported: "powershellProvider.ts (danger patterns)",
		})
	}

	// 스냅샷 생성 (실패해도 계속 진행)
	var scriptParts []string
	var cleanups []func() error
	scriptParts = append(scriptParts,
		"[Console]::OutputEncoding = [System.Text.Encoding]::UTF8",
		"$OutputEncoding = [System.Text.Encoding]::UTF8",
	)
	snap, snapErr := CreateAndSave(ctx)
	if snapErr == nil && snap != nil {
		scriptParts = append(scriptParts, ". "+Quote(snap.Path))
		cleanups = append(cleanups, snap.Cleanup)
	}
	scriptParts = append(scriptParts, command)

	script := strings.Join(scriptParts, "; ")
	encoded := EncodeUTF16LE(script)

	return shell.PreparedCmd{
		Argv:       []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encoded},
		Analysis:   a,
		CleanupFns: cleanups,
	}, nil
}

// Analyze 는 PowerShell 명령어의 위험도를 spec 패턴으로 분석한다.
// AST 분석 없이 패턴 매칭으로 위험도를 탐지하며, 미식별 명령은 LevelUserApproval.
// Ported from: claude_cli/src/utils/shell/powershellProvider.ts
func (p *Provider) Analyze(_ context.Context, command string) (shell.Analysis, error) {
	lvl, reason := specs.CheckDangerous(command)
	a := shell.Analysis{
		RiskLevel:   lvl,
		DangerScore: danger.ScoreFromLevel(lvl),
		IsReadOnly:  lvl == shell.LevelLow,
		RawCommand:  command,
		ShellName:   "powershell",
	}
	if lvl > shell.LevelLow {
		a.Findings = append(a.Findings, shell.Finding{
			Node:   "PSPattern",
			Reason: reason,
			Level:  lvl,
			Ported: "powershellProvider.ts (danger patterns)",
		})
	}
	return a, nil
}
