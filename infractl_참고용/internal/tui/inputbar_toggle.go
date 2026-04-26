// Package tui
// File: inputbar_toggle.go
// Description: 입력바 붙여넣기 블록 접기/펼치기 토글 로직
// Responsibility: Space 키로 [Pasted text #N] 플레이스홀더를 펼치거나 다시 접기

package tui

import "strings"

// togglePasteAtCursor는 커서 위치의 붙여넣기 블록을 펼치거나 접는다.
// 동작:
//   - 커서가 [Pasted text #N] 위에 있으면 → 실제 내용으로 펼침
//   - 커서가 펼쳐진 내용 안에 있으면 → [Pasted text #N]으로 다시 접음
//
// 토글이 일어난 경우 true를 반환한다.
func (ib *inputBar) togglePasteAtCursor() bool {
	val := ib.ti.Value()
	if len(ib.pastedContents) == 0 {
		return false
	}

	cursorByte := ib.cursorByteOffset()

	// Phase 1: 커서가 펼친 내용 위에 있으면 → 접기
	for id := range ib.expandedPasteIDs {
		content, ok := ib.pastedContents[id]
		if !ok {
			delete(ib.expandedPasteIDs, id)
			continue
		}
		idx := strings.Index(val, content.Content)
		if idx < 0 {
			// 내용이 편집되어 찾을 수 없음 — 추적 해제
			delete(ib.expandedPasteIDs, id)
			continue
		}
		end := idx + len(content.Content)
		if idx <= cursorByte && cursorByte <= end {
			numLines := getPastedTextRefNumLines(content.Content)
			ref := formatPastedTextRef(id, numLines)
			newVal := val[:idx] + ref + val[end:]
			ib.ti.SetValue(newVal)
			ib.ti.CursorEnd()
			delete(ib.expandedPasteIDs, id)
			ib.refreshSlashSuggestions()
			return true
		}
	}

	// Phase 2: 커서가 [Pasted text #N] 플레이스홀더 위에 있으면 → 펼치기
	refs := parseReferences(val)
	for _, ref := range refs {
		if ref.Index <= cursorByte && cursorByte < ref.Index+len(ref.Match) {
			content, ok := ib.pastedContents[ref.ID]
			if !ok {
				continue
			}
			newVal := val[:ref.Index] + content.Content + val[ref.Index+len(ref.Match):]
			ib.ti.SetValue(newVal)
			ib.ti.CursorEnd()
			if ib.expandedPasteIDs == nil {
				ib.expandedPasteIDs = make(map[int]bool)
			}
			ib.expandedPasteIDs[ref.ID] = true
			ib.refreshSlashSuggestions()
			return true
		}
	}

	return false
}

// cursorByteOffset은 textarea 전체 값에서 현재 커서의 바이트 오프셋을 계산한다.
// ti.Line()은 논리적 줄 번호(실제 \n 기준), ti.LineInfo().CharOffset은 현재 줄 내 룬 오프셋이다.
func (ib *inputBar) cursorByteOffset() int {
	val := ib.ti.Value()
	if val == "" {
		return 0
	}

	lines := strings.Split(val, "\n")
	lineIdx := ib.ti.Line()
	if lineIdx >= len(lines) {
		return len(val)
	}

	// 현재 줄 시작까지의 바이트 오프셋
	byteOffset := 0
	for i := 0; i < lineIdx; i++ {
		byteOffset += len(lines[i]) + 1 // +1 은 \n
	}

	// 현재 줄 안에서 룬 오프셋 → 바이트 오프셋 변환
	currentLine := lines[lineIdx]
	runeOffset := ib.ti.LineInfo().CharOffset
	runeCount := 0
	for i := range currentLine {
		if runeCount >= runeOffset {
			return byteOffset + i
		}
		runeCount++
	}
	return byteOffset + len(currentLine)
}
