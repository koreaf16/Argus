// Package agent
// File: idle_handler_smart_vault.go
// Description: SmartIdleInputHandler의 Vault 기반 비밀번호 재사용 조회 로직
// Responsibility: 세션 스코프 자격증명 캐시(privilege.Vault)에서 비밀번호를 먼저 조회

package agent

import (
	"log/slog"
	"strings"

	"github.com/yourorg/infractl/internal/privilege"
)

// resolveVaultPassword looks up the credential vault before prompting the user.
// It checks whether the idle prompt is a password prompt, then queries the vault
// using the detected method and user.
// Returns (password, true) if found; ("", false) otherwise.
func resolveVaultPassword(vault *privilege.Vault, req IdleInputRequest) (string, bool) {
	if vault == nil {
		return "", false
	}

	combined := strings.Join(req.LastLines, "\n")
	combined = ansiEscapeRegex.ReplaceAllString(combined, "")

	if !passwordPromptRegex.MatchString(combined) {
		return "", false
	}

	match := parsePasswordPrompt(combined)

	// Determine privilege method from command context
	method := privilege.MethodSU
	cmd := strings.ToLower(strings.TrimSpace(req.Command))
	if strings.Contains(cmd, "sudo") {
		method = privilege.MethodSudo
	}

	lookupUser := match.user
	if lookupUser == "" {
		lookupUser = "root" // default elevation target
	}

	pw, ok := vault.Lookup(req.Target, method, lookupUser)
	if ok {
		slog.Info("vault password hit", "target", req.Target, "user", lookupUser)
	}
	return pw, ok
}
