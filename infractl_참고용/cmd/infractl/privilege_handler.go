// Package main
// File: privilege_handler.go
// Description: ServerStore 기반 sudo/su 비밀번호 자동 주입 핸들러
// Responsibility: 저장된 SSH 크리덴셜을 권한 상승 비밀번호로 먼저 시도하고, 없으면 fallback 핸들러에 위임

package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/store"
)

// serverStorePrivilegeHandler는 ServerStore에 저장된 SSH 비밀번호를
// sudo/su 권한 상승에 우선 사용하는 PromptHandler 구현체다.
// 저장된 비밀번호가 없거나 대상 서버를 찾지 못하면 fallback 핸들러에 위임한다.
type serverStorePrivilegeHandler struct {
	store    store.ServerStore
	fallback privilege.PromptHandler
}

// newStorePrivilegeHandler는 ServerStore를 우선 조회하는 PromptHandler를 생성한다.
func newStorePrivilegeHandler(st store.ServerStore, fallback privilege.PromptHandler) privilege.PromptHandler {
	return &serverStorePrivilegeHandler{store: st, fallback: fallback}
}

// RequestPassword는 target 서버의 저장된 SSH 비밀번호를 권한 상승에 사용한다.
// sudo는 SSH 로그인 유저의 비밀번호를 요구하므로, become_user가 root이거나
// SSH 유저 본인일 때 저장된 SSH 비밀번호를 반환한다.
func (h *serverStorePrivilegeHandler) RequestPassword(ctx context.Context, req privilege.PromptRequest) (privilege.PromptResponse, error) {
	if h.store != nil {
		if pw, ok := h.lookupStoredPassword(ctx, req); ok {
			return privilege.PromptResponse{Password: pw}, nil
		}
	}
	if h.fallback != nil {
		return h.fallback.RequestPassword(ctx, req)
	}
	return privilege.PromptResponse{}, fmt.Errorf("sudo 비밀번호를 찾을 수 없습니다 (target=%s)", req.Target)
}

func (h *serverStorePrivilegeHandler) lookupStoredPassword(ctx context.Context, req privilege.PromptRequest) (string, bool) {
	servers, err := h.store.List(ctx)
	if err != nil {
		slog.Warn("privilege handler: server store lookup failed", "err", err)
		return "", false
	}

	target := strings.TrimSpace(req.Target)
	for _, srv := range servers {
		if srv.AuthType != store.AuthTypePassword || srv.Credential == "" {
			continue
		}
		if !strings.EqualFold(srv.Name, target) && !strings.EqualFold(srv.Host, target) {
			continue
		}
		// sudo는 항상 명령을 실행하는 사용자의 비밀번호를 요구하므로 (대상 사용자와 무관),
		// MethodSudo이면 저장된 SSH 비밀번호를 주입한다.
		// su는 대상 사용자의 비밀번호를 요구하므로, become_user가 본인이거나 root(생략)일 때만 주입한다.
		becomeUser := strings.TrimSpace(req.User)
		if req.Method == privilege.MethodSudo || becomeUser == "" || becomeUser == "root" || strings.EqualFold(becomeUser, srv.User) {
			slog.Info("privilege: using stored ssh credential for sudo/su",
				"method", req.Method, "target", target, "become_user", req.User, "ssh_user", srv.User)
			return srv.Credential, true
		}
	}
	return "", false
}
