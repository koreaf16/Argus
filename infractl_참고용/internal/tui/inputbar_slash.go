package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type slashMenuState struct {
	active   bool
	selected int
	items    []slashCommandDef
	start    int
	end      int
}

func (ib *inputBar) clearSlashSuggestions() {
	ib.slashMenu = slashMenuState{}
}

func (ib *inputBar) hasSlashSuggestions() bool {
	return ib.slashMenu.active && len(ib.slashMenu.items) > 0
}

func (ib *inputBar) refreshSlashSuggestions() {
	if ib.searchMode || ib.idleMode || ib.isPassword {
		ib.clearSlashSuggestions()
		return
	}

	value := ib.ti.Value()
	if value == "" || strings.ContainsRune(value, '\n') {
		ib.clearSlashSuggestions()
		return
	}
	if strings.HasPrefix(value, " ") || strings.HasPrefix(value, "\t") {
		ib.clearSlashSuggestions()
		return
	}
	if !strings.HasPrefix(value, "/") {
		ib.clearSlashSuggestions()
		return
	}
	if len(value) > 1 && value[1] == '/' {
		ib.clearSlashSuggestions()
		return
	}

	cursor := ib.cursorByteOffset()
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(value) {
		cursor = len(value)
	}

	tokenEnd := len(value)
	if idx := strings.IndexAny(value, " \t"); idx >= 0 {
		tokenEnd = idx
	}
	if cursor > tokenEnd {
		ib.clearSlashSuggestions()
		return
	}

	prefix := value[:tokenEnd]
	matches := slashCommandMatchesPrefix(prefix)
	if len(matches) == 0 {
		ib.clearSlashSuggestions()
		return
	}

	selected := 0
	if ib.hasSlashSuggestions() && ib.slashMenu.selected >= 0 && ib.slashMenu.selected < len(ib.slashMenu.items) {
		current := slashCommandDisplayName(ib.slashMenu.items[ib.slashMenu.selected])
		for i, item := range matches {
			if slashCommandDisplayName(item) == current {
				selected = i
				break
			}
		}
	}

	ib.slashMenu = slashMenuState{
		active:   true,
		selected: selected,
		items:    matches,
		start:    0,
		end:      tokenEnd,
	}
}

func (ib *inputBar) moveSlashSelection(delta int) {
	if !ib.hasSlashSuggestions() {
		return
	}
	total := len(ib.slashMenu.items)
	if total == 0 {
		return
	}
	selected := ib.slashMenu.selected + delta
	selected %= total
	if selected < 0 {
		selected += total
	}
	ib.slashMenu.selected = selected
}

func (ib *inputBar) applySlashSuggestion(def slashCommandDef) {
	start := ib.slashMenu.start
	end := ib.slashMenu.end
	value := ib.ti.Value()
	if start < 0 || end < start || end > len(value) {
		return
	}

	newValue := value[:start] + def.Name + value[end:]
	ib.ti.SetValue(newValue)
	ib.ti.CursorEnd()
}

func (ib *inputBar) acceptSlashSuggestion() {
	if !ib.hasSlashSuggestions() {
		return
	}
	idx := ib.slashMenu.selected
	if idx < 0 || idx >= len(ib.slashMenu.items) {
		idx = 0
	}
	ib.applySlashSuggestion(ib.slashMenu.items[idx])
	ib.clearSlashSuggestions()
}

func (ib *inputBar) handleSlashSuggestionKey(msg tea.KeyMsg) bool {
	if !ib.hasSlashSuggestions() {
		return false
	}

	switch msg.Type {
	case tea.KeyEsc:
		ib.clearSlashSuggestions()
		return true
	case tea.KeyUp:
		ib.moveSlashSelection(-1)
		return true
	case tea.KeyDown:
		ib.moveSlashSelection(1)
		return true
	case tea.KeyTab:
		ib.acceptSlashSuggestion()
		return true
	case tea.KeyEnter:
		if !IsSlashCommand(ib.ti.Value()) {
			ib.acceptSlashSuggestion()
			return true
		}
	}

	return false
}

func (ib *inputBar) renderSlashSuggestions() string {
	if !ib.hasSlashSuggestions() {
		return ""
	}

	items := ib.slashMenu.items
	const maxItems = 6
	extra := 0
	if len(items) > maxItems {
		extra = len(items) - maxItems
		items = items[:maxItems]
	}

	width := ib.width - 8
	if width < 24 {
		width = 24
	}

	var sb strings.Builder
	sb.WriteString("  " + StyleSelectionHint.Render("Commands"))
	for i, item := range items {
		sb.WriteByte('\n')
		selected := i == ib.slashMenu.selected
		name := item.Name
		if selected {
			sb.WriteString("  " + StyleSelectionCursor.Render(">") + " " + StyleSelectionSelected.Render(name))
		} else {
			sb.WriteString("    " + StyleSelectionOption.Render(name))
		}

		if item.Description != "" {
			desc := truncateRunes(item.Description, width-len(name)-6)
			if desc != "" {
				sb.WriteString(StyleSelectionSub.Render(" - " + desc))
			}
		}
	}

	if extra > 0 {
		sb.WriteString("\n" + StyleSelectionSub.Render(fmt.Sprintf("  +%d more", extra)))
	}

	return sb.String()
}

func truncateRunes(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}
