// Package agent
// File: session_summary.go
// Description: 대화 히스토리의 프로그레시브 요약 관리자 — 오래된 라운드를 점진적으로 요약하여 컨텍스트 노이즈 감소
// Responsibility: 콘텐츠 유형 분류, 문서/설정 원문 보존, command/chat 요약 흡수, 요약 기반 히스토리 조합

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/yourorg/infractl/internal/llm"
)

// ContentType은 API 라운드의 콘텐츠 성격을 나타낸다.
// 유형에 따라 요약 정책이 달라진다.
type ContentType string

const (
	// ContentCommand는 일반 명령어 실행 결과 — 요약 가능.
	ContentCommand ContentType = "command"
	// ContentDocument는 매뉴얼, 작업 계획서, 문서 — 원문 보존 필수.
	ContentDocument ContentType = "document"
	// ContentConfig는 설정 파일 내용 — 원문 보존 필수.
	ContentConfig ContentType = "config"
	// ContentAnalysis는 분석 결과 — 핵심 결론만 요약.
	ContentAnalysis ContentType = "analysis"
	// ContentChat는 단순 대화 — 요약 가능.
	ContentChat ContentType = "chat"
)

const (
	// mildCompactionThreshold는 프로그레시브 요약이 시작되는 라운드 수 기준이다.
	// 이 수를 초과하면 오래된 라운드부터 요약에 흡수한다.
	mildCompactionThreshold = 8

	// maxPreservedItems는 원문 보존 항목의 최대 개수이다.
	// 초과 시 오래된 것은 제목+요약 3줄로 축소된다.
	maxPreservedItems = 5

)

// preservedItem은 요약 금지 — 원문을 그대로 보존해야 하는 라운드 항목이다.
type preservedItem struct {
	roundIndex  int
	contentType ContentType
	content     string // 원문 전체
	label       string // 사용자에게 보여줄 레이블 (예: "작업 계획서", "/etc/nginx/nginx.conf")
}

// SessionSummaryManager는 대화 히스토리를 점진적으로 요약하며
// 문서/설정 등 보존 필요 콘텐츠는 원문을 별도 보관한다.
type SessionSummaryManager struct {
	summary         string          // 누적 요약 텍스트 (command/chat 라운드 요약)
	preserved       []preservedItem // 원문 보존 항목 (document/config)
	summarizedCount int             // 요약에 흡수된 라운드 수
	llmClient       llm.Client
	threshold       int // 요약 시작 라운드 수 기준 (0이면 mildCompactionThreshold 사용)
}

// NewSessionSummaryManager는 SessionSummaryManager를 생성한다.
func NewSessionSummaryManager(client llm.Client) *SessionSummaryManager {
	return &SessionSummaryManager{llmClient: client, threshold: mildCompactionThreshold}
}

// SetThreshold는 요약 시작 라운드 수 기준을 동적으로 설정한다.
// maxHistory 메시지 수를 기준으로 avgMsgsPerRound(≈5)로 나눠 라운드 수를 추정한다.
func (m *SessionSummaryManager) SetThreshold(rounds int) {
	if rounds > 0 {
		m.threshold = rounds
	}
}

// GetSummary는 현재까지의 누적 요약 텍스트를 반환한다.
func (m *SessionSummaryManager) GetSummary() string {
	return m.summary
}

// GetPreserved는 원문 보존 항목 목록을 반환한다.
func (m *SessionSummaryManager) GetPreserved() []preservedItem {
	return m.preserved
}

// HasContext는 요약 또는 보존 항목이 있는지 확인한다.
func (m *SessionSummaryManager) HasContext() bool {
	return m.summary != "" || len(m.preserved) > 0
}

// GetUnsummarizedRounds는 히스토리에서 아직 요약되지 않은 라운드의 메시지를 반환한다.
// 요약된 라운드는 제외하고, summarizedCount 이후 라운드만 포함한다.
func (m *SessionSummaryManager) GetUnsummarizedRounds(history []llm.Message) []llm.Message {
	rounds := groupByApiRound(history)
	if m.summarizedCount >= len(rounds) {
		return nil
	}
	return flattenRounds(rounds[m.summarizedCount:])
}

// UpdateIfNeeded는 총 라운드 수가 threshold를 초과하면
// 오래된 라운드를 요약에 흡수한다. 최근 preserveRecentRounds 라운드는 원문 유지.
func (m *SessionSummaryManager) UpdateIfNeeded(ctx context.Context, history []llm.Message) {
	rounds := groupByApiRound(history)
	total := len(rounds)

	if total <= m.threshold {
		return // 아직 요약 불필요
	}

	// 최근 preserveRecentRounds는 원문 보존, 나머지는 요약 대상
	targetSummarized := total - preserveRecentRounds
	if targetSummarized <= m.summarizedCount {
		return // 이미 요약 최신 상태
	}

	// summarizedCount ~ targetSummarized 사이 라운드를 처리
	newRounds := rounds[m.summarizedCount:targetSummarized]
	slog.Debug("session summary: absorbing rounds",
		"from", m.summarizedCount, "to", targetSummarized, "count", len(newRounds))

	var toSummarize []apiRound
	for _, r := range newRounds {
		ct := classifyRoundContent(r)
		switch ct {
		case ContentDocument, ContentConfig:
			label := extractContentLabel(r, ct)
			content := flattenRoundToText(r)
			m.addPreserved(m.summarizedCount, ct, content, label)
			slog.Debug("session summary: preserving verbatim", "type", ct, "label", label)
		case ContentAnalysis:
			// 분석 결과도 summarize 대상에 포함 (핵심만 추출됨)
			toSummarize = append(toSummarize, r)
		default: // ContentCommand, ContentChat
			toSummarize = append(toSummarize, r)
		}
		m.summarizedCount++
	}

	if len(toSummarize) == 0 {
		return
	}

	// LLM을 통해 증분 요약 수행
	newSummary, err := m.requestIncrementalSummary(ctx, toSummarize)
	if err != nil {
		slog.Warn("session summary: incremental summarization failed", "err", err)
		return
	}
	m.summary = newSummary
}

// addPreserved는 원문 보존 항목을 추가하고 maxPreservedItems를 초과하면 오래된 것을 축소한다.
func (m *SessionSummaryManager) addPreserved(roundIdx int, ct ContentType, content, label string) {
	item := preservedItem{
		roundIndex:  roundIdx,
		contentType: ct,
		content:     content,
		label:       label,
	}

	m.preserved = append(m.preserved, item)

	// 보존 항목이 최대치를 초과하면 가장 오래된 것을 "제목+3줄"로 축소
	if len(m.preserved) > maxPreservedItems {
		oldest := &m.preserved[0]
		lines := strings.SplitN(oldest.content, "\n", 5)
		truncated := strings.Join(lines[:min(3, len(lines))], "\n")
		oldest.content = fmt.Sprintf("[축약됨 — 원문 생략]\n%s\n...", truncated)
		slog.Debug("session summary: truncated oldest preserved item", "label", oldest.label)
	}
}

// requestIncrementalSummary는 기존 요약 + 새 라운드를 LLM에 전달하여 업데이트된 요약을 반환한다.
func (m *SessionSummaryManager) requestIncrementalSummary(ctx context.Context, newRounds []apiRound) (string, error) {
	if m.llmClient == nil {
		return m.summary, nil
	}

	roundsText := flattenRoundToText2(newRounds)
	existing := m.summary
	if existing == "" {
		existing = "(없음 — 첫 번째 요약)"
	}

	prompt := fmt.Sprintf(`%s

## 기존 요약:
%s

## 새로 추가된 대화 (command/analysis/chat 유형):
%s

요약을 업데이트하세요.`, sessionSummarySystemPrompt, existing, roundsText)

	resp, err := m.llmClient.Chat(ctx, []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}, nil, nil)
	if err != nil {
		return "", fmt.Errorf("incremental summary request: %w", err)
	}
	if strings.TrimSpace(resp.Content) == "" {
		return m.summary, nil // 실패 시 기존 요약 유지
	}
	return strings.TrimSpace(resp.Content), nil
}

// sessionSummarySystemPrompt는 증분 요약 시 LLM에게 전달하는 시스템 지시이다.
const sessionSummarySystemPrompt = `당신은 인프라 관리 대화의 컨텍스트 요약기입니다.
주의: 문서/설정 파일 유형은 이미 별도 보존되었으므로 여기서는 command/chat/analysis만 요약합니다.

규칙:
1. 기존 요약에 새 대화의 핵심 정보를 통합하세요.
2. 반드시 보존할 정보:
   - 각 작업이 수행된 서버명 (작업 당시 기준, "서버 X에서" 형식으로 명시)
   - 실행한 주요 명령어와 그 결과 핵심 (성공/실패 + 핵심 수치)
   - 발견된 문제점과 해결 상태
   - 사용자가 명시한 의도나 목표
   - 별도 보존된 문서/설정이 있다는 사실 ("이전에 작업 계획서/설정 파일 공유됨" 등)
3. 제거할 정보:
   - 명령어 출력 전문 (핵심만 유지)
   - 이미 해결된 에러의 상세 내용
   - 중간 시행착오의 세부 과정
4. 200단어 이내로 유지하세요.
5. "현재 상태"라는 표현 대신 "이전 작업"으로 표현하세요. 요약은 과거 기록이지 현재 상태가 아닙니다.`

// BuildContextMessages는 요약 + 보존 항목 + 미요약 라운드를 조합하여 LLM 메시지 슬라이스를 생성한다.
// history.go의 buildMessages()에서 호출한다.
func (m *SessionSummaryManager) BuildContextMessages(systemMsg llm.Message, history []llm.Message) []llm.Message {
	messages := []llm.Message{systemMsg}

	if !m.HasContext() {
		// 요약 없음: 기존 filterHistory 방식으로 전체 히스토리 주입
		filtered := filterHistoryStandard(history)
		messages = append(messages, filtered...)
		return messages
	}

	// 1. 누적 요약 주입
	if m.summary != "" {
		messages = append(messages,
			llm.Message{
				Role:    llm.RoleUser,
				Content: "[이전 대화 요약]\n" + m.summary,
			},
			llm.Message{
				Role:    llm.RoleAssistant,
				Content: "이전 대화 맥락을 파악했습니다. 계속 진행하겠습니다.",
			},
		)
	}

	// 2. 보존된 문서/설정 원문 주입
	if len(m.preserved) > 0 {
		var sb strings.Builder
		sb.WriteString("[보존된 문서/설정 원문]\n")
		for _, p := range m.preserved {
			sb.WriteString(fmt.Sprintf("\n### %s\n%s\n", p.label, p.content))
		}
		messages = append(messages,
			llm.Message{Role: llm.RoleUser, Content: sb.String()},
			llm.Message{Role: llm.RoleAssistant, Content: "문서/설정 내용을 확인했습니다."},
		)
	}

	// 3. 미요약 최근 라운드 원문 주입
	recentMsgs := m.GetUnsummarizedRounds(history)
	messages = append(messages, recentMsgs...)

	return messages
}

// --- 콘텐츠 유형 분류 ---

// documentStructurePattern은 마크다운 문서 구조(헤딩, 번호 목록, 표)를 감지한다.
var documentStructurePattern = regexp.MustCompile(`(?m)(^#{1,4}\s+\S|^\d+\.\s+\S|^\|\s*\S.*\|)`)

// configPattern은 설정 파일 형식(key=value, JSON, YAML, XML, INI)을 감지한다.
var configPattern = regexp.MustCompile(`(?m)(^\s*\w[\w.-]*\s*=\s*\S|^\s*\w[\w.-]*:\s*\S|^\s*<\w+[\s/>]|\{\s*"\w+"\s*:)`)

// analysisKeywordPattern은 분석 결과 텍스트를 감지한다.
var analysisKeywordPattern = regexp.MustCompile(`(?i)(원인|결론|분석 결과|진단 결과|root cause|conclusion|analysis result|findings|recommendations|권고사항)`)

// classifyRoundContent는 API 라운드의 콘텐츠 유형을 규칙 기반으로 분류한다.
// LLM 호출 없이 패턴 매칭으로 처리하여 비용을 절감한다.
func classifyRoundContent(r apiRound) ContentType {
	combined := flattenRoundToText(r)

	// 문서 구조 감지: 2000자 이상 + 마크다운 헤딩/목록/표 패턴
	if len(combined) > 2000 && documentStructurePattern.MatchString(combined) {
		return ContentDocument
	}

	// 설정 파일 감지: key=value, YAML, JSON, XML 패턴
	if configPattern.MatchString(combined) {
		lines := strings.Split(combined, "\n")
		configLines := 0
		for _, line := range lines {
			if configPattern.MatchString(line) {
				configLines++
			}
		}
		// 전체 라인의 30% 이상이 설정 패턴이면 config로 분류
		if len(lines) > 0 && configLines*10/len(lines) >= 3 {
			return ContentConfig
		}
	}

	// 분석 결과 감지
	if analysisKeywordPattern.MatchString(combined) {
		return ContentAnalysis
	}

	// 도구 호출이 있으면 command
	for _, msg := range r.messages {
		if len(msg.ToolCalls) > 0 {
			return ContentCommand
		}
	}

	return ContentChat
}

// extractContentLabel은 라운드에서 보존 항목의 레이블을 추출한다.
func extractContentLabel(r apiRound, ct ContentType) string {
	// 사용자 메시지에서 파일 경로나 제목을 추출
	for _, msg := range r.messages {
		if msg.Role != llm.RoleUser {
			continue
		}
		// 파일 경로 패턴
		if idx := strings.Index(msg.Content, "/etc/"); idx >= 0 {
			end := strings.IndexAny(msg.Content[idx:], " \n\t")
			if end < 0 {
				end = len(msg.Content) - idx
			}
			return msg.Content[idx : idx+end]
		}
		// 첫 줄을 레이블로 사용
		firstLine := strings.SplitN(strings.TrimSpace(msg.Content), "\n", 2)[0]
		if len(firstLine) > 60 {
			firstLine = firstLine[:60] + "..."
		}
		if firstLine != "" {
			return firstLine
		}
	}

	switch ct {
	case ContentDocument:
		return "문서/계획서"
	case ContentConfig:
		return "설정 파일"
	default:
		return "보존 항목"
	}
}

// flattenRoundToText는 API 라운드의 모든 메시지를 단일 텍스트로 합친다.
func flattenRoundToText(r apiRound) string {
	var sb strings.Builder
	for _, msg := range r.messages {
		if msg.Content != "" {
			sb.WriteString(msg.Content)
			sb.WriteString("\n")
		}
		for _, tc := range msg.ToolCalls {
			sb.WriteString(tc.Function.Arguments)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// flattenRoundToText2는 여러 API 라운드를 단일 텍스트로 합친다.
func flattenRoundToText2(rounds []apiRound) string {
	var sb strings.Builder
	for i, r := range rounds {
		sb.WriteString(fmt.Sprintf("--- Round %d ---\n", i+1))
		sb.WriteString(flattenRoundToText(r))
	}
	return sb.String()
}

// MarkServerTransition은 활성 서버가 변경될 때 요약에 컨텍스트 전환 마커를 삽입한다.
// 이후 LLM이 이전 요약 내용이 다른 서버의 작업 기록임을 명확히 인지하게 한다.
func (m *SessionSummaryManager) MarkServerTransition(from, to string) {
	marker := fmt.Sprintf(
		"\n\n⚠️ [서버 컨텍스트 전환: %s → %s]\n"+
			"이하 내용은 %s 작업 이전의 기록입니다. 위 요약 내용은 현재 서버(%s)와 무관할 수 있습니다.",
		from, to, to, to,
	)
	m.summary += marker
	slog.Debug("session summary: server transition marked", "from", from, "to", to)
}

// filterHistoryStandard는 요약이 없을 때 사용하는 기존 히스토리 필터 로직이다.
// history.go의 filterHistory와 동일한 로직을 재사용한다.
func filterHistoryStandard(history []llm.Message) []llm.Message {
	rounds := groupByApiRound(history)
	if len(rounds) <= preserveRecentRounds {
		return history
	}

	var filtered []llm.Message
	for i, r := range rounds {
		if i >= len(rounds)-preserveRecentRounds {
			filtered = append(filtered, r.messages...)
			continue
		}
		for _, msg := range r.messages {
			if msg.Role == llm.RoleUser {
				filtered = append(filtered, msg)
			} else if msg.Role == llm.RoleAssistant && msg.Content != "" {
				newMsg := msg
				newMsg.ToolCalls = nil
				filtered = append(filtered, newMsg)
			}
		}
	}
	return filtered
}

