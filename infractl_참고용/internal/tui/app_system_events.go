package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m AppModel) handleSystemMsg(msg tea.Msg) (AppModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case PrivilegePromptRequestMsg:
		m.privilege = privilegePromptState{
			active:  true,
			request: msg.Request,
			replyCh: msg.ReplyCh,
		}
		return m, nil, true

	case PrivilegePromptResponseMsg:
		go func() {
			msg.ReplyCh <- msg.Response
		}()
		return m, nil, true

	case SelectRequestMsg:
		m.selection.ActivateWithHeader(msg.Question, msg.Options, msg.ReplyCh, msg.Header)
		return m, nil, true

	case SelectResponseMsg:
		go func() {
			msg.ReplyCh <- msg.Result
		}()
		m.selection.Deactivate()
		return m, nil, true

	case FormRequestMsg:
		m.form.Activate(msg.Title, msg.Fields, msg.ReplyCh, msg.Header)
		m.input.Reset()
		if len(msg.Fields) > 0 {
			m.input.ti.Placeholder = msg.Fields[0].Placeholder
		}
		return m, nil, true

	case FormResponseMsg:
		go func() {
			msg.ReplyCh <- msg.Result
		}()
		m.form.Deactivate()
		m.input.Reset()
		m.input.ti.Placeholder = ""
		return m, nil, true

	case ActiveServerMsg:
		m.activeServer = msg.Server
		m.statusBar.setActiveServer(msg.Server)
		return m, nil, true

	case JobCompleteMsg:
		if m.box != nil {
			if msg.Success {
				m.box.Println(renderSystemLine(fmt.Sprintf("background job #%d completed: %s", msg.JobID, msg.Description)))
			} else {
				m.box.Println(renderSystemLine(fmt.Sprintf("background job #%d failed: %s", msg.JobID, msg.Description)))
			}
		}
		return m, nil, true

	case StallNoticeMsg:
		if m.box != nil {
			m.box.Println(renderStallNotice(msg.JobID, msg.Tail))
		}
		return m, nil, true

	}

	return m, nil, false
}
