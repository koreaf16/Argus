// Package llm
// File: tier.go
// Description: LLM 티어 상수 및 폴백 순서 정의
// Responsibility: reasoning/general/fast 티어 열거형과 폴백 우선순위 제공

package llm

// Tier는 LLM 사용 목적에 따른 티어를 나타낸다.
type Tier string

const (
	// TierReasoning은 고차원 추론 작업용 티어이다 (복잡한 분석, 디버깅, 아키텍처 결정).
	TierReasoning Tier = "reasoning"
	// TierGeneral은 일반 작업용 티어이다. 에이전트 루프의 기본 두뇌 역할을 한다.
	TierGeneral Tier = "general"
	// TierFast는 단순/저비용 작업용 티어이다 (간단한 조회, 포맷팅, 상태 확인).
	TierFast Tier = "fast"
)

// FallbackOrder는 요청된 티어부터 시작하여 폴백 순서로 정렬된 티어 목록을 반환한다.
// 원칙: 요청 티어 → 능력이 높은 쪽 우선 → 낮은 쪽
//
//   - reasoning: reasoning → general → fast
//   - general:   general → reasoning → fast
//   - fast:      fast → general → reasoning
func FallbackOrder(requested Tier) []Tier {
	switch requested {
	case TierReasoning:
		return []Tier{TierReasoning, TierGeneral, TierFast}
	case TierFast:
		return []Tier{TierFast, TierGeneral, TierReasoning}
	default: // TierGeneral
		return []Tier{TierGeneral, TierReasoning, TierFast}
	}
}
