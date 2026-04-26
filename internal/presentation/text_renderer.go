package presentation

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

type TextRenderer struct {
	w io.Writer

	debug bool

	mu              sync.Mutex
	assistantOpen   bool
	assistantBuffer strings.Builder
}

func NewTextRenderer(w io.Writer, debug bool) *TextRenderer {
	return &TextRenderer{
		w:     w,
		debug: debug,
	}
}

func (r *TextRenderer) Emit(evt Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.debug {
		switch evt.Kind {
		case EventAssistantDelta:
			r.assistantBuffer.WriteString(evt.Text)
			return
		case EventAssistantDone:
			fullText := r.assistantBuffer.String()
			r.assistantBuffer.Reset()
			if strings.TrimSpace(fullText) != "" {
				r.emitLines(DebugLines(Event{Kind: EventAssistantText, Text: fullText}))
			}
			r.emitLines(DebugLines(evt))
			return
		default:
			r.emitLines(DebugLines(evt))
			return
		}
	}

	switch evt.Kind {
	case EventAssistantDelta:
		if !r.assistantOpen {
			fmt.Fprintln(r.w, "assistant")
			r.assistantOpen = true
		}
		fmt.Fprint(r.w, evt.Text)
		return
	case EventAssistantDone:
		if r.assistantOpen {
			fmt.Fprintln(r.w)
			fmt.Fprintln(r.w)
			r.assistantOpen = false
		}
		return
	default:
		if r.assistantOpen {
			fmt.Fprintln(r.w)
			fmt.Fprintln(r.w)
			r.assistantOpen = false
		}
		r.emitLines(HumanLines(evt))
	}
}

func (r *TextRenderer) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.assistantOpen {
		fmt.Fprintln(r.w)
		fmt.Fprintln(r.w)
		r.assistantOpen = false
	}
}

func (r *TextRenderer) emitLines(lines []string) {
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			fmt.Fprintln(r.w)
			continue
		}
		fmt.Fprintln(r.w, line)
	}
}
