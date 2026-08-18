package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"siggy/src/internal/harness"
)

func (m model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

	if isNewlineKey(msg) {
		m.insertNewline()
		return m, nil
	}
	switch msg.String() {
	case "enter":
		if msg.Alt {
			m.insertNewline()
			return m, nil
		}
		return m.submit()
	}

	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	m.syncMentions()
	return m, cmd
}

func isNewlineKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "ctrl+j", "alt+enter", "shift+enter":
		return true
	}
	return msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == '\n'
}

func (m *model) insertNewline() {
	m.ta, _ = m.ta.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.syncMentions()
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
		if m.form.field == 3 && len(m.form.models) > 0 {
			m.form.modelIdx = clamp(m.form.modelIdx-1, 0, len(m.form.models)-1)
			m.formOff = ensureVisible(m.form.modelIdx, m.reg.formVis, m.formOff)
		}
	case "down":
		if m.form.field == 3 && len(m.form.models) > 0 {
			m.form.modelIdx = clamp(m.form.modelIdx+1, 0, len(m.form.models)-1)
			m.formOff = ensureVisible(m.form.modelIdx, m.reg.formVis, m.formOff)
		}
	case "backspace":
		m.formBackspace()
	default:
		if len(msg.Runes) == 1 && msg.Type == tea.KeyRunes {
			m.formType(string(msg.Runes))
		}
	}
	m.layout()
	return m, nil
}

func (m *model) formType(s string) {
	switch {
	case m.form.field == 0:
		m.form.name += s
	case m.form.field == 1:
		m.form.url += s
	case m.form.field == 2:
		m.form.apiKey += s
	case m.form.field == 3 && m.form.modelIdx >= 0 && m.form.modelIdx < len(m.form.models):
		m.form.models[m.form.modelIdx] += s
	}
}

func (m *model) formBackspace() {
	cut := func(s string) string {
		r := []rune(s)
		if len(r) == 0 {
			return ""
		}
		return string(r[:len(r)-1])
	}
	switch {
	case m.form.field == 0:
		m.form.name = cut(m.form.name)
	case m.form.field == 1:
		m.form.url = cut(m.form.url)
	case m.form.field == 2:
		m.form.apiKey = cut(m.form.apiKey)
	case m.form.field == 3 && m.form.modelIdx >= 0 && m.form.modelIdx < len(m.form.models):
		m.form.models[m.form.modelIdx] = cut(m.form.models[m.form.modelIdx])
	}
}

func (m model) decide(i int) {
	if m.approval == nil {
		return
	}
	d := harness.Deny
	switch i {
	case 0:
		d = harness.AllowOnce
	case 1:
		d = harness.AllowSession
	}
	select {
	case m.approval.Reply <- d:
	default:
	}
	m.approval = nil
	m.status = "running"
}
