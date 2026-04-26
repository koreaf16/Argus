// Package bash — Bash parsing and AST construction.
//
// 파일 역할: Pure Go implementation of bash parser producing tree-sitter-bash-compatible ASTs.
//
//	For Phase 1, this is a simplified tokenizer-based parser (not full grammar).
//
// 포함 모듈:
//   - TsNode: tree-sitter-bash compatible AST node type
//   - parseSource: Tokenizes and parses bash source into TsNode tree
//   - ensureParserInitialized: No-op for compatibility
//   - getParserModule: Stub parser module getter
//
// 호출/사용 방식:
//   - parser.go, ast.go call parseSource(src, timeoutMs) to parse bash commands
//   - AnalyzeBashCommand in tree_sitter_analysis.go uses this
//
// 연결:
//   - internal/utils/bash: ParsedCommand, AST analysis, security checks
//   - 원본: src/utils/bash/bashParser.ts
package bash

import (
	"strings"
	"time"
)

// TsNode represents a tree-sitter-bash compatible AST node.
type TsNode struct {
	Type       string    `json:"type"`
	Text       string    `json:"text"`
	StartIndex int       `json:"startIndex"`
	EndIndex   int       `json:"endIndex"`
	Children   []*TsNode `json:"children"`
}

const parseTimeoutMs = 50
const maxNodes = 50000

var shellKeywords = map[string]bool{
	"if":       true,
	"then":     true,
	"elif":     true,
	"else":     true,
	"fi":       true,
	"while":    true,
	"until":    true,
	"for":      true,
	"in":       true,
	"do":       true,
	"done":     true,
	"case":     true,
	"esac":     true,
	"function": true,
	"{":        true,
	"}":        true,
	"!":        true,
	"[[":       true,
	"]]":       true,
	"[":        true,
	"]":        true,
	"return":   true,
	"break":    true,
	"continue": true,
	"exit":     true,
}

// EnsureParserInitialized is a no-op for Phase 1.
func EnsureParserInitialized() error {
	return nil
}

// GetParserModule returns a stub parser module for compatibility.
func GetParserModule() interface{} {
	return &parserModule{parse: parseSource}
}

type parserModule struct {
	parse func(source string, timeoutMs int) *TsNode
}

// ParseSource parses bash source code into a simplified TsNode tree.
// For Phase 1, this performs basic tokenization without full grammar validation.
// timeoutMs of 0 uses the default (50ms); pass any positive value to set custom timeout.
func parseSource(source string, timeoutMs int) *TsNode {
	if timeoutMs <= 0 {
		timeoutMs = parseTimeoutMs
	}

	// Simple timeout implementation using a time check
	startTime := time.Now()
	timeoutDur := time.Duration(timeoutMs) * time.Millisecond

	// Phase 1: Very simplified tokenizer. Full tree-sitter grammar is Phase 2.
	// For now, split on obvious delimiters and build a shallow tree.
	root := &TsNode{
		Type:     "program",
		Text:     source,
		Children: make([]*TsNode, 0),
	}

	// Check timeout
	if time.Since(startTime) > timeoutDur {
		return root // Return partial tree on timeout
	}

	// Split by semicolons and newlines into statements
	lines := strings.Split(source, "\n")
	offset := 0

	for _, line := range lines {
		if time.Since(startTime) > timeoutDur {
			break
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			offset += len(line) + 1
			continue
		}

		// Create a simple command node
		cmdNode := &TsNode{
			Type:       "command",
			Text:       trimmed,
			StartIndex: offset,
			EndIndex:   offset + len(trimmed),
			Children:   make([]*TsNode, 0),
		}

		// Very basic tokenization: split on spaces (not respecting quotes for Phase 1)
		tokens := strings.Fields(trimmed)
		for i, tok := range tokens {
			tokNode := &TsNode{
				Type:       "word",
				Text:       tok,
				StartIndex: offset + i,
				EndIndex:   offset + i + len(tok),
				Children:   make([]*TsNode, 0),
			}
			cmdNode.Children = append(cmdNode.Children, tokNode)
		}

		root.Children = append(root.Children, cmdNode)
		offset += len(line) + 1
	}

	return root
}
