// Package cost
// File: types.go
// Description: 비용/사용량 추적 도메인 타입 정의
// Responsibility: UsageEntry, ModelPricing, Source 상수, 집계 결과 타입 제공

package cost

import "time"

// Source는 LLM 호출의 발생 원천을 나타낸다.
type Source string

const (
	SourceUser     Source = "user"
	SourceSubagent Source = "subagent"
	SourceSchedule Source = "schedule"
	SourceAutoMode Source = "auto_mode"
	SourceClassify Source = "classify"
)

// ModelPricing은 모델별 100만 토큰 단가(USD)이다.
type ModelPricing struct {
	Input  float64 `yaml:"input"`
	Output float64 `yaml:"output"`
}

// UsageEntry는 LLM 호출 1회의 사용량 기록이다.
type UsageEntry struct {
	ID            int64
	Model         string
	InputTokens   int
	OutputTokens  int
	EstimatedCost float64
	Source        Source
	SessionID     int64
	Timestamp     time.Time
}

// CostSummary는 특정 기간의 비용 집계 결과이다.
type CostSummary struct {
	TotalInputTokens  int
	TotalOutputTokens int
	TotalCost         float64
	CallCount         int
	Period            string
}

// DailyCost는 하루치 비용 집계이다.
type DailyCost struct {
	Date          string
	InputTokens   int
	OutputTokens  int
	EstimatedCost float64
	CallCount     int
}
