package query

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/services/llm"
	toolpkg "github.com/koreaf16/argus/internal/tools"
)

type persistencePolicy struct {
	Enabled                bool
	MaxForcedContinuations int
	MaxSameFailureRetries  int
	Web                    webEvidencePolicy
	RequireVerification    bool
}

type persistenceState struct {
	Web                    webEvidenceState
	ForcedContinuations    int
	TotalToolCalls         int
	ActionToolSeen         bool
	VerificationSeen       bool
	LastRecoverableError   bool
	LastRecoverableTool    string
	LastRecoverableOutput  string
	lastFailureKey         string
	SameFailureRetries     int
	BufferedAssistantText  string
	LastAssistantSnapshot  string
	RepeatedAssistantCount int
}

// maxRepeatedAssistantTurns caps how many turns the engine will keep forcing a
// continuation when the assistant produces an essentially identical reply each
// time. Two consecutive duplicates already strongly suggest the model is stuck
// in a loop instead of making progress.
const maxRepeatedAssistantTurns = 2

// NoteAssistantTurnText records the assistant's plain-text reply for a turn
// and updates the repetition counter. Empty replies (tool-only turns) do not
// reset the counter so a long action sequence does not mask an earlier loop.
func (st *persistenceState) NoteAssistantTurnText(text string) {
	current := strings.TrimSpace(text)
	if current == "" {
		return
	}
	if current == st.LastAssistantSnapshot {
		st.RepeatedAssistantCount++
		return
	}
	st.LastAssistantSnapshot = current
	st.RepeatedAssistantCount = 1
}

func classifyPersistencePolicy(cfg Config) persistencePolicy {
	if !cfg.PersistenceEnabled {
		return persistencePolicy{}
	}
	cfg = applyPersistenceDefaults(cfg)
	return persistencePolicy{
		Enabled:                true,
		MaxForcedContinuations: cfg.MaxForcedContinuations,
		MaxSameFailureRetries:  cfg.MaxSameFailureRetries,
		Web:                    defaultWebEvidencePolicy(),
	}
}

func (st *persistenceState) ObserveToolResult(call llm.ToolUseStart, output string, isErr bool, registry *toolpkg.Registry) {
	st.TotalToolCalls++
	st.Web.ObserveToolCall(call.Name, output, isErr)

	if isActionToolCall(call, registry) {
		st.ActionToolSeen = true
	}
	if !isErr && isVerificationToolCall(call) {
		st.VerificationSeen = true
	}

	if !isErr {
		st.LastRecoverableError = false
		return
	}
	if !isRecoverableToolError(output) {
		st.LastRecoverableError = false
		return
	}

	key := toolFailureKey(call.Name, output)
	if key == st.lastFailureKey {
		st.SameFailureRetries++
	} else {
		st.lastFailureKey = key
		st.SameFailureRetries = 1
	}
	st.LastRecoverableError = true
	st.LastRecoverableTool = strings.TrimSpace(call.Name)
	st.LastRecoverableOutput = strings.TrimSpace(output)
}

func shouldBufferPersistenceAssistantText(policy persistencePolicy, st persistenceState) bool {
	if !policy.Enabled {
		return false
	}
	if shouldBufferAssistantText(policy.Web, st.Web) {
		return true
	}
	if st.LastRecoverableError {
		return true
	}
	return policy.RequireVerification && st.ActionToolSeen && !st.VerificationSeen
}

func shouldForcePersistenceContinuation(policy persistencePolicy, st persistenceState) bool {
	if !shouldBufferPersistenceAssistantText(policy, st) {
		return false
	}
	if st.ForcedContinuations >= policy.MaxForcedContinuations {
		return false
	}
	if st.LastRecoverableError && st.SameFailureRetries > policy.MaxSameFailureRetries {
		return false
	}
	if st.RepeatedAssistantCount >= maxRepeatedAssistantTurns {
		return false
	}
	return true
}

func buildPersistenceFollowUpPrompt(policy persistencePolicy, st persistenceState) string {
	if shouldBufferAssistantText(policy.Web, st.Web) {
		return buildWebEvidenceFollowUpPrompt(policy.Web, st.Web)
	}
	if st.LastRecoverableError {
		return buildRecoverableToolErrorPrompt(policy, st)
	}
	if policy.RequireVerification && st.ActionToolSeen && !st.VerificationSeen {
		return buildVerificationFollowUpPrompt()
	}
	return "Do not finalize your answer yet. Continue with the next concrete tool call needed to complete and verify the task."
}

func buildPersistenceFailureMessage(policy persistencePolicy, st persistenceState) string {
	if shouldBufferAssistantText(policy.Web, st.Web) {
		return buildWebEvidenceFailureMessage()
	}
	if st.LastRecoverableError {
		return fmt.Sprintf("Execution could not make progress after retrying. Last failing tool: %s. Last error: %s", st.LastRecoverableTool, compactOneLine(st.LastRecoverableOutput, 600))
	}
	if policy.RequireVerification && st.ActionToolSeen && !st.VerificationSeen {
		return "Verification was required but no successful verification step was completed. I am not marking the task complete until a check/test/status command succeeds."
	}
	return "Persistence checks did not pass, and no safe continuation remains."
}

func buildRecoverableToolErrorPrompt(policy persistencePolicy, st persistenceState) string {
	remaining := max(0, policy.MaxSameFailureRetries-st.SameFailureRetries+1)
	var sb strings.Builder
	sb.WriteString("Do not finalize your answer yet.\n")
	sb.WriteString("Resume directly. Do not apologize, recap, or ask the user to run commands manually.\n")
	sb.WriteString(fmt.Sprintf("The previous %s tool call failed with a recoverable error. Try a different concrete approach now.\n", st.LastRecoverableTool))
	if remaining > 0 {
		sb.WriteString(fmt.Sprintf("You have %d same-failure retry budget left. Do not repeat the exact same tool input after the same error.\n", remaining))
	}
	sb.WriteString("Use a narrower diagnostic command, an alternate command, or inspect the environment before retrying. If permission or user input is truly required, state that explicitly.")
	return sb.String()
}

func buildVerificationFollowUpPrompt() string {
	return "Do not finalize your answer yet.\nResume directly. Run a concrete verification step now: test, build, status, health check, or a read-only command that proves the requested change or operation succeeded. Then answer only after the verification result is available."
}

func isActionToolCall(call llm.ToolUseStart, registry *toolpkg.Registry) bool {
	name := strings.ToLower(strings.TrimSpace(call.Name))
	switch name {
	case "file_write", "server_copy", "server_connect", "server_tunnel":
		return true
	case "bash", "powershell":
		cmd := strings.ToLower(extractToolInputString(call.Input, "command"))
		return hasAnyTerm(cmd, []string{
			" install ", " apt ", " yum ", " dnf ", " choco ", " winget ",
			"npm install", "pnpm install", "go get", "go mod tidy",
			"write", "edit", "sed -i", "copy", "move", "mkdir", "touch",
			"systemctl restart", "service ", "docker run", "kubectl apply",
		})
	}
	if registry == nil {
		return false
	}
	t, ok := registry.Lookup(call.Name)
	return ok && !t.IsReadOnly()
}

func isVerificationToolCall(call llm.ToolUseStart) bool {
	name := strings.ToLower(strings.TrimSpace(call.Name))
	switch name {
	case "server_metrics", "server_inspect", "grep", "glob", "fileread", "file_read", "fs_list":
		return true
	case "bash", "powershell":
		cmd := strings.ToLower(extractToolInputString(call.Input, "command"))
		return hasAnyTerm(cmd, []string{
			"test", "go test", "npm test", "pnpm test", "pytest", "cargo test",
			"build", "go build", "npm run build", "pnpm build",
			"status", "health", "check", "verify", "curl", "ping",
			"mysqladmin ping", "show ", "select ", "systemctl status",
		})
	default:
		return false
	}
}

func isRecoverableToolError(output string) bool {
	lowered := strings.ToLower(strings.TrimSpace(output))
	if lowered == "" {
		return false
	}
	terminalTerms := []string{
		"permission denied", "requires approval", "user rejected", "blocked",
		"workflow phase", "task_plan_init", "tool not found", "no such tool",
		"not allowed", "approval required",
	}
	if hasAnyTerm(lowered, terminalTerms) {
		return false
	}
	return hasAnyTerm(lowered, []string{
		"exit status", "failed", "error", "timeout", "timed out", "connection",
		"not found", "no such file", "cannot", "refused", "unreachable",
	})
}

func toolFailureKey(toolName string, output string) string {
	key := strings.ToLower(strings.TrimSpace(toolName)) + ":" + compactOneLine(output, 240)
	return strings.ToLower(key)
}

func extractToolInputString(input json.RawMessage, key string) string {
	var obj map[string]any
	if err := json.Unmarshal(input, &obj); err != nil {
		return ""
	}
	v, _ := obj[key].(string)
	return strings.TrimSpace(v)
}

func compactOneLine(s string, maxChars int) string {
	oneLine := strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if maxChars > 0 && len(oneLine) > maxChars {
		return oneLine[:maxChars] + "..."
	}
	return oneLine
}
