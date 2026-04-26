package tui

import "time"

type activeToolState struct {
	toolID           string
	toolName         string
	target           string
	args             map[string]any
	startTime        time.Time
	shellLines       []string
	shellTotal       int
	backgrounded     bool
}

type activeToolMap struct {
	ids    []string
	states map[string]*activeToolState
}

func (m *activeToolMap) Add(toolID, toolName, target string, args map[string]any) {
	if m.states == nil {
		m.states = make(map[string]*activeToolState)
	}
	m.ids = append(m.ids, toolID)
	clonedArgs := make(map[string]any, len(args))
	for k, v := range args {
		clonedArgs[k] = v
	}
	m.states[toolID] = &activeToolState{
		toolID:    toolID,
		toolName:  toolName,
		target:    target,
		args:      clonedArgs,
		startTime: time.Now(),
	}
}

func (m *activeToolMap) AppendOutput(toolID, line string) {
	if s, ok := m.states[toolID]; ok {
		s.shellTotal++
		s.shellLines = appendShellLine(s.shellLines, line, shellBoxMaxLines)
	}
}

func (m *activeToolMap) Remove(toolID string) {
	delete(m.states, toolID)
	for i, id := range m.ids {
		if id == toolID {
			m.ids = append(m.ids[:i], m.ids[i+1:]...)
			return
		}
	}
}

func (m *activeToolMap) RunningCount() int {
	return len(m.ids)
}

func (m *activeToolMap) MostRecent() *activeToolState {
	if len(m.ids) == 0 {
		return nil
	}
	return m.states[m.ids[len(m.ids)-1]]
}

func (m *activeToolMap) MostRecentForeground() *activeToolState {
	for i := len(m.ids) - 1; i >= 0; i-- {
		s := m.states[m.ids[i]]
		if !s.backgrounded {
			return s
		}
	}
	return nil
}

func (m *activeToolMap) IsBackgrounded(toolID string) bool {
	if s, ok := m.states[toolID]; ok {
		return s.backgrounded
	}
	return false
}

func (m *activeToolMap) BackgroundAll() int {
	count := 0
	for _, s := range m.states {
		if !s.backgrounded {
			s.backgrounded = true
			count++
		}
	}
	return count
}

func (m *activeToolMap) BackgroundCount() int {
	count := 0
	for _, s := range m.states {
		if s.backgrounded {
			count++
		}
	}
	return count
}

func (m *activeToolMap) ForegroundTools() []*activeToolState {
	out := make([]*activeToolState, 0, len(m.ids))
	for _, id := range m.ids {
		if s := m.states[id]; s != nil && !s.backgrounded {
			out = append(out, s)
		}
	}
	return out
}

func (m *activeToolMap) Clear() {
	m.ids = nil
	m.states = nil
}
