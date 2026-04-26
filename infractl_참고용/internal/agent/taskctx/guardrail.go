// Package taskctx
// File: guardrail.go
// Description: Forbidden 패턴 컴파일 및 도구 호출 평가
// Responsibility: 금지 패턴 컴파일(CompileForbidden)과 GuardVerdict 판정(Evaluate)

package taskctx

import (
	"fmt"
	"regexp"
	"strings"
)

// GuardVerdict 는 도구 호출 평가 결과이다.
type GuardVerdict struct {
	Blocked bool   // Forbidden 매칭
	Warned  bool   // 서버/계정/디렉토리 불일치
	Reason  string // 사람 읽기용
}

// Guardrails 는 Forbidden 패턴을 컴파일된 형태로 보관한다.
type Guardrails struct {
	forbiddenRegex   []*regexp.Regexp
	forbiddenLiteral []string // prefix 매칭
}

// CompileForbidden 는 []string 을 받아 "^/" 로 시작하면 regexp.Compile,
// 아니면 literal prefix 로 저장한다.
// 컴파일 실패 시 에러 반환 (fail-closed).
func CompileForbidden(patterns []string) (*Guardrails, error) {
	g := &Guardrails{}
	for _, p := range patterns {
		if strings.HasPrefix(p, "^/") || (len(p) > 0 && p[0] == '^') {
			re, err := regexp.Compile(p)
			if err != nil {
				return nil, fmt.Errorf("compile forbidden pattern %q: %w", p, err)
			}
			g.forbiddenRegex = append(g.forbiddenRegex, re)
		} else {
			g.forbiddenLiteral = append(g.forbiddenLiteral, p)
		}
	}
	return g, nil
}

// Evaluate 는 TaskContext 가 있을 때 도구 호출을 검사한다.
// task: 현재 TaskContext (nil이면 모두 통과)
// elevationUser: 현재 elevation.CurrentUser (비어있으면 TargetAccount 비교 안함)
// toolName: 도구 이름
// args: 도구 arguments (map[string]any)
// target: 이번 도구가 향하는 서버 (shell_exec의 server 인자 등)
func (g *Guardrails) Evaluate(
	task *TaskContext,
	elevationUser string,
	toolName string,
	args map[string]any,
	target string,
) GuardVerdict {
	if task == nil {
		return GuardVerdict{}
	}

	// 명령어 추출
	cmd := extractCommand(args)

	// Forbidden 검사 (Blocked 우선)
	if cmd != "" {
		for _, re := range g.forbiddenRegex {
			if re.MatchString(cmd) {
				return GuardVerdict{
					Blocked: true,
					Reason:  fmt.Sprintf("Forbidden 매칭: %s", re.String()),
				}
			}
		}
		for _, lit := range g.forbiddenLiteral {
			if strings.HasPrefix(cmd, lit) {
				return GuardVerdict{
					Blocked: true,
					Reason:  fmt.Sprintf("Forbidden 매칭: %s", lit),
				}
			}
		}
	}

	// Warn 검사 (여러 건 누적)
	var warns []string

	if task.TargetServer != "" && target != "" && target != task.TargetServer {
		warns = append(warns, "대상 서버 불일치")
	}

	if task.TargetAccount != "" &&
		elevationUser != "" &&
		elevationUser != task.TargetAccount &&
		elevationUser != "root" {
		warns = append(warns, "대상 계정 불일치")
	}

	if task.WorkingDir != "" {
		if cwd, ok := args["cwd"].(string); ok && cwd != "" && cwd != task.WorkingDir {
			warns = append(warns, "작업 디렉토리 불일치")
		}
	}

	if len(warns) > 0 {
		return GuardVerdict{
			Warned: true,
			Reason: strings.Join(warns, "; "),
		}
	}

	return GuardVerdict{}
}

// extractCommand 는 args에서 명령어 문자열을 추출한다.
func extractCommand(args map[string]any) string {
	if args == nil {
		return ""
	}
	if v, ok := args["command"].(string); ok {
		return v
	}
	if v, ok := args["cmd"].(string); ok {
		return v
	}
	return ""
}
