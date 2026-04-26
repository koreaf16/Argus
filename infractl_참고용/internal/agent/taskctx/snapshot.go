// Package taskctx
// File: snapshot.go
// Description: TaskContext 의 불변 복사본 — 프롬프트/UI 렌더 및 cache key 용
// Responsibility: TaskContextSnapshot 생성, CacheKey 해시, RenderMarkdown 마크다운 렌더

package taskctx

import (
	"fmt"
	"hash/fnv"
	"maps"
	"strings"
	"time"
)

// TaskContextSnapshot 는 TaskContext 의 불변 복사본 — 프롬프트/UI 렌더 및 cache key 용.
type TaskContextSnapshot struct {
	ID            string
	Title         string
	Kind          TaskKind
	TargetServer  string
	TargetAccount string
	WorkingDir    string
	Forbidden     []string
	Preconditions []string
	PlanSteps     []string
	KindFields    map[string]string
	ElevationUser string // Elevation.CurrentUser (nil이면 "")
	ElevationMethod string
	Status        TaskStatus
	CreatedAt     time.Time
}

// CacheKey 는 prompt cache key에 포함할 해시 — fnv32 단축형.
func (s TaskContextSnapshot) CacheKey() string {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s|%s|%s|%s|%s|%s|%s|%s",
		s.ID, s.Title, s.TargetServer, s.TargetAccount,
		s.WorkingDir, s.ElevationUser, s.Status,
		strings.Join(s.Forbidden, ","),
	)
	return fmt.Sprintf("%08x", h.Sum32())
}

// RenderMarkdown 는 LLM 시스템 프롬프트에 삽입할 마크다운 텍스트 반환.
// Empty snapshot (ID=="") 은 빈 문자열 반환.
func (s TaskContextSnapshot) RenderMarkdown() string {
	if s.ID == "" {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("## [현재 작업 컨텍스트]\n")
	fmt.Fprintf(&sb, "- 작업: %s\n", s.Title)
	fmt.Fprintf(&sb, "- 유형: %s\n", string(s.Kind))
	fmt.Fprintf(&sb, "- 대상 서버: %s  /  대상 계정: %s  /  작업 디렉토리: %s\n",
		s.TargetServer, s.TargetAccount, s.WorkingDir)

	if len(s.Forbidden) > 0 {
		fmt.Fprintf(&sb, "- 금지 사항: %s\n", strings.Join(s.Forbidden, ", "))
	}

	if len(s.Preconditions) > 0 {
		sb.WriteString("- 사전조건:\n")
		for _, pc := range s.Preconditions {
			fmt.Fprintf(&sb, "  - %s\n", pc)
		}
	}

	if len(s.PlanSteps) > 0 {
		sb.WriteString("- 작업 단계:\n")
		for i, step := range s.PlanSteps {
			fmt.Fprintf(&sb, "  %d. %s\n", i+1, step)
		}
	}

	if s.ElevationUser != "" {
		sb.WriteString("\n## [권한 상태]\n")
		fmt.Fprintf(&sb, "- 현재 유저: **%s**\n", s.ElevationUser)
		if s.ElevationMethod != "" {
			fmt.Fprintf(&sb, "- 상승 방법: %s\n", s.ElevationMethod)
		}
		if s.ElevationUser == "root" {
			sb.WriteString("- 규칙: 현재 root이므로 후속 명령에 sudo를 붙이지 마라. 일반 명령만으로 root 권한 실행됨.\n")
		}
	}

	fmt.Fprintf(&sb, "\n_작업 ID: %s | 상태: %s_\n", s.ID, string(s.Status))

	return sb.String()
}

// snapshotFrom 은 TaskContext 에서 TaskContextSnapshot 을 생성하는 내부 헬퍼.
func snapshotFrom(tc *TaskContext) TaskContextSnapshot {
	if tc == nil {
		return TaskContextSnapshot{}
	}

	s := TaskContextSnapshot{
		ID:            tc.ID,
		Title:         tc.Title,
		Kind:          tc.Kind,
		TargetServer:  tc.TargetServer,
		TargetAccount: tc.TargetAccount,
		WorkingDir:    tc.WorkingDir,
		Status:        tc.Status,
		CreatedAt:     tc.CreatedAt,
	}

	if len(tc.Forbidden) > 0 {
		s.Forbidden = make([]string, len(tc.Forbidden))
		copy(s.Forbidden, tc.Forbidden)
	}

	if len(tc.Preconditions) > 0 {
		s.Preconditions = make([]string, len(tc.Preconditions))
		copy(s.Preconditions, tc.Preconditions)
	}

	if len(tc.PlanSteps) > 0 {
		s.PlanSteps = make([]string, len(tc.PlanSteps))
		copy(s.PlanSteps, tc.PlanSteps)
	}

	if tc.KindFields != nil {
		s.KindFields = make(map[string]string, len(tc.KindFields))
		maps.Copy(s.KindFields, tc.KindFields)
	}

	if tc.Elevation != nil {
		s.ElevationUser = tc.Elevation.CurrentUser
		s.ElevationMethod = string(tc.Elevation.Method)
	}

	return s
}
