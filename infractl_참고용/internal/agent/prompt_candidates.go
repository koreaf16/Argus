// Package agent
// File: prompt_candidates.go
// Description: 사용자 입력에 등장하는 워크스페이스/서비스 후보 목록을 시스템 프롬프트에 주입
// Responsibility: LLM이 이름 중복 여부를 즉시 판단할 수 있도록 입력 관련 후보만 필터링·렌더링

package agent

import (
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/store"
)

// candidateInfo는 disambiguation 판단용 단일 후보 정보다.
type candidateInfo struct {
	Kind string // "server" | "service"
	Name string
	Hint string
}

// collectDisambiguationCandidates는 사용자 입력에 이름이 등장하는
// 워크스페이스와 커넥터 서비스를 후보로 수집한다.
// 0개면 nil을 반환하여 프롬프트 섹션을 생략할 수 있다.
func collectDisambiguationCandidates(
	input string,
	servers []store.Server,
	connStates []connector.ConnectorState,
) []candidateInfo {
	var out []candidateInfo
	seen := make(map[string]bool)

	for _, srv := range servers {
		if mentionsAlias(input, srv.Name) {
			key := "workspace:" + strings.ToLower(srv.Name)
			if !seen[key] {
				seen[key] = true
				hint := fmt.Sprintf("%s@%s:%d", srv.User, srv.Host, srv.Port)
				out = append(out, candidateInfo{Kind: "server", Name: srv.Name, Hint: hint})
			}
		}
	}

	for _, st := range connStates {
		if mentionsAlias(input, st.ServiceName) || mentionsAlias(input, st.Type) {
			key := "service:" + strings.ToLower(st.ServiceName)
			if !seen[key] {
				seen[key] = true
				hint := fmt.Sprintf("%s connector on %s", st.Type, st.ServerName)
				out = append(out, candidateInfo{Kind: "service", Name: st.ServiceName, Hint: hint})
			}
		}
	}

	return out
}

// renderCandidatesSection은 후보 목록을 시스템 프롬프트 섹션 문자열로 렌더링한다.
func renderCandidatesSection(candidates []candidateInfo) string {
	if len(candidates) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Disambiguation Candidates\n")
	sb.WriteString("The user's message matches multiple entities. If the target is ambiguous, call `ask_user_question` to clarify before taking action.\n\n")
	for _, c := range candidates {
		sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", c.Name, c.Kind, c.Hint))
	}
	sb.WriteString("\n")
	return sb.String()
}
