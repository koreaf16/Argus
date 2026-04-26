// Package tui
// File: shimmer_label.go
// Description: LLM 티어와 모델명으로 shimmer 표시 텍스트를 생성
// Responsibility: tier+model 조합을 사용자 친화적 shimmer 레이블로 변환

package tui

import "strings"

// ThinkingLabel은 LLM 티어와 모델명으로 shimmer 텍스트를 생성한다.
//
//	reasoning → "reasoning [model]..."
//	general   → "thinking [model]..."
//	fast      → "quick check [model]..."
func ThinkingLabel(tier, model string) string {
	prefix := "thinking"
	switch tier {
	case "reasoning":
		prefix = "reasoning"
	case "fast":
		prefix = "quick check"
	}

	short := shortenModel(model)
	if short != "" {
		return prefix + " [" + short + "]..."
	}
	return prefix + "..."
}

// shortenModel은 모델명에서 날짜 접미사 등을 제거하여 축약한다.
// 예: "qwen3-235b-a22b-instruct-20250401" → "qwen3-235b-a22b-instruct"
//
//	"claude-sonnet-4-20250514"          → "claude-sonnet-4"
func shortenModel(model string) string {
	if model == "" {
		return ""
	}
	// 마지막 "-YYYYMMDD" 패턴 제거
	parts := strings.Split(model, "-")
	if len(parts) > 1 {
		last := parts[len(parts)-1]
		if len(last) == 8 && isAllDigits(last) {
			return strings.Join(parts[:len(parts)-1], "-")
		}
	}
	return model
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
