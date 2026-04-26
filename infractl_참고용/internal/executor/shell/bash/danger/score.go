// Package danger
// File: score.go
// Description: 명령어 위험 점수 상수 및 Level ↔ Score 변환 유틸리티
// Responsibility: DangerScore 0..100 정의 및 shell.Level 변환

// Ported from: claude_cli/src/utils/bash/ast.ts (parseForSecurity 위험도 체계)

package danger

import "github.com/yourorg/infractl/internal/executor/shell"

const (
	// ScoreCritical 은 즉각·회복불가 파괴 명령 점수 (rm -rf /, fork bomb, mkfs, dd of=/dev/*)
	ScoreCritical = 100
	// ScoreHigh 는 고위험 명령 점수 (sudo, chmod 777, curl|sh, 미식별 CallExpr FAIL-CLOSED)
	ScoreHigh = 70
	// ScoreMedium 은 일반 mutation 명령 점수 (rm/mv/cp, apt install, git push)
	ScoreMedium = 40
	// ScoreLow 는 읽기 전용 화이트리스트 매칭 점수
	ScoreLow = 10
)

// ScoreFromLevel 은 Level 을 대표 점수로 변환한다.
// LevelUserApproval 은 FAIL-CLOSED 이므로 High 점수(70)에 대응한다.
// Ported from: claude_cli/src/utils/bash/ast.ts:parseForSecurity
func ScoreFromLevel(l shell.Level) int {
	switch l {
	case shell.LevelCritical:
		return ScoreCritical
	case shell.LevelHigh:
		return ScoreHigh
	case shell.LevelMedium:
		return ScoreMedium
	case shell.LevelLow:
		return ScoreLow
	default: // LevelUserApproval → High score (FAIL-CLOSED)
		return ScoreHigh
	}
}

// LevelFromScore 는 점수에서 Level 을 추론한다.
func LevelFromScore(score int) shell.Level {
	switch {
	case score >= ScoreCritical:
		return shell.LevelCritical
	case score >= ScoreHigh:
		return shell.LevelHigh
	case score >= ScoreMedium:
		return shell.LevelMedium
	case score > 0:
		return shell.LevelLow
	default:
		return shell.LevelUserApproval
	}
}

// levelPriority 는 위험도 집계용 우선순위를 반환한다.
// Critical 이 UserApproval 보다 높다 — 알려진 파괴적 명령이 미식별 명령보다 위험하다.
// Ported from: claude_cli/src/utils/bash/ast.ts (parseForSecurity 집계)
func levelPriority(l shell.Level) int {
	switch l {
	case shell.LevelCritical:
		return 5
	case shell.LevelHigh:
		return 4
	case shell.LevelUserApproval:
		return 3
	case shell.LevelMedium:
		return 2
	case shell.LevelLow:
		return 1
	default:
		return 0
	}
}

// MaxLevel 은 두 Level 중 우선순위가 높은(더 위험한) 것을 반환한다.
// Critical > High > UserApproval > Medium > Low
func MaxLevel(a, b shell.Level) shell.Level {
	if levelPriority(a) >= levelPriority(b) {
		return a
	}
	return b
}
