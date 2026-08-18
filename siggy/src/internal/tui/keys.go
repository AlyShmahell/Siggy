package tui

import (
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"siggy/src/internal/harness"
)

func (m model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if cmd, ok := m.handleEditKey(msg); ok {
		if m.page == pageSession {
			m.syncMentions()
		}
		if m.page == pageProviderForm {
			m.layout()
		}
		m.syncView()
		return m, cmd
	}
	if msg.String() == "ctrl+c" {
		if m.cancel != nil {
			m.cancel()
		}
		disableEnhancedKeys()
		return m, tea.Quit
	}

	if m.approval != nil {
		return m.listKey(msg)
	}
	if m.float != floatNone && m.float != floatMentions {
		return m.listKey(msg)
	}
	if m.float == floatMentions {
		switch msg.String() {
		case "up", "down", "enter", "esc":
			return m.listKey(msg)
		}
	}
	if m.page == pageSettings {
		return m.settingsKey(msg)
	}
	if m.page == pageProviderForm {
		return m.formKey(msg)
	}
	if m.page != pageSession {
		return m, nil
	}

	if m.focusTrans {
		return m.transKey(msg)
	}

	if isNewlineKey(msg) {
		m.insertText("\n")
		m.syncMentions()
		return m, nil
	}
	switch msg.String() {
	case "enter":
		if msg.Alt {
			m.insertText("\n")
			m.syncMentions()
			return m, nil
		}
		return m.submit()
	case "left":
		m.moveCaret(-1, false)
		return m, nil
	case "right":
		m.moveCaret(1, false)
		return m, nil
	case "up":
		m.moveCaretLine(-1, false)
		return m, nil
	case "down":
		m.moveCaretLine(1, false)
		return m, nil
	case "shift+left":
		m.moveCaret(-1, true)
		return m, nil
	case "shift+right":
		m.moveCaret(1, true)
		return m, nil
	case "shift+up":
		m.moveCaretLine(-1, true)
		return m, nil
	case "shift+down":
		m.moveCaretLine(1, true)
		return m, nil
	case "backspace":
		m.editBackspace()
		m.syncMentions()
		return m, nil
	}
	if isSpaceKey(msg) {
		m.insertText(" ")
		m.syncMentions()
		return m, nil
	}
	if len(msg.Runes) == 1 && msg.Type == tea.KeyRunes {
		m.insertText(string(msg.Runes))
		m.syncMentions()
		return m, nil
	}

	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	m.syncMentions()
	return m, cmd
}

func (m model) transKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.blurTranscript()
		m.syncView()
		return m, nil
	case "left", "shift+left":
		m.moveTransCaret(-1, 0)
		return m, nil
	case "right", "shift+right":
		m.moveTransCaret(1, 0)
		return m, nil
	case "up", "shift+up":
		m.moveTransCaret(0, -1)
		return m, nil
	case "down", "shift+down":
		m.moveTransCaret(0, 1)
		return m, nil
	}
	return m, nil
}

func (m *model) canEditText() bool {
	return m.page == pageSession || m.page == pageProviderForm
}

func (m *model) handleEditKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.approval != nil || !m.canEditText() {
		return nil, false
	}
	if m.focusTrans {
		switch msg.String() {
		case "ctrl+c":
			if m.hasTransSel() {
				return m.transCopy(), true
			}
			return nil, true
		case "ctrl+a":
			m.transSelectAll()
			return nil, true
		case "ctrl+x", "ctrl+v":
			return nil, true
		}
		if isPasteKey(msg) {
			return nil, true
		}
		return nil, false
	}
	if isPasteKey(msg) {
		m.insertText(string(msg.Runes))
		return nil, true
	}
	switch msg.String() {
	case "ctrl+c":
		if m.hasSel() {
			return m.editCopy(), true
		}
		return nil, false
	case "ctrl+x":
		return m.editCut(), true
	case "ctrl+v":
		m.editPaste("")
		return nil, true
	case "ctrl+a":
		m.selectAll()
		return nil, true
	}
	return nil, false
}

func isSpaceKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeySpace {
		return true
	}
	switch msg.String() {
	case " ", "space":
		return true
	}
	return msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == ' '
}

func isNewlineKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "ctrl+j", "alt+enter", "shift+enter":
		return true
	}
	return msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == '\n'
}

func isPasteKey(msg tea.KeyMsg) bool {
	if msg.Paste && len(msg.Runes) > 0 {
		return true
	}
	return msg.Type == tea.KeyRunes && len(msg.Runes) > 1
}

func (m model) listKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.approval != nil {
			m.decide(2)
		}
		m.closeFloat()
		m.syncView()
		return m, nil
	case "up":
		m.moveList(-1)
		return m, nil
	case "down":
		m.moveList(1)
		return m, nil
	case "enter":
		if m.approval != nil {
			m.decide(m.choice)
			m.syncView()
			return m, nil
		}
		return m.finishLayout(m.activateFloatItem(m.palIdx))
	}
	return m, nil
}

func (m *model) moveList(delta int) {
	if m.approval != nil {
		m.choice = clamp(m.choice+delta, 0, 2)
		m.listOff = ensureVisible(m.choice, m.reg.listVis, m.listOff)
		return
	}
	n := len(m.floatItems())
	if n == 0 {
		return
	}
	m.palIdx = clamp(m.palIdx+delta, 0, n-1)
	if m.float == floatSessions {
		if m.palIdx > 0 {
			m.listOff = ensureVisible(m.palIdx-1, m.reg.listVis, m.listOff)
		}
		return
	}
	m.listOff = ensureVisible(m.palIdx, m.reg.listVis, m.listOff)
}

func (m model) settingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.tab != settingsTabProviders {
		return m, nil
	}
	n := 0
	if m.cfg != nil {
		n = len(m.cfg.Providers)
	}
	switch msg.String() {
	case "up":
		if n > 0 {
			m.provIdx = clamp(m.provIdx-1, 0, n-1)
			m.provOff = ensureVisible(m.provIdx, m.reg.provVis, m.provOff)
		}
	case "down":
		if n > 0 {
			m.provIdx = clamp(m.provIdx+1, 0, n-1)
			m.provOff = ensureVisible(m.provIdx, m.reg.provVis, m.provOff)
		}
	case "enter":
		if n > 0 {
			return m.activate(Target{Kind: KindProviderRow, Index: m.provIdx})
		}
	}
	return m, nil
}

func (m model) formKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		m.formMoveField(-1)
	case "down":
		m.formMoveField(1)
	case "left":
		m.moveCaret(-1, false)
	case "right":
		m.moveCaret(1, false)
	case "shift+left":
		m.moveCaret(-1, true)
	case "shift+right":
		m.moveCaret(1, true)
	case "backspace":
		m.editBackspace()
	default:
		if isSpaceKey(msg) {
			m.insertText(" ")
		} else if isPasteKey(msg) || (len(msg.Runes) == 1 && msg.Type == tea.KeyRunes) {
			m.insertText(string(msg.Runes))
		}
	}
	m.layout()
	return m, nil
}

func (m *model) formMoveField(delta int) {
	switch {
	case m.form.field < 3:
		next := m.form.field + delta
		if next < 0 {
			return
		}
		if next > 2 {
			m.form.field = 3
			if n := len(m.form.models); n > 0 {
				m.form.modelIdx = clamp(m.form.modelIdx, 0, n-1)
				m.formOff = ensureVisible(m.form.modelIdx, m.reg.formVis, m.formOff)
			}
		} else {
			m.form.field = next
		}
	case delta < 0:
		if m.form.modelIdx > 0 && len(m.form.models) > 0 {
			m.form.modelIdx--
			m.formOff = ensureVisible(m.form.modelIdx, m.reg.formVis, m.formOff)
		} else {
			m.form.field = 2
		}
	default:
		if n := len(m.form.models); n > 0 {
			m.form.modelIdx = clamp(m.form.modelIdx+1, 0, n-1)
			m.formOff = ensureVisible(m.form.modelIdx, m.reg.formVis, m.formOff)
		}
	}
	m.selCaret = utf8.RuneCountInString(m.formValue())
	m.collapseSel()
}

func (m *model) decide(i int) {
	if m.approval == nil {
		return
	}
	labels := []string{"allow once", "allow session", "deny"}
	d := harness.Deny
	switch i {
	case 0:
		d = harness.AllowOnce
	case 1:
		d = harness.AllowSession
	}
	choice := clamp(i, 0, 2)
	q := "approve " + m.approval.Tool + " (" + m.approval.Risk + "): " + m.approval.Summary
	a := "→ " + labels[choice]
	select {
	case m.approval.Reply <- d:
	default:
	}
	m.approval = nil
	m.closeFloat()
	m.status = "running"
	m.followBottom = true
	m.lines = append(m.lines, line{kind: "sys", text: q}, line{kind: "sys", text: a})
}
