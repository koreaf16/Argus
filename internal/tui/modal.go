package tui

import (
	"errors"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/koreaf16/argus/internal/services/workspace"
	tool "github.com/koreaf16/argus/internal/tools"
)

type modalKind int

const (
	modalNone modalKind = iota
	modalApproval
	modalConfirm
	modalPassword
	modalAskUser
	modalAskUserBatch
	modalServerForm
	modalServerList
)

// serverFormAuthMode controls which auth-specific fields are shown.
type serverFormAuthMode int

const (
	serverFormAuthAgent    serverFormAuthMode = iota // SSH agent
	serverFormAuthKey                                // identity file
	serverFormAuthPassword                           // password
)

// serverFormField enumerates the focusable fields in the server-add form.
// Fields for identity file, password, and savePassword are conditionally shown
// depending on the selected authMode.
type serverFormField int

const (
	sfAlias serverFormField = iota
	sfHost
	sfPort
	sfUser
	sfAuthMode
	sfIdentityFile
	sfPassword
	sfSavePassword
	sfDefaultCWD
	sfSubmit
	sfFieldCount
)

// serverFormState holds all mutable state for the server-registration form modal.
type serverFormState struct {
	EditAlias    string // non-empty when editing an existing server
	FocusIdx     serverFormField
	Alias        string
	Host         string
	PortStr      string
	User         string
	AuthMode     serverFormAuthMode
	IdentityFile string
	Password     string
	SavePassword bool
	DefaultCWD   string
	ErrorMsg     string
	ErrorField   serverFormField
}

// serverAction enumerates actions available in the server list action sub-menu.
type serverAction int

const (
	actionConnect    serverAction = iota
	actionDisconnect              // shown only when connected
	actionEdit
	actionDelete
)

// serverActionState holds the state of the action sub-menu for a selected server.
type serverActionState struct {
	Alias  string
	Cursor int // index into visibleActions slice
}

// serverListState holds all mutable state for the interactive server list panel.
type serverListState struct {
	Entries       []workspace.StatusEntry
	Cursor        int // 0..len(Entries); len(Entries) == "Add New Server" row
	ShowAction    bool
	Action        serverActionState
	PendingDelete string // non-empty → waiting for y/n delete confirmation
	ErrorMsg      string
}

// serverListResult is the value sent on ServerListC when the panel closes.
type serverListResult struct {
	Action    string // "closed", "add", "edit"
	EditAlias string
	Err       error
}

// serverListRequestMsg triggers opening the server list panel.
type serverListRequestMsg struct {
	Response chan serverListResult
}

// serverListRefreshMsg is sent from goroutines (connect/disconnect/delete) to
// re-snapshot workspace status into the open server list panel.
type serverListRefreshMsg struct {
	ErrorMsg string
}

type modalState struct {
	Kind      modalKind
	Title     string
	Prompt    string
	ToolName  string
	ToolType  string // "edit", "exec", "mcp", etc.
	InputJSON string
	Password  string
	DefaultNo bool
	DecisionC chan decisionResult
	PasswordC chan passwordResult

	// Specialized fields for Gemini-like UI
	DiffContent string
	Commands    []string
	PlanPath    string
	Markdown    string

	Question    *tool.AskUserQuestion
	AskInput    string
	AskCursor   int
	AskSelected map[int]bool
	AskUserC    chan askUserResult

	Questions         []tool.AskUserQuestion
	AskAnswersByIndex map[string]string
	AskAnswersByID    map[string]string
	AskTab            int
	AskError          string
	AskUserBatchC     chan askUserBatchResult

	ServerForm  *serverFormState
	ServerFormC chan serverFormResult

	ServerList  *serverListState
	ServerListC chan serverListResult
}

func (m *uiModel) enqueueModal(md modalState) {
	if md.Kind == modalAskUser {
		initializeAskUserModal(&md)
	}
	if md.Kind == modalAskUserBatch {
		initializeAskUserBatchModal(&md)
	}
	if m.modal.Kind == modalNone {
		m.modal = md
		return
	}
	m.modalQueue = append(m.modalQueue, md)
}

func (m *uiModel) closeModal() {
	m.modal = modalState{}
	if len(m.modalQueue) > 0 {
		m.modal = m.modalQueue[0]
		m.modalQueue = m.modalQueue[1:]
	}
}

func (m *uiModel) handleModalKey(msg tea.KeyMsg) {
	switch m.modal.Kind {
	case modalApproval, modalConfirm:
		m.handleApprovalModalKey(msg)
	case modalPassword:
		m.handlePasswordModalKey(msg)
	case modalAskUser:
		m.handleAskUserModalKey(msg)
	case modalAskUserBatch:
		m.handleAskUserBatchModalKey(msg)
	case modalServerForm:
		m.handleServerFormKey(msg)
	case modalServerList:
		m.handleServerListKey(msg)
	}
}

func (m *uiModel) handleApprovalModalKey(msg tea.KeyMsg) {
	if m.modal.Kind == modalConfirm {
		if m.modal.DefaultNo && m.modal.AskCursor == 0 {
			m.modal.AskCursor = 1
		}
		switch msg.String() {
		case "y", "Y":
			if m.modal.DecisionC != nil {
				m.modal.DecisionC <- decisionResult{Allow: true, Scope: approvalScopeOnce}
			}
			m.closeModal()
		case "n", "N":
			if m.modal.DecisionC != nil {
				m.modal.DecisionC <- decisionResult{Allow: false}
			}
			m.closeModal()
		case "up", "ctrl+p", "down", "ctrl+n":
			if m.modal.AskCursor == 0 {
				m.modal.AskCursor = 1
			} else {
				m.modal.AskCursor = 0
			}
		case "enter":
			allow := m.modal.AskCursor == 0
			if m.modal.DecisionC != nil {
				m.modal.DecisionC <- decisionResult{Allow: allow, Scope: approvalScopeOnce}
			}
			m.closeModal()
		case "esc":
			if m.modal.DecisionC != nil {
				m.modal.DecisionC <- decisionResult{Allow: false}
			}
			m.closeModal()
		}
		return
	}

	options := 4
	if m.modal.AskCursor < 0 {
		m.modal.AskCursor = 0
	}
	if m.modal.AskCursor >= options {
		m.modal.AskCursor = options - 1
	}
	switch msg.String() {
	case "y", "Y":
		if m.modal.DecisionC != nil {
			m.modal.DecisionC <- decisionResult{Allow: true, Scope: approvalScopeOnce}
		}
		m.closeModal()
	case "n", "N":
		if m.modal.DecisionC != nil {
			m.modal.DecisionC <- decisionResult{Allow: false}
		}
		m.closeModal()
	case "up", "ctrl+p":
		if m.modal.AskCursor == 0 {
			m.modal.AskCursor = options - 1
		} else {
			m.modal.AskCursor--
		}
	case "down", "ctrl+n":
		if m.modal.AskCursor == options-1 {
			m.modal.AskCursor = 0
		} else {
			m.modal.AskCursor++
		}
	case "1", "2", "3", "4":
		m.modal.AskCursor = int(msg.String()[0] - '1')
		if m.modal.AskCursor < 0 {
			m.modal.AskCursor = 0
		}
		if m.modal.AskCursor >= options {
			m.modal.AskCursor = options - 1
		}
		allow := m.modal.AskCursor < 3
		scope := approvalScopeOnce
		switch m.modal.AskCursor {
		case 1:
			scope = approvalScopeSession
		case 2:
			scope = approvalScopePermanent
		}
		if m.modal.DecisionC != nil {
			m.modal.DecisionC <- decisionResult{Allow: allow, Scope: scope}
		}
		m.closeModal()
	case "enter":
		allow := m.modal.AskCursor < 3
		scope := approvalScopeOnce
		switch m.modal.AskCursor {
		case 1:
			scope = approvalScopeSession
		case 2:
			scope = approvalScopePermanent
		}
		if m.modal.DecisionC != nil {
			m.modal.DecisionC <- decisionResult{Allow: allow, Scope: scope}
		}
		m.closeModal()
	case "esc":
		if m.modal.DecisionC != nil {
			m.modal.DecisionC <- decisionResult{Allow: false}
		}
		m.closeModal()
	}
}

func (m *uiModel) handlePasswordModalKey(msg tea.KeyMsg) {
	switch msg.String() {
	case "enter":
		if m.modal.PasswordC != nil {
			m.modal.PasswordC <- passwordResult{Value: m.modal.Password}
		}
		m.closeModal()
	case "esc":
		if m.modal.PasswordC != nil {
			m.modal.PasswordC <- passwordResult{Err: errors.New("password input canceled")}
		}
		m.closeModal()
	case "backspace":
		if len(m.modal.Password) > 0 {
			m.modal.Password = m.modal.Password[:len(m.modal.Password)-1]
		}
	default:
		if len(msg.Runes) > 0 {
			m.modal.Password += string(msg.Runes)
		}
	}
}

func (m *uiModel) handleAskUserModalKey(msg tea.KeyMsg) {
	q := m.modal.Question
	if q == nil {
		m.finishAskUser(tool.AskUserResponse{Canceled: true})
		return
	}

	switch msg.String() {
	case "esc":
		m.finishAskUser(tool.AskUserResponse{Canceled: true})
		return
	}

	qType := normalizeAskQuestionType(q)
	switch qType {
	case "text":
		m.handleAskUserTextKey(msg, q)
	default:
		m.handleAskUserChoiceKey(msg, q)
	}
}

func (m *uiModel) handleAskUserTextKey(msg tea.KeyMsg, q *tool.AskUserQuestion) {
	switch msg.String() {
	case "enter":
		value := strings.TrimSpace(m.modal.AskInput)
		if value == "" {
			value = strings.TrimSpace(q.Default)
		}
		if q.Required && value == "" {
			return
		}
		m.finishAskUser(tool.AskUserResponse{Value: value})
	case "backspace":
		if len(m.modal.AskInput) > 0 {
			m.modal.AskInput = m.modal.AskInput[:len(m.modal.AskInput)-1]
		}
	default:
		if len(msg.Runes) > 0 {
			m.modal.AskInput += string(msg.Runes)
		}
	}
}

func (m *uiModel) handleAskUserChoiceKey(msg tea.KeyMsg, q *tool.AskUserQuestion) {
	options := askQuestionOptions(q)
	if len(options) == 0 {
		m.finishAskUser(tool.AskUserResponse{Value: strings.TrimSpace(q.Default)})
		return
	}
	modalMax := len(options) - 1
	if m.modal.AskCursor < 0 {
		m.modal.AskCursor = 0
	}
	if m.modal.AskCursor > modalMax {
		m.modal.AskCursor = modalMax
	}

	isMulti := q.MultiSelect
	switch msg.String() {
	case "up", "ctrl+p":
		if m.modal.AskCursor == 0 {
			m.modal.AskCursor = modalMax
		} else {
			m.modal.AskCursor--
		}
	case "down", "ctrl+n":
		if m.modal.AskCursor == modalMax {
			m.modal.AskCursor = 0
		} else {
			m.modal.AskCursor++
		}
	case " ":
		if !isMulti {
			return
		}
		if m.modal.AskSelected == nil {
			m.modal.AskSelected = make(map[int]bool)
		}
		if m.modal.AskSelected[m.modal.AskCursor] {
			delete(m.modal.AskSelected, m.modal.AskCursor)
		} else {
			m.modal.AskSelected[m.modal.AskCursor] = true
		}
	case "enter":
		if isMulti {
			values := make([]string, 0, len(m.modal.AskSelected))
			for idx := range m.modal.AskSelected {
				if idx < 0 || idx >= len(options) {
					continue
				}
				values = append(values, optionAnswerValue(options[idx]))
			}
			if len(values) == 0 {
				if q.Required {
					return
				}
				m.finishAskUser(tool.AskUserResponse{Value: ""})
				return
			}
			m.finishAskUser(tool.AskUserResponse{Value: strings.Join(values, ", ")})
			return
		}
		m.finishAskUser(tool.AskUserResponse{Value: optionAnswerValue(options[m.modal.AskCursor])})
	case "y", "Y":
		if normalizeAskQuestionType(q) == "yesno" {
			m.finishAskUser(tool.AskUserResponse{Value: "yes"})
		}
	case "n", "N":
		if normalizeAskQuestionType(q) == "yesno" {
			m.finishAskUser(tool.AskUserResponse{Value: "no"})
		}
	default:
		if len(msg.Runes) == 1 {
			r := msg.Runes[0]
			if r >= '1' && r <= '9' {
				idx := int(r - '1')
				if idx >= 0 && idx < len(options) {
					m.modal.AskCursor = idx
					if !isMulti {
						m.finishAskUser(tool.AskUserResponse{Value: optionAnswerValue(options[idx])})
					}
				}
			}
		}
	}
}

func (m *uiModel) handleAskUserBatchModalKey(msg tea.KeyMsg) {
	if len(m.modal.Questions) == 0 {
		m.finishAskUserBatch(tool.AskUserBatchResponse{Canceled: true, Error: "question payload is missing"})
		return
	}

	switch msg.String() {
	case "esc":
		m.finishAskUserBatch(tool.AskUserBatchResponse{Canceled: true})
		return
	case "tab", "right":
		m.commitAskBatchDraft()
		m.moveAskBatchTab(1)
		return
	case "shift+tab", "left":
		m.commitAskBatchDraft()
		m.moveAskBatchTab(-1)
		return
	}

	if m.isAskBatchReviewTab() {
		if msg.String() == "enter" {
			m.submitAskUserBatch()
		}
		return
	}

	q := m.currentAskBatchQuestion()
	if q == nil {
		return
	}
	m.modal.AskError = ""
	switch normalizeAskQuestionType(q) {
	case "text":
		switch msg.String() {
		case "enter":
			m.commitAskBatchDraft()
			if q.Required && strings.TrimSpace(m.currentAskBatchAnswer()) == "" {
				m.modal.AskError = "answer is required"
				return
			}
			if m.askBatchHasReviewTab() {
				m.moveAskBatchTab(1)
			} else {
				m.submitAskUserBatch()
			}
		case "backspace":
			if len(m.modal.AskInput) > 0 {
				m.modal.AskInput = m.modal.AskInput[:len(m.modal.AskInput)-1]
			}
		default:
			if len(msg.Runes) > 0 {
				m.modal.AskInput += string(msg.Runes)
			}
		}
	default:
		options := askQuestionOptions(q)
		if len(options) == 0 {
			m.setCurrentAskBatchAnswer(strings.TrimSpace(q.Default))
			if m.askBatchHasReviewTab() {
				m.moveAskBatchTab(1)
			} else {
				m.submitAskUserBatch()
			}
			return
		}

		isMulti := q.MultiSelect
		maxCursor := len(options) - 1
		if m.modal.AskCursor < 0 {
			m.modal.AskCursor = 0
		}
		if m.modal.AskCursor > maxCursor {
			m.modal.AskCursor = maxCursor
		}
		switch msg.String() {
		case "up", "ctrl+p":
			if m.modal.AskCursor == 0 {
				m.modal.AskCursor = maxCursor
			} else {
				m.modal.AskCursor--
			}
		case "down", "ctrl+n":
			if m.modal.AskCursor == maxCursor {
				m.modal.AskCursor = 0
			} else {
				m.modal.AskCursor++
			}
		case " ":
			if !isMulti {
				return
			}
			if m.modal.AskSelected == nil {
				m.modal.AskSelected = make(map[int]bool)
			}
			if m.modal.AskSelected[m.modal.AskCursor] {
				delete(m.modal.AskSelected, m.modal.AskCursor)
			} else {
				m.modal.AskSelected[m.modal.AskCursor] = true
			}
			m.commitAskBatchDraft()
		case "enter":
			if isMulti {
				values := make([]string, 0, len(m.modal.AskSelected))
				for idx := range m.modal.AskSelected {
					if idx < 0 || idx >= len(options) {
						continue
					}
					values = append(values, optionAnswerValue(options[idx]))
				}
				if len(values) == 0 && q.Required {
					m.modal.AskError = "at least one choice is required"
					return
				}
				m.setCurrentAskBatchAnswer(strings.Join(values, ", "))
			} else {
				m.setCurrentAskBatchAnswer(optionAnswerValue(options[m.modal.AskCursor]))
			}
			if m.askBatchHasReviewTab() {
				m.moveAskBatchTab(1)
			} else {
				m.submitAskUserBatch()
			}
		case "y", "Y":
			if normalizeAskQuestionType(q) == "yesno" {
				m.setCurrentAskBatchAnswer("yes")
				if m.askBatchHasReviewTab() {
					m.moveAskBatchTab(1)
				} else {
					m.submitAskUserBatch()
				}
			}
		case "n", "N":
			if normalizeAskQuestionType(q) == "yesno" {
				m.setCurrentAskBatchAnswer("no")
				if m.askBatchHasReviewTab() {
					m.moveAskBatchTab(1)
				} else {
					m.submitAskUserBatch()
				}
			}
		default:
			if len(msg.Runes) == 1 {
				r := msg.Runes[0]
				if r >= '1' && r <= '9' {
					idx := int(r - '1')
					if idx >= 0 && idx < len(options) {
						m.modal.AskCursor = idx
						if !isMulti {
							m.setCurrentAskBatchAnswer(optionAnswerValue(options[idx]))
							if m.askBatchHasReviewTab() {
								m.moveAskBatchTab(1)
							} else {
								m.submitAskUserBatch()
							}
						}
					}
				}
			}
		}
	}
}

func (m *uiModel) askBatchHasReviewTab() bool {
	return len(m.modal.Questions) > 1
}

func (m *uiModel) askBatchTabCount() int {
	count := len(m.modal.Questions)
	if m.askBatchHasReviewTab() {
		count++
	}
	return count
}

func (m *uiModel) isAskBatchReviewTab() bool {
	return m.askBatchHasReviewTab() && m.modal.AskTab == len(m.modal.Questions)
}

func (m *uiModel) moveAskBatchTab(delta int) {
	total := m.askBatchTabCount()
	if total <= 0 {
		return
	}
	m.modal.AskError = ""
	next := m.modal.AskTab + delta
	for next < 0 {
		next += total
	}
	for next >= total {
		next -= total
	}
	m.modal.AskTab = next
	m.loadAskBatchCurrentQuestionState()
}

func (m *uiModel) currentAskBatchQuestion() *tool.AskUserQuestion {
	if m.isAskBatchReviewTab() {
		return nil
	}
	if m.modal.AskTab < 0 || m.modal.AskTab >= len(m.modal.Questions) {
		return nil
	}
	return &m.modal.Questions[m.modal.AskTab]
}

func (m *uiModel) currentAskBatchAnswer() string {
	if m.modal.AskAnswersByIndex == nil {
		return ""
	}
	return strings.TrimSpace(m.modal.AskAnswersByIndex[strconv.Itoa(m.modal.AskTab)])
}

func (m *uiModel) setCurrentAskBatchAnswer(value string) {
	if m.modal.AskAnswersByIndex == nil {
		m.modal.AskAnswersByIndex = make(map[string]string)
	}
	if m.modal.AskAnswersByID == nil {
		m.modal.AskAnswersByID = make(map[string]string)
	}
	idx := strconv.Itoa(m.modal.AskTab)
	value = strings.TrimSpace(value)
	if value == "" {
		delete(m.modal.AskAnswersByIndex, idx)
		if q := m.currentAskBatchQuestion(); q != nil && strings.TrimSpace(q.ID) != "" {
			delete(m.modal.AskAnswersByID, q.ID)
		}
		return
	}
	m.modal.AskAnswersByIndex[idx] = value
	if q := m.currentAskBatchQuestion(); q != nil && strings.TrimSpace(q.ID) != "" {
		m.modal.AskAnswersByID[q.ID] = value
	}
}

func (m *uiModel) commitAskBatchDraft() {
	q := m.currentAskBatchQuestion()
	if q == nil {
		return
	}
	switch normalizeAskQuestionType(q) {
	case "text":
		m.setCurrentAskBatchAnswer(strings.TrimSpace(m.modal.AskInput))
	default:
		if !q.MultiSelect {
			return
		}
		options := askQuestionOptions(q)
		if len(options) == 0 {
			return
		}
		values := make([]string, 0, len(m.modal.AskSelected))
		for idx := range m.modal.AskSelected {
			if idx < 0 || idx >= len(options) {
				continue
			}
			values = append(values, optionAnswerValue(options[idx]))
		}
		m.setCurrentAskBatchAnswer(strings.Join(values, ", "))
	}
}

func (m *uiModel) loadAskBatchCurrentQuestionState() {
	q := m.currentAskBatchQuestion()
	m.modal.AskInput = ""
	m.modal.AskCursor = 0
	m.modal.AskSelected = nil
	if q == nil {
		return
	}

	answer := m.currentAskBatchAnswer()
	if answer == "" {
		answer = strings.TrimSpace(q.Default)
	}
	switch normalizeAskQuestionType(q) {
	case "text":
		m.modal.AskInput = answer
	default:
		options := askQuestionOptions(q)
		if len(options) == 0 {
			return
		}
		if q.MultiSelect {
			m.modal.AskSelected = make(map[int]bool)
			for _, tok := range strings.Split(answer, ",") {
				token := strings.TrimSpace(tok)
				if token == "" {
					continue
				}
				for i, opt := range options {
					if strings.EqualFold(token, optionAnswerValue(opt)) || strings.EqualFold(token, strings.TrimSpace(opt.Label)) {
						m.modal.AskSelected[i] = true
						break
					}
				}
			}
			return
		}
		for i, opt := range options {
			if strings.EqualFold(answer, optionAnswerValue(opt)) || strings.EqualFold(answer, strings.TrimSpace(opt.Label)) {
				m.modal.AskCursor = i
				break
			}
		}
	}
}

func (m *uiModel) submitAskUserBatch() {
	if len(m.modal.Questions) == 0 {
		m.finishAskUserBatch(tool.AskUserBatchResponse{Canceled: true, Error: "question payload is missing"})
		return
	}
	m.commitAskBatchDraft()
	for i, q := range m.modal.Questions {
		idx := strconv.Itoa(i)
		value := ""
		if m.modal.AskAnswersByIndex != nil {
			value = strings.TrimSpace(m.modal.AskAnswersByIndex[idx])
		}
		if value == "" {
			value = strings.TrimSpace(q.Default)
		}
		if normalizeAskQuestionType(&q) == "yesno" {
			value = normalizeYesNo(value)
		}
		if q.Required && value == "" {
			m.modal.AskError = "answer is required"
			m.modal.AskTab = i
			m.loadAskBatchCurrentQuestionState()
			return
		}
		if value == "" {
			delete(m.modal.AskAnswersByIndex, idx)
			if strings.TrimSpace(q.ID) != "" {
				delete(m.modal.AskAnswersByID, q.ID)
			}
			continue
		}
		m.modal.AskAnswersByIndex[idx] = value
		if strings.TrimSpace(q.ID) != "" {
			m.modal.AskAnswersByID[q.ID] = value
		}
	}
	resp := tool.AskUserBatchResponse{
		AnswersByIndex: make(map[string]string, len(m.modal.AskAnswersByIndex)),
		AnswersByID:    make(map[string]string, len(m.modal.AskAnswersByID)),
	}
	for k, v := range m.modal.AskAnswersByIndex {
		resp.AnswersByIndex[k] = v
	}
	for k, v := range m.modal.AskAnswersByID {
		resp.AnswersByID[k] = v
	}
	m.finishAskUserBatch(resp)
}

func (m *uiModel) finishAskUser(resp tool.AskUserResponse) {
	if m.modal.AskUserC != nil {
		m.modal.AskUserC <- askUserResult{Response: resp}
	}
	m.closeModal()
}

func (m *uiModel) finishAskUserBatch(resp tool.AskUserBatchResponse) {
	if m.modal.AskUserBatchC != nil {
		m.modal.AskUserBatchC <- askUserBatchResult{Response: resp}
	}
	m.closeModal()
}

func initializeAskUserModal(md *modalState) {
	if md == nil || md.Question == nil {
		return
	}
	q := md.Question
	if normalizeAskQuestionType(q) == "text" {
		md.AskInput = strings.TrimSpace(q.Default)
		return
	}
	options := askQuestionOptions(q)
	if len(options) == 0 {
		return
	}
	md.AskCursor = 0
	def := strings.TrimSpace(q.Default)
	if def == "" {
		return
	}
	for i, opt := range options {
		if strings.EqualFold(def, strings.TrimSpace(opt.Value)) || strings.EqualFold(def, strings.TrimSpace(opt.Label)) {
			md.AskCursor = i
			break
		}
	}
}

func initializeAskUserBatchModal(md *modalState) {
	if md == nil {
		return
	}
	if md.AskAnswersByIndex == nil {
		md.AskAnswersByIndex = make(map[string]string)
	}
	if md.AskAnswersByID == nil {
		md.AskAnswersByID = make(map[string]string)
	}
	for i, q := range md.Questions {
		def := strings.TrimSpace(q.Default)
		if def == "" {
			continue
		}
		idx := strconv.Itoa(i)
		md.AskAnswersByIndex[idx] = def
		if strings.TrimSpace(q.ID) != "" {
			md.AskAnswersByID[q.ID] = def
		}
	}
	md.AskTab = 0
	mm := &uiModel{modal: *md}
	mm.loadAskBatchCurrentQuestionState()
	*md = mm.modal
}

func askQuestionOptions(q *tool.AskUserQuestion) []tool.AskUserOption {
	if q == nil {
		return nil
	}
	if normalizeAskQuestionType(q) == "yesno" && len(q.Options) == 0 {
		return []tool.AskUserOption{
			{Value: "yes", Label: "Yes"},
			{Value: "no", Label: "No"},
		}
	}
	return q.Options
}

func optionAnswerValue(option tool.AskUserOption) string {
	value := strings.TrimSpace(option.Value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(option.Label)
}

func normalizeAskQuestionType(q *tool.AskUserQuestion) string {
	if q == nil {
		return "text"
	}
	t := strings.ToLower(strings.TrimSpace(q.Type))
	switch t {
	case "choice", "yesno", "text":
		return t
	default:
		if len(q.Options) > 0 {
			return "choice"
		}
		return "text"
	}
}

func normalizeYesNo(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "y", "yes", "true", "1":
		return "yes"
	case "n", "no", "false", "0":
		return "no"
	default:
		return strings.TrimSpace(v)
	}
}
