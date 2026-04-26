// Package agent
// File: audit_privilege_hook.go
// Description: 권한 상승 이벤트를 audit 레이어에 기록하는 훅 구현
// Responsibility: PrivilegeAuditHook 인터페이스 + ShellExecTool 연동 어댑터

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

// PrivilegeAuditHook은 ShellExecTool 에 주입되는 콜백 인터페이스이다.
// tools 패키지 의존성을 역전시켜 순환 임포트를 방지한다.
type privilegeAuditHook struct {
	store       AuditStore
	getRunID    func() string
	getServer   func() string
}

// NewPrivilegeAuditHook은 Agent 상태에 접근하는 클로저 기반 훅을 생성한다.
func (a *Agent) NewPrivilegeAuditHook() tools.PrivilegeAuditNotifier {
	return &privilegeAuditHook{
		store:     a.auditStore,
		getRunID:  func() string { return a.auditRunID },
		getServer: func() string {
			if a.activeServer != nil {
				return a.activeServer.Name
			}
			return "localhost"
		},
	}
}

// NotifyPrivilegeStart는 권한 상승 명령 실행 직전에 호출된다.
// pre-snapshot 수집 및 audit_privilege_events INSERT를 수행한다.
// 반환값 eventID 는 NotifyPrivilegeEnd 에 전달해야 한다.
func (h *privilegeAuditHook) NotifyPrivilegeStart(ctx context.Context, req tools.PrivilegeAuditRequest) string {
	if h.store == nil {
		return ""
	}
	eventID := uuid.New().String()
	serverName := h.getServer()
	evt := store.AuditPrivilegeEvent{
		ID:            eventID,
		RunID:         h.getRunID(),
		ServerID:      serverName,
		ServerName:    serverName,
		Command:       req.Command,
		NormalizedCmd: req.NormalizedCmd,
		Invoker:       req.Invoker,
		Method:        req.Method,
		ApprovedBy:    req.ApprovedBy,
		StartedAt:     time.Now().UTC(),
	}
	if err := h.store.InsertAuditPrivilegeEvent(ctx, evt); err != nil {
		slog.Warn("audit: insert privilege event", "err", err)
		return ""
	}

	// pre-snapshot: 타겟 경로 추출 후 hash 기록
	for _, path := range extractFilePaths(req.Command) {
		snap := buildFileSnapshot(eventID, path, "pre", req.FileContents[path])
		if err := h.store.InsertAuditFileSnapshot(ctx, snap); err != nil {
			slog.Warn("audit: pre-snapshot", "path", path, "err", err)
		}
	}
	return eventID
}

// NotifyPrivilegeEnd는 권한 상승 명령 완료 후 호출된다.
// post-snapshot 비교 및 이벤트 종결을 수행한다.
func (h *privilegeAuditHook) NotifyPrivilegeEnd(ctx context.Context, eventID string, exitCode int, execLogID *int64, postContents map[string][]byte) {
	if h.store == nil || eventID == "" {
		return
	}
	if err := h.store.UpdateAuditPrivilegeEventEnd(ctx, eventID, exitCode, time.Now().UTC(), execLogID); err != nil {
		slog.Warn("audit: update privilege event end", "err", err)
	}
	for path, content := range postContents {
		snap := buildFileSnapshot(eventID, path, "post", content)
		if err := h.store.InsertAuditFileSnapshot(ctx, snap); err != nil {
			slog.Warn("audit: post-snapshot", "path", path, "err", err)
		}
	}
}

func buildFileSnapshot(eventID, path, phase string, content []byte) store.AuditFileSnapshot {
	h := sha256.New()
	h.Write(content)
	sum := hex.EncodeToString(h.Sum(nil))
	return store.AuditFileSnapshot{
		ID:               uuid.New().String(),
		PrivilegeEventID: eventID,
		Path:             path,
		Phase:            phase,
		SHA256:           sum,
		Size:             int64(len(content)),
		ContentBlob:      content,
		CapturedAt:       time.Now().UTC(),
	}
}

// extractFilePaths는 쉘 커맨드에서 변경 가능성이 있는 파일 경로를 추출한다.
// 지원: sed -i, tee, cp/mv, 리다이렉션(>/>>) 대상, systemctl edit, vi/vim/nano
var (
	sedInlineRe   = regexp.MustCompile(`(?i)sed\s+(?:-[a-z]*i[a-z]*\s+)([/~][^\s;|&]+)`)
	teeRe         = regexp.MustCompile(`(?i)\btee\b\s+(?:-[a-z]+\s+)*([/~][^\s;|&]+)`)
	cpMvRe        = regexp.MustCompile(`(?i)\b(?:cp|mv)\b[^;|&]*\s([/~][^\s;|&>]+)\s*$`)
	redirectRe    = regexp.MustCompile(`>+\s*([/~][^\s;|&]+)`)
	editorRe      = regexp.MustCompile(`(?i)\b(?:vi|vim|nano|emacs|gedit)\b\s+([/~][^\s;|&]+)`)
	chmod_chownRe = regexp.MustCompile(`(?i)\b(?:chmod|chown)\b[^;|&]*\s([/~][^\s;|&>]+)`)
	rmRe          = regexp.MustCompile(`(?i)\brm\b[^;|&]*\s+(?:-[a-z]+\s+)*([/~][^\s;|&>]+)`)
)

func extractFilePaths(command string) []string {
	seen := map[string]bool{}
	var result []string
	addPath := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			return
		}
		// 쉘 특수문자 또는 변수 포함 경로 제외
		if strings.ContainsAny(p, "$*?[]{}") {
			return
		}
		seen[p] = true
		result = append(result, p)
	}

	for _, re := range []*regexp.Regexp{sedInlineRe, teeRe, cpMvRe, redirectRe, editorRe, chmod_chownRe, rmRe} {
		for _, m := range re.FindAllStringSubmatch(command, -1) {
			if len(m) > 1 {
				addPath(m[1])
			}
		}
	}
	return result
}
