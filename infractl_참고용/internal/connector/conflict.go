// Package connector
// File: conflict.go
// Description: server/service name conflict helper
// Responsibility: only flag real ambiguity when execution target differs from alias server

package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/store"
)

// checkNameConflict evaluates server/service naming ambiguity.
// It no longer overrides explicit server alias intent.
func checkNameConflict(ctx context.Context, st store.ServerStore, handler DisambiguateHandler, server, serviceType, currentTarget string) (string, bool, bool) {
	_ = handler // explicit alias priority: no interactive disambiguation in this flow.

	if st == nil {
		return server, false, false
	}
	if !strings.EqualFold(strings.TrimSpace(server), strings.TrimSpace(serviceType)) {
		return server, false, false
	}

	servers, err := st.List(ctx)
	if err != nil {
		return server, false, false
	}
	for _, s := range servers {
		if !strings.EqualFold(s.Name, server) {
			continue
		}
		if sameServerTarget(currentTarget, s.Name) {
			return server, false, false
		}
		// Real ambiguity exists (alias name collides with service_type and target differs),
		// but explicit alias should remain the resolved server.
		return s.Name, true, false
	}

	return server, false, false
}

// checkNameConflictMsg returns a plain-text warning when real ambiguity exists.
func checkNameConflictMsg(ctx context.Context, st store.ServerStore, server, serviceType, currentTarget string) string {
	if st == nil {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(server), strings.TrimSpace(serviceType)) {
		return ""
	}

	servers, err := st.List(ctx)
	if err != nil {
		return ""
	}
	for _, s := range servers {
		if !strings.EqualFold(s.Name, server) {
			continue
		}
		if sameServerTarget(currentTarget, s.Name) {
			return ""
		}
		target := strings.TrimSpace(currentTarget)
		if target == "" {
			target = "localhost"
		}
		return fmt.Sprintf(
			"이름 충돌 주의: '%s'는 SSH 서버 alias이면서 service_type이기도 합니다. 현재 실행 target은 '%s'입니다.",
			server, target,
		)
	}

	return ""
}
