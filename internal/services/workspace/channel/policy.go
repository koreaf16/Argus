package channel

import "strings"

// ElevationPolicy is a read-only snapshot of workspace.Elevation used by the
// channel layer so it does not import the workspace package (import cycle).
type ElevationPolicy struct {
	Allowed     bool
	Mode        string   // "" | "none" | "password" | "reuse_login"
	TargetUsers []string // empty means any target is allowed
}

// ElevationVerdict is the result of EvaluateElevation.
type ElevationVerdict struct {
	Allow    bool
	Reason   string // human-readable rejection message
	Code     string // machine-readable code for emit
	Password string // resolved password to inject (non-empty only when Allow=true)
}

// RejectCode constants match the plan's per-code error message variants.
const (
	RejectCodeElevationDisabled    = "elevation_disabled"
	RejectCodeTargetUserBlocked    = "target_user_blocked"
	RejectCodePasswordMissing      = "password_missing"
	RejectCodeLoginPasswordMissing = "login_password_missing"
)

// CredentialResolver lets the channel policy layer query the workspace
// credential store without importing the workspace package.
type CredentialResolver interface {
	// GetSudoPassword returns the cached sudo/su password for the given alias
	// and targetUser. targetUser="" means the wildcard slot.
	GetSudoPassword(alias, targetUser string) string
	// GetLoginPassword returns the cached SSH login password for the alias.
	GetLoginPassword(alias string) string
}

// EvaluateElevation decides whether a privilege-escalation transition is
// permitted under p, and resolves the password to inject.
//
//   - t.Kind == None or Exit: always allowed (no escalation happening).
//   - t.Kind == Enter or Inline: checked against p.
func EvaluateElevation(alias string, p ElevationPolicy, t AccountTransitionHint, creds CredentialResolver) ElevationVerdict {
	// Non-escalating transitions are always allowed.
	if !t.IsEscalation {
		return ElevationVerdict{Allow: true}
	}

	// Check top-level gate.
	if !p.Allowed || strings.EqualFold(p.Mode, "none") {
		return ElevationVerdict{
			Allow:  false,
			Code:   RejectCodeElevationDisabled,
			Reason: elevationDisabledMsg(alias),
		}
	}

	// Check target-user allowlist.
	if t.TargetUser != "" && len(p.TargetUsers) > 0 {
		allowed := false
		hasRootInList := false
		for _, u := range p.TargetUsers {
			if strings.EqualFold(u, "root") {
				hasRootInList = true
			}
			if strings.EqualFold(u, t.TargetUser) {
				allowed = true
				break
			}
		}

		// If root is in the allowlist, permit any target user (root omnipotence).
		if hasRootInList {
			allowed = true
		}

		if !allowed {
			targets := strings.Join(p.TargetUsers, ", ")
			return ElevationVerdict{
				Allow:  false,
				Code:   RejectCodeTargetUserBlocked,
				Reason: "🛑 target user `" + t.TargetUser + "`는 " + alias + "의 elevation 화이트리스트에 없습니다 (허용 목록: " + targets + ").\n\n  /server edit " + alias + " 명령으로 허용 목록을 수정하세요.",
			}
		}
	}

	// Resolve password by mode.
	switch strings.ToLower(p.Mode) {
	case "password":
		pw := ""
		if creds != nil {
			pw = creds.GetSudoPassword(alias, t.TargetUser)
			if pw == "" && t.TargetUser != "" {
				pw = creds.GetSudoPassword(alias, "")
			}
		}
		if pw == "" {
			return ElevationVerdict{
				Allow:  false,
				Code:   RejectCodePasswordMissing,
				Reason: "🔒 " + alias + "의 sudo/su 비밀번호가 등록되어 있지 않습니다.\n\n  /server edit " + alias + " 명령으로 비밀번호를 등록하세요.",
			}
		}
		return ElevationVerdict{Allow: true, Password: pw}

	case "reuse_login":
		pw := ""
		if creds != nil {
			pw = creds.GetLoginPassword(alias)
		}
		if pw == "" {
			return ElevationVerdict{
				Allow:  false,
				Code:   RejectCodeLoginPasswordMissing,
				Reason: "🔒 " + alias + "의 로그인 비밀번호가 캐시되지 않아 reuse_login 모드를 적용할 수 없습니다.\n\n  /server edit " + alias + " 명령으로 mode=password로 전환하고 비밀번호를 등록하세요.",
			}
		}
		return ElevationVerdict{Allow: true, Password: pw}

	default:
		// Mode is "" (unset) — treat as allowed without injecting a password.
		// This keeps backward-compat with servers that have Allowed=true but
		// no Mode set; the channel layer will prompt interactively if needed.
		return ElevationVerdict{Allow: true}
	}
}

func elevationDisabledMsg(alias string) string {
	return "🛑 " + alias + "에 elevation 정책이 비활성화되어 있어 sudo/su를 실행할 수 없습니다.\n\n" +
		"  허용하려면 다음을 실행하세요:\n" +
		"    /server edit " + alias + "\n\n" +
		"  modal에서 ON으로 토글하고 mode/비밀번호를 등록한 뒤 Ctrl+S로 저장하세요.\n" +
		"  이는 서버별 정책이며 자동 활성화되지 않습니다."
}

// AccountTransitionHint is a minimal view of lane.AccountTransition used by
// the channel policy layer. It avoids re-importing the lane package where only
// this subset of data is needed.
type AccountTransitionHint struct {
	// IsEscalation is true for Enter and Inline transitions (sudo -i, su -,
	// sudo -u X cmd). Exit and None transitions set this to false.
	IsEscalation bool
	// TargetUser is the account being entered. Empty for None/Exit.
	TargetUser string
}
