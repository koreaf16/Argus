// Package constants — 시스템 프롬프트 섹션 타입 정의.
//
// 파일 역할: 시스템 프롬프트를 섹션 단위로 나누어 캐시 안정성을 관리하는
//
//	타입과 헬퍼 함수를 제공한다.
//	각 섹션은 이름·compute 함수·캐시 파괴 여부로 구성된다.
//
// 포함 모듈:
//   - SystemPromptSection: 섹션 타입.
//   - NewSection(): 캐시 유지 섹션 생성.
//   - NewUncachedSection(): 매 턴 재계산 섹션 생성.
//   - ResolveSystemPromptSections(): 모든 섹션을 평가해 문자열 슬라이스 반환.
//
// 호출/사용 방식:
//   - internal/query/context.go 에서 시스템 프롬프트 조립 시 사용.
//
// 연결:
//   - 원본: src/constants/systemPromptSections.ts
package constants

import "context"

// ComputeFn 은 섹션 내용을 동적으로 계산하는 함수 타입이다.
type ComputeFn func(ctx context.Context) (string, error)

// SystemPromptSection 은 이름과 compute 함수를 가진 시스템 프롬프트 구성 단위다.
type SystemPromptSection struct {
	Name       string
	Compute    ComputeFn
	CacheBreak bool // true 이면 값이 변경될 때 프롬프트 캐시가 무효화된다.
}

// NewSection 은 캐시를 유지하는 시스템 프롬프트 섹션을 생성한다.
func NewSection(name string, compute ComputeFn) SystemPromptSection {
	return SystemPromptSection{Name: name, Compute: compute, CacheBreak: false}
}

// NewUncachedSection 은 매 턴 재계산되는 섹션을 생성한다.
// 프롬프트 캐시를 파괴할 수 있으므로 꼭 필요한 경우에만 사용한다.
func NewUncachedSection(name string, compute ComputeFn, _ string) SystemPromptSection {
	return SystemPromptSection{Name: name, Compute: compute, CacheBreak: true}
}

// ResolveSystemPromptSections 는 모든 섹션을 평가해 문자열 슬라이스를 반환한다.
// 오류가 발생한 섹션은 빈 문자열로 처리한다.
func ResolveSystemPromptSections(ctx context.Context, sections []SystemPromptSection) []string {
	results := make([]string, 0, len(sections))
	for _, s := range sections {
		val, err := s.Compute(ctx)
		if err != nil || val == "" {
			continue
		}
		results = append(results, val)
	}
	return results
}
