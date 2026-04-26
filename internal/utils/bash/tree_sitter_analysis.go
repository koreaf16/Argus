// Package bash — Tree-sitter analysis for bash security checks.
//
// 파일 역할: Provides tree-sitter-based bash AST analysis for security validation.
//
//	For Phase 1, this is a stub that returns empty results.
//
// 포함 모듈:
//   - TreeSitterAnalysisResult: Result structure for AST analysis
//   - AnalyzeBashCommand: Analyzes command AST for risks
//
// 호출/사용 방식:
//   - tools/bash security checks call AnalyzeBashCommand
//   - ast.go uses this for deep command analysis
//
// 연결:
//   - internal/utils/bash: bash_parser.go, ast.go
//   - 원본: src/utils/bash/ (tree-sitter integration, Phase 2)
package bash

import "strings"

// TreeSitterAnalysisResult contains results from AST-based bash analysis.
type TreeSitterAnalysisResult struct {
	HasCommandSubstitution bool
	HasProcessSubstitution bool
	HasEval                bool
	HasExec                bool
	HasSourceDot           bool
	HasPowerShellFeatures  bool
	ComplexityScore        int
	PotentialRisks         []string
}

// AnalyzeBashCommand analyzes a bash command AST for security issues.
// Phase 1 returns stub results; full implementation uses tree-sitter in Phase 2.
func AnalyzeBashCommand(source string) *TreeSitterAnalysisResult {
	result := &TreeSitterAnalysisResult{
		ComplexityScore: 1,
		PotentialRisks:  make([]string, 0),
	}

	// Phase 1: Stub analysis based on string matching
	// Phase 2: Use actual tree-sitter AST

	if contains(source, "$(") || contains(source, "`") {
		result.HasCommandSubstitution = true
	}

	if contains(source, "<(") {
		result.HasProcessSubstitution = true
	}

	if contains(source, "eval ") {
		result.HasEval = true
		result.PotentialRisks = append(result.PotentialRisks, "eval usage detected")
	}

	if contains(source, "exec ") {
		result.HasExec = true
	}

	if contains(source, "source ") || contains(source, ". ") {
		result.HasSourceDot = true
	}

	return result
}

// contains is a helper to check if source contains a substring.
func contains(source, substr string) bool {
	return strings.Contains(source, substr)
}
