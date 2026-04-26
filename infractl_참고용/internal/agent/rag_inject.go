// Package agent
// File: rag_inject.go
// Description: RAG 지식 주입 — 세션 시작 시 및 툴 실행 결과 후 동적 주입
// Responsibility: 사용자 입력 기반 사전 주입(prefetch)과 툴 결과 기반 사후 주입(post-tool) 두 가지 시점의 RAG 주입 관리

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/rag"
)

// buildKnowledgeContext injects scoped internal memory hits before the tool loop.
// Returns the formatted context string and the number of results injected.
// Only knowledge and learned_system entries are searched — never past session messages.
func buildKnowledgeContext(ctx context.Context, ragMgr *rag.Manager, userInput, activeServerName string) (string, int) {
	if ragMgr == nil || strings.TrimSpace(userInput) == "" {
		return "", 0
	}

	opts := rag.SearchOptions{
		TopK:        3,
		MinScore:    0.05,
		LocalOnly:   true,
		ServerName:  strings.TrimSpace(activeServerName),
		SourceTypes: []string{"knowledge", "learned_system"},
	}

	results, err := ragMgr.SearchWithOptions(ctx, userInput, opts)
	if err != nil {
		slog.Debug("pre-tool knowledge search failed", "err", err)
		return "", 0
	}
	if len(results) == 0 {
		return "", 0
	}

	var sb strings.Builder
	sb.WriteString("## 관련 내부 메모리\n")
	sb.WriteString("명령 실행 전에 아래 범위별 내부 메모리를 참고한다.\n")
	sb.WriteString("현재 대상과 세션 컨텍스트가 일치하는 경우에만 이전 해결책을 우선 적용한다.\n\n")

	for i, r := range results {
		sourceType := r.LocalSourceType
		if sourceType == "" {
			sourceType = "memory"
		}
		fmt.Fprintf(&sb, "**%d. [%s] %s** (score %.2f)\n", i+1, sourceType, r.Title, r.Score)
		sb.WriteString(truncateKnowledgeSnippet(r.Content, 600))
		sb.WriteString("\n\n")
	}

	return strings.TrimSpace(sb.String()), len(results)
}

// prefetchKnowledgeAsync는 사용자 입력에 대한 관련 내부 지식을 비동기로 검색한다.
// classification과 병렬로 실행하여 임베딩 검색 지연을 숨긴다.
// ragMgr가 nil이거나 입력이 비어 있으면 즉시 빈 문자열을 채널에 보낸다.
func prefetchKnowledgeAsync(ctx context.Context, ragMgr *rag.Manager, userInput, serverName string) <-chan string {
	ch := make(chan string, 1)
	if ragMgr == nil || strings.TrimSpace(userInput) == "" {
		ch <- ""
		return ch
	}
	go func() {
		snippet, _ := buildKnowledgeContext(ctx, ragMgr, userInput, serverName)
		ch <- snippet
	}()
	return ch
}

func truncateKnowledgeSnippet(content string, limit int) string {
	content = strings.TrimSpace(content)
	if limit <= 0 || len(content) <= limit {
		return content
	}
	return content[:limit] + "..."
}

// --- 툴 결과 기반 사후 RAG 주입 (Post-tool injection) ---

// oraErrRe는 Oracle ORA-NNNNN 에러 코드를 감지한다.
var oraErrRe = regexp.MustCompile(`ORA-\d{4,5}`)

// invalidColRe는 "invalid identifier" 에러에서 컬럼/오브젝트명을 추출한다.
// 예: ORA-00904: "CON_NAME": invalid identifier → CON_NAME
var invalidColRe = regexp.MustCompile(`ORA-00904:\s*"?([^":]+)"?\s*:\s*invalid identifier`)

// sqlTableRe는 FROM / JOIN 뒤의 테이블/뷰명을 추출한다.
var sqlTableRe = regexp.MustCompile(`(?i)(?:FROM|JOIN)\s+([\w$]+)`)

// hasToolErrors는 툴 결과 메시지 중 에러가 포함된 것이 있는지 확인한다.
func hasToolErrors(results []llm.Message) bool {
	for _, r := range results {
		c := r.Content
		if oraErrRe.MatchString(c) ||
			strings.Contains(c, "ERROR") ||
			strings.Contains(c, "Error") ||
			strings.Contains(c, "[ExitCode:") {
			return true
		}
	}
	return false
}

// buildPostToolRAGQuery는 툴콜과 그 결과로부터 RAG 검색 쿼리를 구성한다.
// 툴 이름, SQL 핵심 키워드, 에러 메시지를 조합한다.
func buildPostToolRAGQuery(toolCalls []llm.ToolCall, results []llm.Message) string {
	var parts []string

	for _, tc := range toolCalls {
		// 툴 이름에서 서비스 타입 추출 (oracle_ai26.query → oracle)
		name := tc.Function.Name
		parts = append(parts, name)

		// SQL 인자에서 핵심 오브젝트명 추출
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil {
			if sql, ok := args["sql"].(string); ok {
				for _, m := range sqlTableRe.FindAllStringSubmatch(sql, -1) {
					if len(m) > 1 {
						parts = append(parts, strings.ToLower(m[1]))
					}
				}
			}
		}
	}

	// 에러 메시지에서 핵심 키워드 추출
	for _, r := range results {
		if matches := oraErrRe.FindAllString(r.Content, -1); len(matches) > 0 {
			parts = append(parts, matches...)
		}
		// ORA-00904 컬럼명 추출
		if m := invalidColRe.FindStringSubmatch(r.Content); len(m) > 1 {
			parts = append(parts, strings.TrimSpace(m[1]))
		}
	}

	if len(parts) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(parts))
	unique := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, exists := seen[p]; exists {
			continue
		}
		seen[p] = struct{}{}
		unique = append(unique, p)
	}
	return strings.Join(unique, " ")
}

// buildColumnDiscoveryNote는 ORA-00904 컬럼 오류 발생 시
// LLM이 먼저 실제 컬럼 목록을 조회하도록 안내하는 메시지를 생성한다.
func buildColumnDiscoveryNote(toolCalls []llm.ToolCall, results []llm.Message) string {
	// ORA-00904 컬럼 오류가 없으면 빈 문자열 반환
	hasColError := false
	for _, r := range results {
		if strings.Contains(r.Content, "ORA-00904") {
			hasColError = true
			break
		}
	}
	if !hasColError {
		return ""
	}

	// 오류가 발생한 SQL에서 테이블/뷰명 수집
	var tables []string
	for _, tc := range toolCalls {
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			continue
		}
		sql, ok := args["sql"].(string)
		if !ok {
			continue
		}
		for _, m := range sqlTableRe.FindAllStringSubmatch(sql, -1) {
			if len(m) > 1 {
				tables = append(tables, strings.ToUpper(m[1]))
			}
		}
	}
	if len(tables) == 0 {
		return ""
	}

	// 중복 제거
	seen := make(map[string]struct{}, len(tables))
	unique := tables[:0]
	for _, t := range tables {
		if _, exists := seen[t]; exists {
			continue
		}
		seen[t] = struct{}{}
		unique = append(unique, t)
	}

	var sb strings.Builder
	sb.WriteString("[시스템: 컬럼 오류 복구]\n")
	sb.WriteString("ORA-00904 (invalid identifier) 발생 — 쿼리하기 전에 실제 컬럼 목록을 먼저 확인하세요.\n\n")
	sb.WriteString("아래 쿼리로 각 뷰의 실제 컬럼을 조회한 뒤 SQL을 재작성하세요:\n")
	for _, t := range unique {
		// SYS 소유 Dynamic Performance View는 V_ 접두어 뷰에 컬럼 정보가 있음
		viewName := t
		// V$ → V_ 변환 (all_tab_columns에는 V_$* 형태로 등록됨)
		if strings.HasPrefix(viewName, "V$") {
			viewName = "V_$" + viewName[2:]
		}
		fmt.Fprintf(&sb, "  SELECT column_name, data_type FROM all_tab_columns WHERE owner='SYS' AND table_name='%s' ORDER BY column_id;\n", viewName)
	}
	sb.WriteString("\n실제 컬럼이 확인된 후에만 쿼리를 재시도하세요. 같은 컬럼명으로 재시도하지 마세요.")
	return sb.String()
}

// InjectPostToolKnowledge는 툴 실행 결과를 기반으로 관련 지식을 히스토리에 주입한다.
// 에러가 있는 경우에만 실행되며, RAG 검색 결과와 컬럼 오류 안내를 조합한다.
// 주입은 user 역할 메시지로 삽입되어 다음 LLM 호출 시 컨텍스트로 활용된다.
func (a *Agent) InjectPostToolKnowledge(ctx context.Context, toolCalls []llm.ToolCall, results []llm.Message) {
	if !hasToolErrors(results) {
		return
	}

	var notes []string

	// 1. 컬럼 오류 안내 (즉각적이고 구체적 — 항상 우선)
	if colNote := buildColumnDiscoveryNote(toolCalls, results); colNote != "" {
		notes = append(notes, colNote)
	}

	// 2. RAG 검색 기반 지식 주입
	if a.ragManager != nil {
		query := buildPostToolRAGQuery(toolCalls, results)
		if query != "" {
			serverName := ""
			if a.activeServer != nil {
				serverName = a.activeServer.Name
			}
			opts := rag.SearchOptions{
				TopK:        2,
				MinScore:    0.10,
				LocalOnly:   true,
				ServerName:  serverName,
				SourceTypes: []string{"knowledge", "learned_system"},
			}
			ragResults, err := a.ragManager.SearchWithOptions(ctx, query, opts)
			if err != nil {
				slog.Debug("post-tool rag search failed", "err", err)
			} else if len(ragResults) > 0 {
				var sb strings.Builder
				sb.WriteString("[시스템: 관련 지식]\n")
				for _, r := range ragResults {
					fmt.Fprintf(&sb, "- %s: %s\n", r.Title, truncateKnowledgeSnippet(r.Content, 300))
				}
				notes = append(notes, strings.TrimSpace(sb.String()))
			}
		}
	}

	if len(notes) == 0 {
		return
	}

	combined := strings.Join(notes, "\n\n")
	const maxCombinedNoteLen = 5000
	if len(combined) > maxCombinedNoteLen {
		combined = combined[:maxCombinedNoteLen] + "\n[... additional knowledge truncated ...]"
	}
	slog.Debug("injecting post-tool knowledge", "length", len(combined))

	a.history = append(a.history, llm.Message{
		Role:    llm.RoleUser,
		Content: combined,
	})
}
