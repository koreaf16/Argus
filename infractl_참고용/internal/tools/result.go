// Package tools
// File: result.go
// Description: 구조화된 tool 실행 결과 타입
// Responsibility: artifact/input-request를 포함한 tool 결과를 표현

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

// ToolResultStatus는 구조화된 tool 결과의 상태를 나타낸다.
type ToolResultStatus string

const (
	ToolResultOK         ToolResultStatus = "ok"
	ToolResultNeedsInput ToolResultStatus = "needs_input"
	ToolResultError      ToolResultStatus = "error"
)

// ArtifactRef는 tool이 생성하거나 참조한 파일/산출물을 나타낸다.
type ArtifactRef struct {
	Kind        string `json:"kind"`
	Name        string `json:"name,omitempty"`
	Path        string `json:"path,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Description string `json:"description,omitempty"`
}

// InputQuestion는 user input이 필요한 질문 하나를 나타낸다.
type InputQuestion struct {
	ID          string   `json:"id,omitempty"`
	Header      string   `json:"header,omitempty"`
	Question    string   `json:"question,omitempty"`
	Choices     []string `json:"choices,omitempty"`
	Default     string   `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
}

// InputRequest는 tool 실행에 필요한 질문 묶음이다.
type InputRequest struct {
	Title     string          `json:"title,omitempty"`
	Prompt    string          `json:"prompt,omitempty"`
	Questions []InputQuestion `json:"questions,omitempty"`
	Notes     []string        `json:"notes,omitempty"`
}

// ToolResult는 tool 실행 결과를 구조화해서 보관한다.
type ToolResult struct {
	Status       ToolResultStatus `json:"status"`
	Summary      string           `json:"summary,omitempty"`
	Body         string           `json:"body,omitempty"`
	Metadata     map[string]any   `json:"metadata,omitempty"`
	Artifacts    []ArtifactRef    `json:"artifacts,omitempty"`
	InputRequest *InputRequest    `json:"input_request,omitempty"`
}

// ResultTool은 구조화된 결과를 반환하는 tool을 위한 선택적 인터페이스다.
type ResultTool interface {
	Tool
	ExecuteResult(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolResult, error)
}

// DisplayString은 LLM/TUI에 보여줄 문자열을 생성한다.
func (r ToolResult) DisplayString() string {
	var parts []string

	status := strings.TrimSpace(string(r.Status))
	if status == "" {
		status = string(ToolResultOK)
	}
	if status != string(ToolResultOK) {
		parts = append(parts, strings.ToUpper(status))
	}

	if s := strings.TrimSpace(r.Summary); s != "" {
		parts = append(parts, s)
	}
	if b := strings.TrimSpace(r.Body); b != "" {
		parts = append(parts, b)
	}
	if len(r.Artifacts) > 0 {
		var artifactLines []string
		artifactLines = append(artifactLines, "Artifacts:")
		for _, a := range r.Artifacts {
			label := a.Kind
			if label == "" {
				label = "artifact"
			}
			line := fmt.Sprintf("- %s", label)
			if a.Name != "" {
				line += fmt.Sprintf(" %s", a.Name)
			}
			if a.Path != "" {
				line += fmt.Sprintf(" @ %s", a.Path)
			}
			if a.Description != "" {
				line += fmt.Sprintf(" (%s)", a.Description)
			}
			artifactLines = append(artifactLines, line)
		}
		parts = append(parts, strings.Join(artifactLines, "\n"))
	}
	if r.InputRequest != nil {
		var inputLines []string
		title := strings.TrimSpace(r.InputRequest.Title)
		if title == "" {
			title = "Input required"
		}
		inputLines = append(inputLines, title)
		if prompt := strings.TrimSpace(r.InputRequest.Prompt); prompt != "" {
			inputLines = append(inputLines, prompt)
		}
		for _, q := range r.InputRequest.Questions {
			line := q.Question
			if q.Header != "" {
				line = fmt.Sprintf("%s: %s", q.Header, line)
			}
			if len(q.Choices) > 0 {
				line += fmt.Sprintf(" [choices: %s]", strings.Join(q.Choices, " | "))
			}
			if q.Default != "" {
				line += fmt.Sprintf(" [default: %s]", q.Default)
			}
			inputLines = append(inputLines, "- "+line)
		}
		parts = append(parts, strings.Join(inputLines, "\n"))
	}

	if len(parts) == 0 {
		return "(no output)"
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

// JSONString은 ToolResult를 JSON 문자열로 직렬화한다.
func (r ToolResult) JSONString() string {
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"status":"error","summary":"failed to marshal tool result: %s"}`, err)
	}
	return string(b)
}

// OKResult는 기본 성공 결과를 만든다.
func OKResult(summary, body string) ToolResult {
	return ToolResult{
		Status:  ToolResultOK,
		Summary: summary,
		Body:    body,
	}
}

// NeedsInputResult는 user input이 필요한 결과를 만든다.
func NeedsInputResult(summary, body string, req *InputRequest) ToolResult {
	return ToolResult{
		Status:       ToolResultNeedsInput,
		Summary:      summary,
		Body:         body,
		InputRequest: req,
	}
}
