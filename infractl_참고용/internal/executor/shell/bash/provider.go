// Package bash
// File: provider.go
// Description: bash ShellProvider 구현 — mvdan.cc/sh/v3 기반
// Responsibility: bash 명령어 Wrap / Prepare / Analyze

// Ported from: claude_cli/src/utils/shell/bashProvider.ts

package bash

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/yourorg/infractl/internal/executor/shell"
	"github.com/yourorg/infractl/internal/executor/shell/bash/specs"
)

// Provider 는 bash ShellProvider 구현체이다.
type Provider struct{}

func init() { shell.Register(&Provider{}) }

// New 는 bash Provider 를 생성하고 전역 레지스트리에 등록한다.
func New() *Provider {
	p := &Provider{}
	shell.Register(p)
	return p
}

// Name 은 "bash" 를 반환한다.
func (p *Provider) Name() string { return "bash" }

// Wrap 은 Prepare 를 호출하고 argv 를 공백으로 합쳐 반환한다.
// 기존 호환성 shim — Phase E 완료 후 호출처를 Prepare 로 이전한다.
func (p *Provider) Wrap(ctx context.Context, command string) (string, error) {
	prepared, err := p.Prepare(ctx, command)
	if err != nil {
		return "", err
	}
	return strings.Join(prepared.Argv, " "), nil
}

// Prepare 는 bash 명령어를 exec.Command 용 PreparedCmd 로 변환한다.
// 비대화형 플래그 주입 → AST 분석 + snapshot 소싱 + set -f + pipe rearrange 순서.
// Ported from: claude_cli/src/utils/shell/bashProvider.ts:buildExecCommand
func (p *Provider) Prepare(ctx context.Context, command string) (shell.PreparedCmd, error) {
	command = applyAutoFlags(command)
	f, parseErr := Parse(ctx, command)

	// AST 분석 (파싱 실패 시 FAIL-CLOSED)
	var a shell.Analysis
	if parseErr != nil {
		a = shell.Analysis{
			RiskLevel:  shell.LevelUserApproval,
			RawCommand: command,
			ShellName:  "bash",
			Findings: []shell.Finding{{
				Node:   "Parse",
				Reason: "bash 파싱 실패: " + parseErr.Error(),
				Level:  shell.LevelUserApproval,
				Ported: "ast.ts:parseForSecurity",
			}},
		}
	} else {
		a = *CheckSemantics(ctx, command, f)
	}

	// 파이프 stdin 재배치 (heredoc/stdin 이미 있으면 스킵)
	arranged := command
	if f != nil {
		arranged = RearrangePipe(command, f)
	}

	// 환경 스냅샷 생성 (실패해도 계속 진행)
	snap, snapErr := CreateAndSave(ctx)

	var wrapped string
	var cleanups []func() error

	if snapErr == nil && snap != nil {
		wrapped = "source " + Quote(snap.Path) + "; set -f; " + arranged
		cleanups = append(cleanups, snap.Cleanup)
	} else {
		wrapped = "set -f; " + arranged
	}

	return shell.PreparedCmd{
		Argv:       []string{"bash", "-c", wrapped},
		Analysis:   a,
		CleanupFns: cleanups,
	}, nil
}

// applyAutoFlags 는 specs.Registry AutoFlags 를 커맨드 문자열에 정규식으로 주입한다.
// IfMissing 플래그가 이미 있으면 건너뛴다. executor.InjectNonInteractiveFlags 대체.
func applyAutoFlags(cmd string) string {
	for name, spec := range specs.Registry {
		for _, fs := range spec.AutoFlags {
			if !fs.IfMissing || strings.Contains(cmd, fs.Flag) {
				continue
			}
			re := regexp.MustCompile(fmt.Sprintf(
				`(?i)(^|[|;&])\s*(?:[\w./]*/)?(%s)(?:\s+|$)`,
				regexp.QuoteMeta(name),
			))
			if re.MatchString(cmd) {
				cmd = re.ReplaceAllString(cmd, fmt.Sprintf(`$1 $2 %s `, fs.Flag))
			}
		}
	}
	return strings.TrimSpace(cmd)
}

// Analyze 는 bash 명령어의 위험도를 AST 기반으로 분석한다.
// Ported from: claude_cli/src/utils/bash/ast.ts:parseForSecurity
func (p *Provider) Analyze(ctx context.Context, command string) (shell.Analysis, error) {
	f, err := Parse(ctx, command)
	if err != nil {
		// 파싱 실패 → FAIL-CLOSED
		return shell.Analysis{
			RiskLevel:  shell.LevelUserApproval,
			RawCommand: command,
			ShellName:  "bash",
			Findings: []shell.Finding{{
				Node:   "Parse",
				Reason: "bash 파싱 실패: " + err.Error(),
				Level:  shell.LevelUserApproval,
				Ported: "ast.ts:parseForSecurity",
			}},
		}, nil
	}
	a := CheckSemantics(ctx, command, f)
	return *a, nil
}
