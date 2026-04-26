// Package tui
// File: progress_phase_view.go
// Description: Phase 그룹 기반 진행 트리 렌더링
// Responsibility: phases가 있을 때 Phase별로 접힘/펼침 형태로 진행 상황 표시

package tui

import (
	"strings"
	"time"
)

// Phase 상태 아이콘
const (
	phaseIconDone    = ""
	phaseIconRunning = "■"
	phaseIconError   = "✗"
	phaseIconPending = "□"
)

// renderPhaseView는 Phase 기반 진행 트리를 렌더링한다.
//
//	✓ Phase A: internal/cache/ 제네릭 LRU 캐시 구현
//	✓ Phase B: pipeline.go 마이그레이션
//	■ Phase C: internal/connector/pool/ 연동
//	   ├─ shell_exec → myserver ✓ (3.2s)
//	   └─ file_write → myserver (1.1s)
//	□ Phase D: executor/parallel.go 멀티타겟
func renderPhaseView(phases []phase, items []progressItem) string {
	if len(phases) == 0 {
		return ""
	}

	var b strings.Builder

	for _, ph := range phases {
		icon := phaseStatusIcon(ph.status)
		renderFn := phaseStatusStyle(ph.status)

		b.WriteString(treeIndent + treeBranchPad + " ")
		b.WriteString(renderFn(icon))
		b.WriteString(" Phase " + ph.id + ": ")
		if ph.status == statusDone {
			b.WriteString(StyleCmdBoxDim.Render(ph.description))
		} else {
			b.WriteString(StyleThinking.Render(ph.description))
		}
		b.WriteString("\n")

		// 실행 중인 Phase만 도구 세부 정보 펼침
		if ph.status == statusRunning || ph.status == statusWaiting {
			phaseTools := filterToolsByIDs(items, ph.tools)
			for i, item := range phaseTools {
				isLast := i == len(phaseTools)-1
				b.WriteString(renderPhaseToolItem(item, isLast))
				b.WriteString("\n")
			}
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// renderPhaseToolItem은 Phase 내부의 단일 도구 항목을 렌더링한다.
func renderPhaseToolItem(item progressItem, isLast bool) string {
	branch := treeBranch
	if isLast {
		branch = treeLastBranch
	}

	var b strings.Builder
	indent := treeIndent + treeBranchPad + "    "
	b.WriteString(indent + StyleTreeLine.Render(branch) + " ")
	b.WriteString(StyleCmdBoxToolName.Render(item.toolName))

	if item.target != "" && item.target != "localhost" {
		b.WriteString(StyleCmdBoxDim.Render(" → ") + StyleCmdBoxTarget.Render(item.target))
	}

	switch item.status {
	case statusDone:
		elapsed := formatElapsedShort(item.duration)
		b.WriteString(" " + StyleCmdBoxDim.Render("("+elapsed+")"))
	case statusWaiting:
		elapsed := formatElapsedShort(item.duration)
		b.WriteString(" " + StyleCmdBoxDim.Render("(검증 대기, "+elapsed+")"))
	case statusError:
		elapsed := formatElapsedShort(item.duration)
		b.WriteString(" " + StyleError.Render("✗") + " " +
			StyleCmdBoxDim.Render("("+elapsed+")"))
	case statusRunning:
		elapsed := formatElapsedShort(time.Since(item.startTime))
		b.WriteString(" " + StyleCmdBoxDim.Render("("+elapsed+")"))
	}

	return b.String()
}

func phaseStatusIcon(status progressStatus) string {
	switch status {
	case statusDone:
		return phaseIconDone
	case statusError:
		return phaseIconError
	case statusRunning:
		return phaseIconRunning
	case statusWaiting:
		return phaseIconPending
	default:
		return phaseIconPending
	}
}

func phaseStatusStyle(status progressStatus) func(strs ...string) string {
	switch status {
	case statusDone:
		return StyleSuccess.Render
	case statusError:
		return StyleError.Render
	case statusRunning:
		return StyleClaude().Render
	case statusWaiting:
		return StyleCmdBoxDim.Render
	default:
		return StyleCmdBoxDim.Render
	}
}

func filterToolsByIDs(items []progressItem, ids []string) []progressItem {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var result []progressItem
	for _, item := range items {
		if idSet[item.toolID] {
			result = append(result, item)
		}
	}
	return result
}

// formatElapsedShort은 shell_render.go에 정의되어 있다.
