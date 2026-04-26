// Package agent
// File: active_server.go
// Description: 에이전트의 현재 활성 인프라 서버 컨텍스트를 관리합니다.
// Responsibility: 서버 전환 감지, 세션 요약 기록, 프롬프트 캐시 무효화 및 원격 감사 데이터 동기화 조정.

package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/yourorg/infractl/internal/store"
)

func (a *Agent) applyActiveServer(next *store.Server) {
	if sameActiveServer(a.activeServer, next) {
		return
	}

	if a.sessionSummary != nil && a.activeServer != nil && next != nil {
		a.sessionSummary.MarkServerTransition(a.activeServer.Name, next.Name)
	}

	if next == nil {
		a.activeServer = nil
	} else {
		cp := *next
		a.activeServer = &cp
	}
	if a.promptCache != nil {
		a.promptCache.Clear()
	}

	if a.activeServerNotify != nil {
		a.activeServerNotify(a.activeServer)
	}

	// Audit pull: 서버 전환 시 원격 audit.db 병합 (비동기, best-effort)
	if next != nil && a.auditSyncEngine != nil {
		if exec, err := a.manager.Get(next.Name); err == nil {
			coord := newAuditSyncCoordinator(a.auditSyncEngine, exec, next.Name, remoteAuditDBPath(next.WorkspaceDir))
			a.auditSync = coord
			go coord.pullOnAttach(context.Background())
		}
	} else if next == nil {
		a.auditSync = nil
	}
}

func sameActiveServer(cur, next *store.Server) bool {
	if cur == nil || next == nil {
		return cur == nil && next == nil
	}
	return strings.EqualFold(strings.TrimSpace(cur.Name), strings.TrimSpace(next.Name))
}

func (a *Agent) ActiveServerSnapshot() *store.Server {
	if a.activeServer == nil {
		return nil
	}
	cp := *a.activeServer
	return &cp
}


func mentionsAlias(input, alias string) bool {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return false
	}

	if isASCIIAlias(alias) {
		pat := fmt.Sprintf(`(?i)(^|[^a-z0-9_-])%s([^a-z0-9_-]|$)`, regexp.QuoteMeta(alias))
		return regexp.MustCompile(pat).MatchString(input)
	}

	return strings.Contains(strings.ToLower(input), strings.ToLower(alias))
}

func isASCIIAlias(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
