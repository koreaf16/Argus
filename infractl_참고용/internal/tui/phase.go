// Package tui
// File: phase.go
// Description: Phase 데이터 모델 및 도구 description에서 Phase 접두사 파싱
// Responsibility: Phase 기반 진행 트리의 핵심 타입과 파싱 로직 제공

package tui

import (
	"regexp"
)

// phase는 복잡한 작업의 명명된 단계 그룹이다.
type phase struct {
	id          string         // "A", "B", "C"
	description string         // "internal/cache/ 제네릭 LRU 캐시 구현"
	status      progressStatus // statusRunning, statusDone, statusError
	tools       []string       // 소속 toolID 목록
}

// phasePattern은 도구 description에서 Phase 접두사를 추출하는 정규식이다.
// 예: "Phase A: internal/cache/ 제네릭 LRU 캐시 구현"
var phasePattern = regexp.MustCompile(`^Phase\s+([A-Z])\s*:\s*(.+)$`)

// parsePhaseFromDescription은 도구 description에서 Phase 정보를 추출한다.
// 반환값: phaseID ("A"), description ("internal/cache/ ..."), ok (매칭 성공 여부)
func parsePhaseFromDescription(desc string) (phaseID string, description string, ok bool) {
	matches := phasePattern.FindStringSubmatch(desc)
	if len(matches) != 3 {
		return "", "", false
	}
	return matches[1], matches[2], true
}

// phaseDeclarationPattern은 LLM 응답에서 Phase 선언을 추출하는 정규식이다.
// 예: "Phase A: cache 구현", "Phase B: pipeline 마이그레이션"
var phaseDeclarationPattern = regexp.MustCompile(`(?m)^Phase\s+([A-Z])\s*:\s*(.+)$`)

// ParsePhaseDeclarations는 LLM 응답 텍스트에서 Phase 선언들을 추출한다.
func ParsePhaseDeclarations(text string) []phaseDeclaration {
	matches := phaseDeclarationPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	decls := make([]phaseDeclaration, 0, len(matches))
	seen := make(map[string]bool)
	for _, m := range matches {
		id := m[1]
		if seen[id] {
			continue
		}
		seen[id] = true
		decls = append(decls, phaseDeclaration{ID: id, Description: m[2]})
	}
	return decls
}

// phaseDeclaration은 LLM이 선언한 Phase 계획의 단일 항목이다.
type phaseDeclaration struct {
	ID          string
	Description string
}
