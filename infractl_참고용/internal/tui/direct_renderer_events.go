package tui

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

func (r *DirectRenderer) OnThinking(tier string, model string) {
	r.mu.Lock()
	r.thinkBuf = ""
	r.mu.Unlock()
	r.SetShimmerHint("")

	r.thinkingLabel = ThinkingLabel(tier, model)
	r.StartShimmer(r.thinkingLabel)
}

func (r *DirectRenderer) OnThinkingToken(token string) {
	r.RecordActivity()
	r.mu.Lock()
	r.thinkBuf += token
	hint := thinkHint(r.thinkBuf, 60)
	r.mu.Unlock()

	r.SetShimmerHint(hint)
}

func (r *DirectRenderer) OnToken(token string) {
	r.RecordActivity()
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isStreaming {
		r.stopShimmerInternal()
		r.isStreaming = true
		r.box.SetTitle("response")
		r.box.Reset()
	}

	r.tokens += token

	if time.Since(r.lastRender) > 150*time.Millisecond {
		r.renderStreamToBox()
		r.lastRender = time.Now()
	}
}

func (r *DirectRenderer) OnResponse(content string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stopShimmerInternal()

	finalContent := content
	if r.isStreaming && content == "" && r.tokens != "" {
		finalContent = r.tokens
	}

	if finalContent != "" {
		lines := r.md.Render(finalContent)
		for i, l := range lines {
			lines[i] = "   " + l
		}
		text := strings.Join(lines, "\n")
		r.box.Reset()
		r.box.PrintPermanent(text)
	}

	r.isStreaming = false
	r.tokens = ""
	r.streamCache.Reset()
}

func (r *DirectRenderer) OnToolStart(toolID, name, target string, args map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stopShimmerInternal()
	r.shellArgs = args
	r.shellLines = nil
	r.shellTotal = 0
	r.shellToolName = name
	r.startedAt = time.Now()

	desc, _ := args["description"].(string)
	if phaseID, phaseName, ok := parsePhaseFromDescription(desc); ok {
		r.progress.AddToolWithPhase(toolID, name, target, phaseID, phaseName, args)
	} else {
		r.progress.AddTool(toolID, name, target, args)
	}

	if r.isStreaming {
		lines := r.md.Render(r.tokens)
		for i, l := range lines {
			lines[i] = "   " + l
		}
		text := strings.Join(lines, "\n")
		r.box.PrintPermanent(text)
		r.streamCache.Reset()
		r.isStreaming = false
		r.tokens = ""
	}

	headerLine := buildToolHeaderLine(name, target, args)
	headerLine = StyleResponseBracket.Render("  * ") + headerLine
	r.box.Reset()
	r.box.SetInfo(shellBoxStatusLine(name, args, 0, boxIconRunning, 0, ""))
	r.box.PrintPermanent(headerLine)
	label := r.thinkingLabel
	if label == "" {
		label = "thinking..."
	}
	r.startShimmerInternal(label)
}

func (r *DirectRenderer) OnToolOutput(toolID, line string) {
	r.RecordActivity()
	r.mu.Lock()
	defer r.mu.Unlock()

	r.progress.SetOutput(toolID, line)
	trimmed := strings.TrimRight(line, "\r\n")
	if trimmed != "" {
		r.shellTotal++
		r.shellLines = appendShellLine(r.shellLines, trimmed, shellBoxMaxLines)
		r.box.SetInfo(shellBoxStatusLine(r.shellToolName, r.shellArgs, r.shellTotal, boxIconRunning, 0, ""))
		r.box.SetContentWithTotal(r.shellLines, r.shellTotal)
		r.box.Redraw()
	}
}

func (r *DirectRenderer) OnToolEnd(toolID, name, result string, duration time.Duration, success bool, metadataJSON string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stopShimmerInternal()
	r.progress.CompleteTool(toolID, duration, success, metadataJSON)
	if name == "verify_complete" {
		if meta, ok := parseTaskProgressMetadata(metadataJSON); ok && strings.TrimSpace(meta.VerifiedByToolID) != "" {
			r.progress.UpdateTaskProgress(meta.VerifiedByToolID, metadataJSON)
		}
	}

	var args map[string]any
	for i := len(r.progress.items) - 1; i >= 0; i-- {
		if r.progress.items[i].toolID == toolID {
			args = r.progress.items[i].args
			break
		}
	}

	contentLines, totalLines := resolveToolBoxContent(name, args, r.shellLines, r.shellTotal, result, success)

	r.box.Reset()
	r.box.PrintPermanent(renderShellBoxCompleted(name, args, contentLines, totalLines, duration, success, r.width, metadataJSON))

	r.shellArgs = nil
	r.shellLines = nil
	r.shellTotal = 0
	r.shellToolName = ""
	r.startedAt = time.Time{}

	label := r.thinkingLabel
	if label == "" {
		label = "thinking..."
	}
	r.startShimmerInternal(label)
}

func (r *DirectRenderer) OnGuardViolation(toolName, reason string, blocked bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stopShimmerInternal()

	if !blocked {
		// 경고일 때는 알림만 출력
		bracket := StyleResponseBracket.Render("  > ")
		r.box.PrintPermanent(bracket + StyleWarning.Render(fmt.Sprintf("[TaskGuard 경고] %s — %s", toolName, reason)))
		return
	}

	// 차단(Blocked)일 때는 도구 박스 상태를 GUARD로 렌더링
	duration := time.Duration(0)
	if !r.startedAt.IsZero() {
		duration = time.Since(r.startedAt) // 대략적인 시간
	}
	lines := []string{reason}
	totalLines := 1

	r.box.Reset()
	r.box.PrintPermanent(renderShellBoxGuard(toolName, r.shellArgs, lines, totalLines, duration, r.width))

	r.shellArgs = nil
	r.shellLines = nil
	r.shellTotal = 0
	r.shellToolName = ""
	r.startedAt = time.Time{}

	label := r.thinkingLabel
	if label == "" {
		label = "thinking..."
	}
	r.startShimmerInternal(label)
}

func (r *DirectRenderer) OnToolCancelled(toolID, name, reason string, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stopShimmerInternal()
	r.progress.CompleteTool(toolID, duration, false, "") // progress 에서는 실패 처리하되 UI 만 다르게 표시

	var args map[string]any
	for i := len(r.progress.items) - 1; i >= 0; i-- {
		if r.progress.items[i].toolID == toolID {
			args = r.progress.items[i].args
			break
		}
	}

	lines := []string{reason}
	totalLines := 1

	r.box.Reset()
	r.box.PrintPermanent(renderShellBoxCancelled(name, args, lines, totalLines, duration, r.width))

	r.shellArgs = nil
	r.shellLines = nil
	r.shellTotal = 0
	r.shellToolName = ""
	r.startedAt = time.Time{}

	label := r.thinkingLabel
	if label == "" {
		label = "thinking..."
	}
	r.startShimmerInternal(label)
}

func (r *DirectRenderer) OnError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stopShimmerInternal()
	slog.Error("agent error in DirectRenderer", "err", err)
	errText := StyleError.Render("  Error: " + err.Error())
	r.box.PrintPermanent(errText)
}

func (r *DirectRenderer) OnUsageUpdate(inputTokens, outputTokens int, costUSD float64) {
	r.state.AddUsage(inputTokens, outputTokens, costUSD)
}

func (r *DirectRenderer) Finish() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stopShimmerInternal()

	if r.isStreaming {
		lines := r.md.Render(r.tokens)
		for i, l := range lines {
			lines[i] = "   " + l
		}
		text := strings.Join(lines, "\n")
		r.box.PrintPermanent(text)
		r.isStreaming = false
		r.tokens = ""
		r.streamCache.Reset()
	}

	r.box.Clear()
	fmt.Fprintln(os.Stdout, StatusLine(r.state))
	r.progress.Reset()
}

func (r *DirectRenderer) PrintNotification(text string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.shimmerMu.Lock()
	wasShimming := r.shimmerOn
	r.shimmerMu.Unlock()

	r.stopShimmerInternal()
	r.box.PrintPermanent(text)

	if wasShimming {
		label := r.thinkingLabel
		if label == "" {
			label = "thinking..."
		}
		r.startShimmerInternal(label)
	}
}

func (r *DirectRenderer) renderStreamToBox() {
	rendered := r.md.RenderStreaming(r.tokens, &r.streamCache)
	if len(rendered) == 0 {
		return
	}
	r.box.SetContent(rendered)
	r.box.Redraw()
}

func buildToolHeaderLine(name, target string, args map[string]any) string {
	bullet := StyleClaude().Render("*")
	toolName := StyleCmdBoxToolName.Render(toolDisplayName(name))
	arg := toolDisplayArg(name, args)
	if arg != "" {
		return bullet + " " + toolName + StyleCmdBoxDim.Render("(") + arg + StyleCmdBoxDim.Render(")")
	}
	if target != "" && target != "localhost" {
		return bullet + " " + toolName + StyleCmdBoxDim.Render(" -> ") + StyleCmdBoxTarget.Render(target)
	}
	return bullet + " " + toolName
}
