package tui

import (
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"siggy/src/internal/config"
	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
)

func (m model) onMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	t, ok := m.hits.At(msg.X, msg.Y)

	if m.dragging && m.pressed != nil && m.pressed.Kind == KindTranscript && m.approval == nil {
		if msg.Action == tea.MouseActionMotion {
			off := m.transcriptClickOffset(msg.X, msg.Y)
			if off != m.transCaret {
				m.dragMoved = true
			}
			m.transCaret = off
			return m, nil
		}
	}

	if m.dragging && m.pressed != nil && isTextInput(*m.pressed) && m.approval == nil {
		if msg.Action == tea.MouseActionMotion {
			off := m.clickOffset(*m.pressed, msg.X, msg.Y)
			if off != m.selCaret {
				m.dragMoved = true
			}
			m.placeCaret(off, true)
			return m, nil
		}
	}

	if msg.Action == tea.MouseActionMotion || msg.Button == tea.MouseButtonNone {
		if ok {
			m.hovered = t
			if !m.dragging {
				m.hoverSelect(t)
			}
		} else {
			m.hovered = Target{}
		}
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		return m.onWheel(msg, t, ok)
	case tea.MouseButtonRight:
		return m, nil
	}

	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	switch msg.Action {
	case tea.MouseActionPress:
		if ok && t.Kind == KindTranscript && m.approval == nil {
			m.focusTranscript()
			now := time.Now()
			dbl := !m.dragMoved && sameTarget(m.lastClick, t) && !m.lastClickAt.IsZero() && now.Sub(m.lastClickAt) < dblClickMax
			if dbl {
				m.transSelectAll()
			} else {
				off := m.transcriptClickOffset(msg.X, msg.Y)
				m.transAnchor = off
				m.transCaret = off
			}
			m.lastClickAt = now
			m.lastClick = t
			cp := t
			m.pressed = &cp
			m.dragging = true
			m.dragMoved = false
			return m, nil
		}
		if ok && isTextInput(t) && m.approval == nil {
			m.blurTranscript()
			m.focusTextInput(t)
			now := time.Now()
			dbl := !m.dragMoved && sameTarget(m.lastClick, t) && !m.lastClickAt.IsZero() && now.Sub(m.lastClickAt) < dblClickMax
			if dbl {
				m.selectAll()
			} else {
				m.placeCaret(m.clickOffset(t, msg.X, msg.Y), msg.Shift)
			}
			m.lastClickAt = now
			m.lastClick = t
			cp := t
			m.pressed = &cp
			m.dragging = true
			m.dragMoved = false
			return m, nil
		}
		if ok {
			cp := t
			m.pressed = &cp
			m.hoverSelect(t)
		}
	case tea.MouseActionRelease:
		wasDrag := m.dragging
		textPress := m.pressed != nil && (isTextInput(*m.pressed) || m.pressed.Kind == KindTranscript)
		m.dragging = false
		if wasDrag && textPress {
			m.pressed = nil
			return m, nil
		}
		if m.pressed != nil && ok && sameTarget(*m.pressed, t) {
			m.pressed = nil
			return m.activate(t)
		}
		m.pressed = nil
	}
	return m, nil
}

func sameTarget(a, b Target) bool {
	return a.Kind == b.Kind && a.Index == b.Index
}

func (m *model) hoverSelect(t Target) {
	switch t.Kind {
	case KindApprove:
		m.choice = t.Index
	case KindModalItem, KindMention:
		m.palIdx = t.Index
	case KindSidebarSession:
		m.sessIdx = t.Index
		if m.float == floatSessions {
			m.palIdx = t.Index + 1
		}
	case KindSidebarDeleteAll:
		if m.float == floatSessions {
			m.palIdx = 0
		}
	case KindWorkspaceUse:
		m.palIdx = 0
	case KindWorkspaceUp:
		if m.wsCanUp() {
			m.palIdx = 1
		}
	case KindWorkspaceDir:
		off := 1
		if m.wsCanUp() {
			off = 2
		}
		m.palIdx = off + t.Index
	case KindProviderRow:
		m.provIdx = t.Index
	case KindFormField:
		if t.Index >= 10 && t.Index < 10+len(protocolOptions) {
			m.form.protoIdx = t.Index - 10
		}
	case KindFormListItem:
	}
}

func (m model) onWheel(msg tea.MouseMsg, t Target, ok bool) (tea.Model, tea.Cmd) {
	dir := 1
	if msg.Button == tea.MouseButtonWheelUp {
		dir = -1
	}
	if m.modal() && ok && (t.Kind == KindModalItem || t.Kind == KindMention || t.Kind == KindApprove || t.Kind == KindNone || t.Kind == KindSidebarSession || t.Kind == KindSidebarDelete || t.Kind == KindSidebarDeleteAll || t.Kind == KindWorkspaceUse || t.Kind == KindWorkspaceUp || t.Kind == KindWorkspaceDir) {
		n := len(m.floatItems())
		if m.float == floatSessions {
			n = len(m.sessions)
		}
		if m.approval != nil {
			n = 3
		}
		m.listOff = scrollOff(m.listOff, n, m.reg.listVis, dir)
		return m, nil
	}
	if m.page == pageSettings && m.tab == settingsTabProviders && msg.X >= m.reg.sidebar.W {
		n := 0
		if m.cfg != nil {
			n = len(m.cfg.Providers)
		}
		m.provOff = scrollOff(m.provOff, n, m.reg.provVis, dir)
		return m, nil
	}
	if m.page == pageProviderForm && msg.X >= m.reg.sidebar.W {
		m.formOff = scrollOff(m.formOff, len(m.form.models), m.reg.formVis, dir)
		return m, nil
	}
	if ok && t.Kind == KindPrompt {
		key := tea.KeyDown
		if dir < 0 {
			key = tea.KeyUp
		}
		m.ta, _ = m.ta.Update(tea.KeyMsg{Type: key})
		return m, nil
	}
	if !ok || t.Kind == KindTranscript {
		if dir < 0 {
			m.vp.ScrollUp(1)
		} else {
			m.vp.ScrollDown(1)
		}
	}
	return m, nil
}

func (m model) activate(t Target) (tea.Model, tea.Cmd) {
	switch t.Kind {
	case KindSidebarNew:
		m.closeFloat()
		m.startFreshSession()
	case KindNavClock:
		if m.float == floatSessions {
			m.closeFloat()
		} else {
			m.reloadSessions()
			m.openFloat(floatSessions)
		}
	case KindNavGear:
		m.leaveSessionPage()
		m.page = pageSettings
		m.tab = settingsTabProviders
		m.provOff = 0
	case KindNavQuit:
		return m, tea.Quit
	case KindNavWorkspace:
		if m.float == floatWorkspace {
			m.closeFloat()
		} else {
			m.openFloat(floatWorkspace)
		}
	case KindNavTitle:
		m.closeFloat()
		m.page = pageSession
	case KindWorkspaceUse:
		m.applyWorkspace()
	case KindWorkspaceUp:
		m.workspaceUp()
	case KindWorkspaceDir:
		m.enterWorkspaceDir(t.Index)
	case KindSidebarProviders:
		m.form.err = ""
		m.page = pageSettings
		m.tab = settingsTabProviders
	case KindSidebarVersion:
		m.form.err = ""
		m.page = pageSettings
		m.tab = settingsTabVersion
	case KindSidebarSession:
		if t.Index >= 0 && t.Index < len(m.sessions) {
			m.openSession(m.sessions[t.Index])
		}
		m.closeFloat()
	case KindSidebarDelete:
		m.deleteSessionAt(t.Index)
		m.closeFloat()
	case KindSidebarDeleteAll:
		m.deleteAllSessions()
		m.closeFloat()
	case KindPrompt:
		m.blurTranscript()
		m.ta.Focus()
	case KindCancel:
		if m.running && m.cancel != nil {
			m.cancel()
			m.status = "cancelled"
			m.running = false
		}
	case KindMention:
		if t.Index >= 0 && t.Index < len(m.mentions) {
			m.insertMention(m.mentions[t.Index])
		}
	case KindComposerMode:
		m.openFloat(floatMode)
	case KindComposerModel:
		m.openFloat(floatModel)
	case KindUsage:
		if m.float == floatUsage {
			m.closeFloat()
		} else {
			m.openFloat(floatUsage)
		}
	case KindApprove:
		m.decide(t.Index)
	case KindModalItem:
		return m.finishLayout(m.activateFloatItem(t.Index))
	case KindModalDismiss:
		if m.approval != nil {
			m.decide(2)
		}
		m.closeFloat()
	case KindProviderRow:
		if t.Index >= 0 && t.Index < len(m.cfg.Providers) {
			_ = m.cfg.SetActive(m.cfg.Providers[t.Index].Name)
			_ = m.cfg.Save()
			m.applyClient()
			m.provIdx = t.Index
			m.syncView()
			return m, m.afterClientChange()
		}
	case KindProviderNew:
		m.openForm(config.Provider{Protocols: []string{config.ProtocolOpenAI}, Models: []string{""}})
	case KindProviderEdit:
		idx := t.Index
		if idx < 0 {
			idx = m.provIdx
		}
		if idx >= 0 && idx < len(m.cfg.Providers) {
			m.openForm(m.cfg.Providers[idx])
		}
	case KindProviderDelete:
		m.deleteProvider(t.Index)
	case KindFormBack, KindFormCancel:
		m.form.err = ""
		if m.page == pageProviderForm {
			m.page = pageSettings
		} else {
			m.page = pageSession
		}
	case KindFormField:
		m.form.field = t.Index
		if t.Index >= 10 && t.Index < 10+len(protocolOptions) {
			opt := protocolOptions[t.Index-10]
			if opt.enabled {
				if containsStr(m.form.protocols, opt.id) && opt.id != config.ProtocolOpenAI {
					m.form.protocols = removeStr(m.form.protocols, opt.id)
				} else if !containsStr(m.form.protocols, opt.id) {
					m.form.protocols = append(m.form.protocols, opt.id)
				}
			}
			m.form.protoIdx = t.Index - 10
		}
	case KindFormListItem:
		m.form.field = 3
		m.form.modelIdx = t.Index
	case KindFormAdd:
		m.form.models = append(m.form.models, "")
		m.form.field = 3
		m.form.modelIdx = len(m.form.models) - 1
	case KindFormDeleteModel:
		m.deleteFormModel(t.Index)
	case KindFormSave:
		if err := m.saveForm(); err != nil {
			m.form.err = err.Error()
		} else {
			m.form.err = ""
			m.page = pageSettings
			m.syncView()
			return m, m.afterClientChange()
		}
	}
	m.syncView()
	return m, nil
}

func (m model) activateFloatItem(i int) (tea.Model, tea.Cmd) {
	switch m.float {
	case floatMode:
		if i >= 0 && i < len(modeItems) {
			m.g.SetMode(harness.ParseMode(modeItems[i]))
			m.lines = append(m.lines, line{kind: "sys", text: "mode → " + modeItems[i]})
		}
		m.closeFloat()
	case floatModel:
		pairs := m.modelPairs()
		if i >= 0 && i < len(pairs) && m.cfg != nil {
			p := pairs[i]
			_ = m.cfg.SetActive(p.provider)
			if p.model != "" {
				m.cfg.Model = p.model
			}
			_ = m.cfg.Save()
			m.applyClient()
			m.closeFloat()
			return m, m.afterClientChange()
		}
		m.closeFloat()
	case floatSessions:
		if i == 0 {
			m.deleteAllSessions()
		} else if i-1 >= 0 && i-1 < len(m.sessions) {
			m.openSession(m.sessions[i-1])
		}
		m.closeFloat()
	case floatRestore:
		m.restoreCheckpointAt(i)
		m.closeFloat()
	case floatMentions:
		if i >= 0 && i < len(m.mentions) {
			m.insertMention(m.mentions[i])
		}
		m.closeFloat()
	case floatWorkspace:
		rows := m.workspaceRows()
		if i >= 0 && i < len(rows) {
			return m.activate(Target{Kind: rows[i].kind, Index: rows[i].index})
		}
	default:
		m.closeFloat()
	}
	return m, nil
}

func (m model) finishLayout(next tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	nm, ok := next.(model)
	if !ok {
		return next, cmd
	}
	nm.syncView()
	return nm, cmd
}

func (m *model) openSession(id string) {
	sess, err := harness.OpenSession(m.home, id)
	if err != nil {
		m.err = err.Error()
		return
	}
	if m.h.Session != nil {
		_ = m.h.Session.Close()
	}
	m.h.Session = sess
	m.g.Resume(sess.Records())
	if m.cfg != nil && m.g.Engine != nil {
		m.g.Engine.ContextWindow = m.cfg.ContextWindow
	}
	m.resetTranscript("resumed " + id)
	m.reloadSessions()
	m.page = pageSession
	m.syncView()
}

func (m *model) startFreshSession() {
	cwd, hash := "", ""
	if m.h != nil && m.h.Workspace != nil {
		cwd = m.h.Workspace.Root
		hash = harness.HashWorkspace(cwd)
	}
	sess, err := harness.NewSessionMeta(m.home, harness.SessionMeta{
		CWD:           cwd,
		WorkspaceHash: hash,
	})
	if err != nil {
		m.err = err.Error()
		return
	}
	if m.h != nil && m.h.Session != nil {
		_ = m.h.Session.Close()
	}
	m.h.Session = sess
	if m.g != nil && m.g.Engine != nil {
		sys := ""
		if len(m.g.Engine.Messages) > 0 && m.g.Engine.Messages[0].Role == llm.RoleSystem {
			sys = m.g.Engine.Messages[0].Content
		}
		if sys != "" {
			_ = sess.Append(harness.Record{Type: "system", Text: sys})
			m.g.Engine.Messages = []llm.Message{{Role: llm.RoleSystem, Content: sys}}
		} else {
			m.g.Engine.Messages = nil
		}
	}
	m.resetTranscript("new session " + sess.ID)
	m.reloadSessions()
	m.page = pageSession
	m.syncView()
}

func (m *model) deleteSessionAt(i int) {
	if i < 0 || i >= len(m.sessions) {
		return
	}
	id := m.sessions[i]
	active := m.h != nil && m.h.Session != nil && m.h.Session.ID == id
	if active {
		_ = m.h.Session.Close()
		m.h.Session = nil
	}
	if err := harness.DeleteSession(m.home, id); err != nil {
		m.err = err.Error()
		return
	}
	m.reloadSessions()
	if !active {
		return
	}
	if len(m.sessions) > 0 {
		m.openSession(m.sessions[0])
		return
	}
	m.startFreshSession()
}

func (m *model) deleteAllSessions() {
	if m.h != nil && m.h.Session != nil {
		_ = m.h.Session.Close()
		m.h.Session = nil
	}
	if err := harness.DeleteAllSessions(m.home); err != nil {
		m.err = err.Error()
		return
	}
	m.startFreshSession()
}

func (m *model) deleteFormModel(i int) {
	if i < 0 || i >= len(m.form.models) {
		return
	}
	m.form.models = append(m.form.models[:i], m.form.models[i+1:]...)
	if len(m.form.models) == 0 {
		m.form.models = []string{""}
	}
	m.form.field = 3
	m.form.modelIdx = clamp(m.form.modelIdx, 0, len(m.form.models)-1)
}

func (m *model) deleteProvider(i int) {
	if m.cfg == nil || i < 0 || i >= len(m.cfg.Providers) {
		return
	}
	name := m.cfg.Providers[i].Name
	m.cfg.Providers = append(m.cfg.Providers[:i], m.cfg.Providers[i+1:]...)
	if m.cfg.ActiveProvider == name {
		if len(m.cfg.Providers) > 0 {
			_ = m.cfg.SetActive(m.cfg.Providers[0].Name)
		} else {
			m.cfg.ActiveProvider = ""
		}
	}
	if len(m.cfg.Providers) == 0 {
		m.provIdx = 0
	} else {
		m.provIdx = clamp(m.provIdx, 0, len(m.cfg.Providers)-1)
	}
	_ = m.cfg.Save()
	m.applyClient()
}

func (m *model) openForm(p config.Provider) {
	models := append([]string{}, p.Models...)
	if len(models) == 0 {
		models = []string{""}
	}
	protos := append([]string{}, p.Protocols...)
	if !containsStr(protos, config.ProtocolOpenAI) {
		protos = append(protos, config.ProtocolOpenAI)
	}
	m.form = providerForm{
		original:  p.Name,
		name:      p.Name,
		url:       p.URL,
		apiKey:    p.APIKey,
		models:    models,
		protocols: protos,
		field:     0,
	}
	m.selCaret = utf8.RuneCountInString(p.Name)
	m.selAnchor = m.selCaret
	m.formOff = 0
	m.page = pageProviderForm
}

func (m *model) saveForm() error {
	p := config.Provider{
		Name:      strings.TrimSpace(m.form.name),
		URL:       strings.TrimSpace(m.form.url),
		APIKey:    m.form.apiKey,
		Models:    m.form.models,
		Protocols: m.form.protocols,
	}
	if err := config.ValidateProvider(p); err != nil {
		return err
	}
	if m.form.original != "" && m.form.original != p.Name {
		var next []config.Provider
		for _, x := range m.cfg.Providers {
			if x.Name != m.form.original {
				next = append(next, x)
			}
		}
		m.cfg.Providers = next
	}
	if err := m.cfg.Upsert(p); err != nil {
		return err
	}
	if err := m.cfg.SetActive(p.Name); err != nil {
		return err
	}
	m.applyClient()
	return m.cfg.Save()
}

func removeStr(xs []string, v string) []string {
	var out []string
	for _, x := range xs {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
