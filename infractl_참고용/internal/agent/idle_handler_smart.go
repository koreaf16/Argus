// Package agent
// File: idle_handler_smart.go
// Description: LLM 기반 idle 프롬프트 자동 응답 및 비밀번호/DB REPL 감지
// Responsibility: SmartIdleInputHandler 구현 — 패스워드 주입, DB REPL EOF, LLM 분류

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/store"
)

// SmartIdleInputHandler uses LLM to autonomously respond to interactive shell prompts.
// There is no user-facing fallback — the LLM decides or aborts.
type SmartIdleInputHandler struct {
	llmClient   llm.Client
	serverStore store.ServerStore
	credVault   *privilege.Vault
}

func NewSmartIdleInputHandler(client llm.Client) *SmartIdleInputHandler {
	return &SmartIdleInputHandler{llmClient: client}
}

func (h *SmartIdleInputHandler) SetServerStore(st store.ServerStore) {
	h.serverStore = st
}

// SetCredVault wires a Vault for session-scoped credential lookup before
// falling through to the server store.
func (h *SmartIdleInputHandler) SetCredVault(v *privilege.Vault) {
	h.credVault = v
}

const idleClassifySystemPrompt = `You are helping an AI agent respond to interactive shell prompts autonomously.
The shell command is waiting for stdin. Analyze the last output lines and respond with JSON ONLY (no other text):

{"input": "<string to send>", "close_stdin": false, "abort": false}

If EOF/Ctrl-D is the best response, respond:
{"input": "", "close_stdin": true, "abort": false}

If you cannot safely determine a response (e.g., an unknown password is required), respond:
{"input": "", "close_stdin": false, "abort": true}

Decision rules:
- SSH host key "(yes/no/[fingerprint])?" or "(yes/no)?": {"input": "yes", "abort": false}
- Simple [Y/n] prompt where continuing is appropriate: {"input": "y", "abort": false}
- Simple [y/N] prompt where the safe default is no: {"input": "n", "abort": false}
- "Press Enter to continue" / "-- More --" / "--More--": {"input": "", "abort": false}
- less/more pager "(END)" or "(end)": {"input": "q", "abort": false}
- unzip "replace FILE? [y]es, [n]o, [A]ll, [N]one, [r]ename:": {"input": "A", "abort": false}
- Any multi-choice prompt with bracketed options: pick the most appropriate option letter/word
- Password / passphrase field when no stored credential is available: {"input": "", "abort": true}
- SQL> (Oracle SQLPlus REPL): {"input": "exit", "close_stdin": false, "abort": false}
- mysql> (MySQL/MariaDB REPL): {"input": "exit", "close_stdin": false, "abort": false}
- psql prompts ending with => or =#: {"input": "\\q", "close_stdin": false, "abort": false}
- DB connection banner (SQL*Plus header, "Connected to Oracle", "Welcome to MySQL monitor") without a prompt yet: {"input": "", "close_stdin": false, "abort": false}
- NEVER abort for DB REPL prompts — always send the appropriate exit command
- NEVER abort for simple yes/no/choice prompts — always pick the best answer`

type idleAction struct {
	Input      string `json:"input"`
	CloseStdin bool   `json:"close_stdin"`
	Abort      bool   `json:"abort"`
}

// passwordPromptRegex detects common password prompts in English and Korean.
// Korean: "암호:" (su/sudo), Japanese: "パスワード:", common sudo "[sudo] password for user:"
var passwordPromptRegex = regexp.MustCompile(`(?i)(password:|passphrase for .*:|password for .*:|password for [^ ]+|[A-Za-z0-9_.-]+@[A-Za-z0-9_.:-]+'s password:|암호:|パスワード:)`)
var passwordPromptUserHostRegex = regexp.MustCompile(`(?i)([A-Za-z0-9_.-]+)@([^']+)'s password:`)
var passwordPromptForRegex = regexp.MustCompile(`(?i)password for ([^:]+):`)

// Korean sudo prompt: "[sudo] <user>의 암호:"
var passwordPromptKoreanSudoRegex = regexp.MustCompile(`\[sudo\]\s+([A-Za-z0-9_.-]+)의 암호:`)

// ansiEscapeRegex strips ANSI color/style codes from PTY output before regex matching.
var ansiEscapeRegex = regexp.MustCompile(`\033\[[0-9;]*[mKHFABCDsuJn]`)

type passwordPromptMatcher struct {
	host string
	user string
}

func resolveStoredPassword(ctx context.Context, st store.ServerStore, req IdleInputRequest) (string, bool) {
	if st == nil {
		return "", false
	}

	combined := ansiEscapeRegex.ReplaceAllString(strings.Join(req.LastLines, "\n"), "")
	if !passwordPromptRegex.MatchString(combined) {
		return "", false
	}

	match := parsePasswordPrompt(combined)

	servers, err := st.List(ctx)
	if err != nil {
		slog.Warn("idle password lookup failed", "err", err, "target", req.Target)
		return "", false
	}

	target := strings.TrimSpace(req.Target)
	for _, srv := range servers {
		if srv.AuthType != store.AuthTypePassword || srv.Credential == "" {
			continue
		}

		// 프롬프트가 서버 SSH 접속 유저와 다른 계정(예: root)을 요구하면
		// 저장된 SSH 크리덴셜은 해당 계정의 비밀번호가 아니므로 주입하지 않는다.
		if match.user != "" && !strings.EqualFold(match.user, srv.User) {
			slog.Debug("idle password: prompt user differs from SSH user, skipping",
				"prompt_user", match.user, "ssh_user", srv.User, "target", target)
			continue
		}

		if target != "" && (strings.EqualFold(srv.Name, target) || strings.EqualFold(srv.Host, target)) {
			return srv.Credential, true
		}
		if match.host != "" && strings.EqualFold(srv.Host, match.host) {
			return srv.Credential, true
		}
		if match.user != "" && strings.EqualFold(srv.User, match.user) && target == "" {
			return srv.Credential, true
		}
	}
	return "", false
}

func parsePasswordPrompt(text string) passwordPromptMatcher {
	matcher := passwordPromptMatcher{}

	if m := passwordPromptUserHostRegex.FindStringSubmatch(text); len(m) == 3 {
		matcher.user = strings.TrimSpace(m[1])
		matcher.host = strings.TrimSpace(m[2])
		return matcher
	}

	if m := passwordPromptForRegex.FindStringSubmatch(text); len(m) == 2 {
		matcher.user = strings.TrimSpace(m[1])
		return matcher
	}

	// Korean sudo: "[sudo] <user>의 암호:"
	if m := passwordPromptKoreanSudoRegex.FindStringSubmatch(text); len(m) == 2 {
		matcher.user = strings.TrimSpace(m[1])
	}

	return matcher
}

func hasInteractivePromptCue(lines []string) bool {
	combined := strings.Join(lines, "\n")
	if passwordPromptRegex.MatchString(ansiEscapeRegex.ReplaceAllString(combined, "")) {
		return true
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(ansiEscapeRegex.ReplaceAllString(line, ""))
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		switch {
		case strings.Contains(lower, "(yes/no"):
			return true
		case strings.Contains(lower, "[y]es"), strings.Contains(lower, "[n]o"):
			return true
		case strings.Contains(lower, "[a]ll"), strings.Contains(lower, "[n]one"):
			return true
		case strings.Contains(lower, "press enter"), strings.Contains(lower, "-- more --"):
			return true
		case strings.Contains(lower, "--more--"), trimmed == "(end)":
			return true
		case strings.HasSuffix(trimmed, "=>"), strings.HasSuffix(trimmed, "=#"):
			return true
		case strings.HasSuffix(trimmed, ">") && !strings.ContainsAny(trimmed, " \t"):
			return true
		case strings.Contains(trimmed, "?") && strings.ContainsAny(trimmed, "[]()"):
			return true
		}
	}
	return false
}

func (h *SmartIdleInputHandler) RequestIdleInput(ctx context.Context, req IdleInputRequest) (IdleInputResponse, error) {
	// Step 1: vault lookup (session-scoped, TTL-aware)
	if pw, ok := resolveVaultPassword(h.credVault, req); ok {
		slog.Info("idle auto-respond password (vault)", "target", req.Target)
		return IdleInputResponse{Input: pw}, nil
	}
	// Step 2: SSH store lookup (existing path)
	if pw, ok := resolveStoredPassword(ctx, h.serverStore, req); ok {
		slog.Info("idle auto-respond password", "target", req.Target, "tool", req.ToolName)
		return IdleInputResponse{Input: pw}, nil
	}
	if !hasInteractivePromptCue(req.LastLines) {
		// No prompt-like output yet: avoid speculative responses (e.g., accidental EOF).
		return IdleInputResponse{}, nil
	}

	userContent := fmt.Sprintf("Command: %s\nTarget: %s\nLast output lines:\n%s",
		req.Command, req.Target, strings.Join(req.LastLines, "\n"))

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: idleClassifySystemPrompt},
		{Role: llm.RoleUser, Content: userContent},
	}

	resp, err := h.llmClient.Chat(ctx, messages, nil, nil)
	if err != nil {
		slog.Warn("idle llm classify failed, aborting command", "err", err)
		return IdleInputResponse{Abort: true}, nil
	}

	var action idleAction
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &action); jsonErr != nil {
		slog.Warn("idle llm response parse failed, aborting command", "raw", resp.Content, "err", jsonErr)
		return IdleInputResponse{Abort: true}, nil
	}

	if action.Abort {
		slog.Warn("idle llm chose to abort", "command", req.Command, "target", req.Target)
		return IdleInputResponse{Abort: true}, nil
	}

	slog.Info("idle auto-respond", "input", action.Input, "close_stdin", action.CloseStdin, "command", req.Command, "target", req.Target)
	return IdleInputResponse{Input: action.Input, CloseStdin: action.CloseStdin}, nil
}
