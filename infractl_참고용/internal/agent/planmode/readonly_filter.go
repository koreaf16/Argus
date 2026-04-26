// Package planmode
// File: readonly_filter.go
// Description: Plan Mode 중 mutation 도구 호출 차단 필터
// Responsibility: 도구 이름 + HookMetadata 로 read-only 여부 판단, mutation 이면 차단

// Ported from: claude_cli/src/services/tools/StreamingToolExecutor.ts:isConcurrencySafe (read-only 분류)
// 핵심 패턴: Tool.IsReadOnly() → 1차 판단, HookMetadata.ReadOnly/DiskModifying → 보조 힌트.
//            shell 계열은 metadata 기반(AST 분석 결과), 그 외는 Tool interface 위임.

package planmode

import (
	"github.com/yourorg/infractl/internal/hooks"
	"github.com/yourorg/infractl/internal/tools"
)

// ReadOnlyFilter 는 Plan Mode 활성 시 mutation 도구 호출을 차단한다.
type ReadOnlyFilter struct {
	registry *tools.Registry
}

// NewReadOnlyFilter 는 ReadOnlyFilter 를 생성한다.
func NewReadOnlyFilter(reg *tools.Registry) *ReadOnlyFilter {
	return &ReadOnlyFilter{registry: reg}
}

// Allow 는 주어진 도구 호출이 Plan Mode 에서 허용되는지 판단한다.
// allowed=true 이면 통과, false 이면 차단 + reason 문자열 포함.
func (f *ReadOnlyFilter) Allow(toolName string, md hooks.HookMetadata) (allowed bool, reason string) {
	// 1차: HookMetadata 기반 (AST 분석 결과 — 셸 계열에서 정확함)
	if md.ReadOnly {
		return true, ""
	}
	if md.DiskModifying {
		return false, "Plan Mode: 파일 시스템 쓰기 도구는 Plan Mode 에서 차단됩니다"
	}

	// 2차: Tool.IsReadOnly() 기반
	if f.registry != nil {
		if t, ok := f.registry.Get(toolName); ok {
			if t.IsReadOnly() {
				return true, ""
			}
			return false, "Plan Mode: mutation 도구는 Plan Mode 에서 차단됩니다"
		}
	}

	// 레지스트리에 없는 도구는 보수적으로 차단 (FAIL-CLOSED)
	return false, "Plan Mode: 알 수 없는 도구는 Plan Mode 에서 차단됩니다"
}
