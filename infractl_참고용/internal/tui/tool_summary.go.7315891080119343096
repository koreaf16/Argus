// Package tui
// File: tool_summary.go
// Description: 도구 실행 결과 요약 생성 및 본문 미리보기 제한
// Responsibility: Claude CLI 스타일 결과 요약 ("⎿ Wrote N lines") 및 줄 수 제한

package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/diff"
)

const maxPreviewLines = 10 // 결과 본문 기본 미리보기 줄 수

// toolSummary는 도구별 결과 요약 한 줄을 생성한다.
// 형식: "  ⎿  Done (1.2s)" 또는 "  ⎿  Wrote 84 lines to statusbar.go (0.3s)"
func toolSummary(toolName string, args map[string]any, result string, metadataJSON string, duration time.Duration, success bool) string {
	prefix := StyleResponseBracket.Render("  ⎿  ")
	dur := StyleCmdBoxDim.Render(" (" + formatElapsedShort(duration) + ")")

	switch toolName {
	case "file_write":
		return prefix + summaryFileWrite(args, result) + dur
	case "file_read":
		return prefix + summaryFileRead(args, result) + dur
	case "shell_exec":
		return prefix + summaryShellExec(args, result, metadataJSON, success) + dur
	case "verify_complete":
		if summary := taskProgressSummaryLine(toolName, args, metadataJSON, success, false); summary != "" {
			return prefix + summary + dur
		}
		if success {
			return prefix + "검증 완료" + dur
		}
		return prefix + "검증 실패" + dur
	case "web_fetch":
		return prefix + summaryWebFetch(result) + dur
	case "web_search":
		return prefix + summaryWebSearch(args, result) + dur
	case "knowledge_search":
		return prefix + summarySearch(result) + dur
	default:
		// 일반 도구: 결과 첫 줄 요약
		first := firstLine(result)
		if len(first) > 80 {
			first = first[:77] + "..."
		}
		if first != "" {
			return prefix + StyleCmdBoxDim.Render(first) + dur
		}
		doneLabel := StyleSuccess.Render("Done")
		if !success {
			doneLabel = StyleError.Render("Failed")
		}
		return prefix + doneLabel + dur
	}
}

func summaryFileWrite(args map[string]any, result string) string {
	path := argString(args, "path")
	added, removed := countDiffLines(result)
	if added > 0 || removed > 0 {
		parts := []string{}
		if added > 0 {
			parts = append(parts, StyleSuccess.Render(fmt.Sprintf("+%d lines", added)))
		}
		if removed > 0 {
			parts = append(parts, StyleError.Render(fmt.Sprintf("-%d lines", removed)))
		}
		return fmt.Sprintf("Wrote %s to %s", strings.Join(parts, ", "), shortPath(path))
	}
	lines := countLines(result)
	return fmt.Sprintf("Wrote %d lines to %s", lines, shortPath(path))
}

func summaryFileRead(args map[string]any, result string) string {
	path := argString(args, "path")
	lines := countLines(result)
	return fmt.Sprintf("Read %d lines from %s", lines, shortPath(path))
}

func summaryShellExec(args map[string]any, result string, metadataJSON string, success bool) string {
	if summary := taskProgressSummaryLine("shell_exec", args, metadataJSON, success, false); summary != "" {
		return summary
	}

	lines := len(shellResultOutputLines("shell_exec", result))
	if success {
		return fmt.Sprintf("Shell command complete (%d lines)", lines)
	}
	return fmt.Sprintf("Shell command failed (%d lines)", lines)
}

func summaryWebFetch(result string) string {
	size := len(result)
	if size > 1024*1024 {
		return fmt.Sprintf("Received %.1fMB", float64(size)/(1024*1024))
	}
	if size > 1024 {
		return fmt.Sprintf("Received %.1fKB", float64(size)/1024)
	}
	return fmt.Sprintf("Received %d bytes", size)
}

func summaryWebSearch(args map[string]any, result string) string {
	query := argString(args, "query")
	sources := countWebSearchSources(result)
	if query != "" {
		if sources > 0 {
			return fmt.Sprintf("Searched \"%s\" — %d sources fetched", truncateStr(query, 40), sources)
		}
		return fmt.Sprintf("Searched \"%s\"", truncateStr(query, 40))
	}
	if sources > 0 {
		return fmt.Sprintf("Search complete — %d sources fetched", sources)
	}
	return "Search complete"
}

// countWebSearchSources는 renderWebResearchOutput 결과에서 "Sources:" 섹션의 항목 수를 센다.
func countWebSearchSources(result string) int {
	inSources := false
	count := 0
	for _, line := range strings.Split(result, "\n") {
		if strings.TrimSpace(line) == "Sources:" {
			inSources = true
			continue
		}
		if inSources {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "- ") {
				count++
			} else if strings.HasPrefix(trimmed, "Warnings:") || strings.HasPrefix(trimmed, "Warning:") {
				break
			}
		}
	}
	return count
}

// webSearchSourcesLine은 박스 내부에 표시할 소스 개수 요약 1줄을 반환한다.
// Engine/Keywords는 renderWebSearchBox가 별도로 표시하므로 개수만 반환한다.
func webSearchSourcesLine(result string) string {
	n := countWebSearchSources(result)
	if n > 0 {
		return fmt.Sprintf("%d sources fetched", n)
	}
	return "No results"
}

func summarySearch(result string) string {
	lines := countLines(result)
	return fmt.Sprintf("Found %d matches", lines)
}

// toolBoxContent는 도구 결과 박스 내부에 표시할 줄들을 반환한다.
// shell_exec / file_transfer처럼 스트리밍 출력이 있는 도구는 호출자가 캡처된 라인을 전달한다.
// 그 외 도구는 결과에서 상세 내용 또는 요약을 생성한다.
func toolBoxContent(name string, args map[string]any, result string, success bool) []string {
	if result == "" {
		if success {
			return []string{StyleSuccess.Render("Done")}
		}
		return []string{StyleError.Render("Failed")}
	}

	switch name {
	case "shell_exec", "file_transfer":
		return nil // 호출자가 스트리밍 캡처 라인을 제공한다
	case "system_info", "process_list", "network_info", "disk_usage", "service_status", "k8s_query":
		return strings.Split(strings.TrimRight(result, "\r\n"), "\n")
	case "file_write":
		return []string{summaryFileWrite(args, result)}
	case "file_read":
		return []string{summaryFileRead(args, result)}
	case "web_fetch":
		return []string{summaryWebFetch(result)}
	case "web_search":
		return []string{webSearchSourcesLine(result)}
	case "knowledge_search":
		return []string{summarySearch(result)}
	default:
		// 여러 줄인 경우 상세 내용 반환 (renderShellBoxContent에서 10줄로 제한됨)
		trimmed := strings.TrimRight(result, "\r\n")
		if strings.Contains(trimmed, "\n") {
			return strings.Split(trimmed, "\n")
		}

		// 한 줄인 경우 요약 또는 그대로 반환
		first := firstLine(trimmed)
		if len(first) > 80 {
			first = first[:77] + "..."
		}
		if first != "" {
			return []string{StyleCmdBoxDim.Render(first)}
		}
		if success {
			return []string{StyleSuccess.Render("Done")}
		}
		return []string{StyleError.Render("Failed")}
	}
}

// truncateResult는 결과를 maxPreviewLines로 제한하고 초과분을 힌트로 표시한다.
func truncateResult(rendered string) string {
	lines := strings.Split(rendered, "\n")
	if len(lines) <= maxPreviewLines {
		return rendered
	}
	preview := strings.Join(lines[:maxPreviewLines], "\n")
	remaining := len(lines) - maxPreviewLines
	hint := StyleCmdBoxDim.Render(fmt.Sprintf("  … +%d lines", remaining))
	return preview + "\n" + hint
}

// countDiffLines는 diff에서 추가/삭제 줄 수를 센다.
func countDiffLines(result string) (added, removed int) {
	for _, line := range strings.Split(result, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	// diff가 아닌 경우 0, 0 반환
	if !diff.IsDiff(result) {
		return 0, 0
	}
	return added, removed
}

// countLines는 텍스트의 줄 수를 반환한다.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// argString은 args 맵에서 문자열 값을 추출한다.
func argString(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// shortPath는 긴 경로를 파일명 또는 마지막 2세그먼트로 축약한다.
func shortPath(path string) string {
	if len(path) <= 40 {
		return path
	}
	// 마지막 / 또는 \ 이후의 파일명
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return "..." + path[i:]
		}
	}
	return path
}

// firstLine은 텍스트의 첫 번째 줄을 반환한다.
func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
