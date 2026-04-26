// Package pipeline
// File: preprocessor.go
// Description: 도구 실행 결과 전처리기 (서비스별 클리닝 + 스마트 트렁케이션)
// Responsibility: LLM에 전달되는 출력을 정제하고 크기를 제한하여 토큰 소비를 최소화

package pipeline

import "strings"

const (
	// DefaultMaxLLMBytes는 LLM에 전달하는 출력의 기본 소프트 한계이다.
	// 4KB에서 16KB로 상향: 포트 스캔 등 구조화된 출력이 잘려 LLM이 추측하는 환각을 방지한다.
	DefaultMaxLLMBytes = 16000
)

// ServiceType은 DB 서비스 종류를 나타낸다.
type ServiceType string

const (
	ServiceOracle     ServiceType = "oracle"
	ServiceMySQL      ServiceType = "mysql"
	ServicePostgreSQL ServiceType = "postgresql"
	ServiceGeneric    ServiceType = ""
)

// PreprocessorOption은 Preprocessor 생성 옵션이다.
type PreprocessorOption func(*Preprocessor)

// Preprocessor는 도구 실행 결과를 LLM 전달 전에 정제하는 전처리기이다.
type Preprocessor struct {
	maxLLMBytes int
	headLines   int
	tailLines   int
	serviceType ServiceType
}

// NewPreprocessor는 지정된 옵션으로 Preprocessor를 생성한다.
func NewPreprocessor(opts ...PreprocessorOption) *Preprocessor {
	p := &Preprocessor{
		maxLLMBytes: DefaultMaxLLMBytes,
		headLines:   DefaultHeadLines,
		tailLines:   DefaultTailLines,
		serviceType: ServiceGeneric,
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// WithMaxLLMBytes는 LLM 전달 출력의 최대 바이트 수를 설정한다.
func WithMaxLLMBytes(n int) PreprocessorOption {
	return func(p *Preprocessor) { p.maxLLMBytes = n }
}

// WithHeadTailLines는 스마트 트렁케이션의 head/tail 보존 라인 수를 설정한다.
func WithHeadTailLines(head, tail int) PreprocessorOption {
	return func(p *Preprocessor) {
		p.headLines = head
		p.tailLines = tail
	}
}

// WithServiceType은 DB 서비스 종류를 설정한다.
// 서비스 종류에 따라 적절한 노이즈 제거 함수가 적용된다.
func WithServiceType(svc ServiceType) PreprocessorOption {
	return func(p *Preprocessor) { p.serviceType = svc }
}

// Process는 원시 명령 출력을 정제하고 크기를 제한하여 반환한다.
// 처리 순서: 1) CLI 노이즈 제거 → 2) 스마트 트렁케이션
func (p *Preprocessor) Process(raw string) string {
	if raw == "" {
		return ""
	}

	// 1단계: 서비스별 CLI 노이즈 제거
	cleaned := p.cleanByService(raw)

	// 2단계: 이미 한계 이내면 그대로 반환
	if len(cleaned) <= p.maxLLMBytes {
		return cleaned
	}

	// 3단계: 스마트 트렁케이션
	return SmartTruncateString(cleaned, p.maxLLMBytes, p.headLines, p.tailLines)
}

// cleanByService는 서비스 종류에 맞는 노이즈 제거 함수를 적용한다.
func (p *Preprocessor) cleanByService(raw string) string {
	switch p.serviceType {
	case ServiceOracle:
		return CleanSQLPlusOutput(raw)
	case ServiceMySQL:
		return CleanMySQLOutput(raw)
	case ServicePostgreSQL:
		return CleanPSQLOutput(raw)
	default:
		return strings.TrimSpace(raw)
	}
}

// DefaultPreprocessor는 일반 용도 기본 Preprocessor이다.
var DefaultPreprocessor = NewPreprocessor()
