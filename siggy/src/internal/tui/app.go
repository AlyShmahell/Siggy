package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"siggy/src/internal/config"
	"siggy/src/internal/graph"
	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/loop"
)

type page int

const (
	pageSession page = iota
	pageSettings
	pageProviderForm
)

type settingsTab int

const (
	settingsTabProviders settingsTab = iota
	settingsTabVersion
)

type line struct {
	kind  string
	text  string
	tool  string
	model string
	toks  int
	rate  float64
}

type providerForm struct {
	original  string
	name      string
	url       string
	apiKey    string
	models    []string
	protocols []string
	field     int
	modelIdx  int
	protoIdx  int
	err       string
}

type model struct {
	width, height int
	cfg           *config.Config
	g             *graph.Graph
	h             *harness.Harness
	home          string

	hits *HitMap
	scr  *frame
	reg  *regions
	page page

	vp viewport.Model
	ta textarea.Model

	lines         []line
	sessions      []string
	sessionTitles map[string]string
	sessIdx       int

	hovered Target
	pressed *Target

	running   bool
	cancel    context.CancelFunc
	evCh      chan loop.Event
	approval  *harness.ApprovalRequest
	choice    int
	float     floatKind
	palIdx    int
	status    string
	err       string
	streaming string

	provIdx      int
	form         providerForm
	tab          settingsTab
	tokensUsed   int
	billedTokens int
	billedEst    bool
	mentions     []string
	modelHealth  string
	listOff      int
	provOff      int
	formOff      int
	followBottom bool

	selAnchor     int
	selCaret      int
	editClip      string
	lastClickAt   time.Time
	lastClick     Target
	dragging      bool
	dragMoved     bool
	cursorOn      bool
	cursorTicking bool
	lastEdit      time.Time
	promptOff     int

	focusTrans  bool
	transPlain  []string
	transAnchor int
	transCaret  int
	streamStart time.Time
	compToks    int

	fillW, fillH, fillSide int
	fillBg, fillNav        string
	fillPanel, fillMain    string

	laidW, laidH int
	laidPage     page
	laidSettings bool

	wsRoot   string
	wsBrowse string
	wsDirs   []string

	imgSlots []imgSlot
	imgCache map[string]imgCacheEntry
}

type evMsg loop.Event
type doneWait struct{}
type healthMsg struct{ status string }

type floatKind int

const (
	floatNone floatKind = iota
	floatMode
	floatModel
	floatSessions
	floatMentions
	floatRestore
	floatUsage
	floatWorkspace
)

var modeItems = []string{string(harness.ModeChat), string(harness.ModePlan), string(harness.ModeAct)}

type modelPair struct {
	provider string
	model    string
	label    string
}

var protocolOptions = []struct {
	id      string
	label   string
	enabled bool
}{
	{config.ProtocolOpenAI, "openai", true},
	{"anthropic", "anthropic (soon)", false},
	{"gemini", "gemini (soon)", false},
}

func New(g *graph.Graph, h *harness.Harness, cfg *config.Config) model {
	ta := textarea.New()
	ta.Placeholder = "message siggy"
	ta.Focus()
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.FocusedStyle.CursorLine = ta.BlurredStyle.CursorLine
	vp := viewport.New(80, 20)
	home := h.Home
	if cfg != nil {
		home = cfg.Home
	}
	m := model{
		vp:          vp,
		ta:          ta,
		g:           g,
		h:           h,
		cfg:         cfg,
		home:        home,
		hits:        &HitMap{},
		scr:         &frame{},
		reg:         &regions{},
		status:      "ready",
		modelHealth: "…",
		cursorOn:    true,
		imgCache:    map[string]imgCacheEntry{},
	}
	m.scr.hits = m.hits
	if h != nil && h.Workspace != nil {
		m.wsRoot = h.Workspace.Root
	}
	if cfg == nil {
		m.cfg = &config.Config{Home: home, ActiveProvider: "env", Model: "gpt-4.1", ContextWindow: 128000, Providers: []config.Provider{{
			Name: "env", URL: "https://api.openai.com/v1", Models: []string{"gpt-4.1"}, Protocols: []string{config.ProtocolOpenAI},
		}}}
	}
	if m.g != nil && m.g.Engine != nil && m.cfg != nil && m.cfg.ContextWindow > 0 {
		m.g.Engine.ContextWindow = m.cfg.ContextWindow
	}
	m.loadTranscript("")
	m.reloadSessions()
	m.width, m.height = 80, 24
	m.layout()
	m.refresh()
	_ = m.paint()
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(enableEnhancedKeys(), m.probeHealth())
}

func Run(g *graph.Graph, h *harness.Harness, cfg *config.Config) error {
	in, restore := programInput()
	defer restore()
	defer disableEnhancedKeys()
	p := tea.NewProgram(New(g, h, cfg), tea.WithAltScreen(), tea.WithMouseAllMotion(), tea.WithInput(in))
	_, err := p.Run()
	return err
}

func waitEvent(ch <-chan loop.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return doneWait{}
		}
		return evMsg(ev)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.syncView()
		return m, nil
	case evMsg:
		return m.onEvent(loop.Event(msg))
	case doneWait:
		m.running = false
		m.status = "ready"
		m.layout()
		return m, nil
	case tea.MouseMsg:
		return m.onMouse(msg)
	case tea.KeyMsg:
		next, cmd := m.onKey(msg)
		nm := next.(model)
		return nm, tea.Batch(cmd, nm.startBlink())
	case healthMsg:
		if msg.status != "" {
			m.modelHealth = msg.status
		}
		return m, nil
	case titleMsg:
		if m.h != nil && m.h.Session != nil && m.h.Session.ID == msg.id {
			m.h.Session.Meta.Title = msg.title
		}
		m.reloadSessions()
		return m, nil
	case cursorTickMsg:
		if !m.shouldBlink() {
			m.cursorOn = true
			m.cursorTicking = false
			return m, nil
		}
		m.cursorOn = !m.cursorOn
		return m, tickCursor()
	}
	var cmd tea.Cmd
	if m.page == pageSession && !m.modal() {
		m.ta, cmd = m.ta.Update(msg)
	}
	return m, cmd
}

func (m model) modal() bool {
	return m.approval != nil || m.float != floatNone
}

func (m *model) openFloat(k floatKind) {
	m.float = k
	m.palIdx = 0
	m.listOff = 0
	if k == floatSessions && len(m.sessions) > 0 {
		m.palIdx = clamp(m.sessIdx, 0, len(m.sessions)-1) + 1
	}
	if k == floatWorkspace {
		m.wsBrowse = m.wsRoot
		if m.h != nil && m.h.Workspace != nil {
			m.wsBrowse = m.h.Workspace.Root
		}
		m.loadWorkspaceDirs()
	}
}

func (m *model) closeFloat() {
	m.float = floatNone
}

func (m *model) floatItems() []string {
	switch m.float {
	case floatMode:
		return append([]string{}, modeItems...)
	case floatModel:
		var out []string
		for _, p := range m.modelPairs() {
			out = append(out, p.label)
		}
		return out
	case floatSessions:
		out := []string{"delete all"}
		return append(out, m.sessions...)
	case floatRestore:
		var out []string
		for _, rec := range m.checkpoints() {
			out = append(out, fmt.Sprintf("%d  %s", rec.Seq, rec.Path))
		}
		return out
	case floatMentions:
		return append([]string{}, m.mentions...)
	case floatWorkspace:
		var out []string
		for _, r := range m.workspaceRows() {
			out = append(out, r.label)
		}
		return out
	default:
		return nil
	}
}

func (m model) onSettings() bool {
	return m.page == pageSettings || m.page == pageProviderForm
}

func (m *model) modelPairs() []modelPair {
	var out []modelPair
	if m.cfg == nil {
		return out
	}
	for _, p := range m.cfg.Providers {
		if len(p.Models) == 0 {
			out = append(out, modelPair{provider: p.Name, label: p.Name})
			continue
		}
		for _, mod := range p.Models {
			out = append(out, modelPair{provider: p.Name, model: mod, label: p.Name + " / " + mod})
		}
	}
	return out
}

func (m *model) currentMode() string {
	mode := string(harness.ModeAct)
	if m.g != nil && m.g.Engine != nil && m.g.Engine.Harness != nil && m.g.Engine.Harness.Mode != "" {
		mode = string(m.g.Engine.Harness.Mode)
	}
	return mode
}

func (m *model) phaseLabel() string {
	if !m.running && m.approval == nil {
		return "ready"
	}
	node := ""
	if m.g != nil && m.g.Node != "" {
		node = string(m.g.Node)
	}
	return phaseFromNode(node)
}

func phaseFromNode(node string) string {
	switch graph.Node(node) {
	case graph.NodeThink, graph.NodeCompact:
		return "thinking"
	case graph.NodeSchedule, graph.NodeExecute, graph.NodeSpawn:
		return "tools"
	case graph.NodeApprove:
		return "approval"
	case graph.NodeDone:
		return "done"
	}
	if node == "" {
		return "thinking"
	}
	return strings.ToLower(node)
}

func (m *model) currentModelLabel() string {
	if m.cfg == nil {
		return "env / gpt-4.1"
	}
	return m.cfg.ActiveProvider + " / " + m.cfg.Model
}

func (m model) onEvent(ev loop.Event) (tea.Model, tea.Cmd) {
	switch ev.Kind {
	case loop.KindText:
		if m.streamStart.IsZero() {
			m.streamStart = time.Now()
		}
		m.streaming += ev.Text
		m.replaceStream()
	case loop.KindToolStart:
		m.flushStream()
		m.lines = append(m.lines, line{kind: "tool", tool: ev.Tool, text: ev.Args})
	case loop.KindToolEnd:
		text := ev.Text
		kind := "ok"
		if ev.Err != nil {
			kind = "err"
		} else if isDiffResult(text) {
			kind = "diff"
			if len(text) > 8192 {
				text = text[:8192] + "…"
			}
		} else if len(text) > 240 {
			text = text[:240] + "…"
		}
		m.lines = append(m.lines, line{kind: kind, tool: ev.Tool, text: text})
	case loop.KindApproval:
		m.approval = ev.Approval
		m.choice = 0
		m.status = "approval"
	case loop.KindError:
		m.flushStream()
		if ev.Err != nil {
			m.err = ev.Err.Error()
			m.lines = append(m.lines, line{kind: "err", text: ev.Err.Error()})
			if ev.Tool == "" {
				m.modelHealth = "err"
			}
		}
	case loop.KindDone:
		m.flushStream()
		m.running = false
		m.status = "ready"
		m.modelHealth = "ok"
		m.syncView()
		return m, m.maybeTitle()
	case loop.KindNode:
		m.status = phaseFromNode(ev.Node)
	case loop.KindMode:
		m.status = ev.Mode
	case loop.KindSystem:
		m.lines = append(m.lines, line{kind: "sys", text: ev.Text})
	case loop.KindUsage:
		if ev.PromptTokens > 0 {
			m.tokensUsed = ev.PromptTokens
		} else if ev.TotalTokens > 0 {
			m.tokensUsed = ev.TotalTokens
		}
		if ev.CompletionTokens > 0 {
			m.compToks = ev.CompletionTokens
		}
		if n := loop.Billed(ev.PromptTokens, ev.CompletionTokens, ev.TotalTokens); n > 0 {
			m.billedTokens += n
		}
		if ev.Estimated {
			m.billedEst = true
		}
	}
	m.syncView()
	if m.running {
		return m, waitEvent(m.evCh)
	}
	return m, nil
}

func (m *model) syncView() {
	m.layout()
	if m.page == pageSession {
		m.refresh()
	}
}

func (m *model) resetTranscript(text string) {
	m.loadTranscript(text)
}

func (m *model) leaveSessionPage() {
	m.err = ""
	m.closeFloat()
}

func (m *model) flushStream() {
	if m.streaming == "" {
		m.streamStart = time.Time{}
		m.compToks = 0
		return
	}
	m.replaceStream()
	m.stampReply()
	m.streaming = ""
	m.streamStart = time.Time{}
	m.compToks = 0
}

func (m *model) replaceStream() {
	if m.hasTransSel() {
		m.transCollapse()
	}
	if len(m.lines) > 0 && m.lines[len(m.lines)-1].kind == "asst-live" && m.lines[len(m.lines)-1].model == "" {
		m.lines[len(m.lines)-1].text = m.streaming
		return
	}
	m.lines = append(m.lines, line{kind: "asst-live", text: m.streaming})
}

func (m *model) stampReply() {
	if len(m.lines) == 0 || m.lines[len(m.lines)-1].kind != "asst-live" {
		return
	}
	text := m.lines[len(m.lines)-1].text
	if strings.TrimSpace(text) == "" {
		return
	}
	toks := m.compToks
	if toks <= 0 {
		toks = utf8.RuneCountInString(text) / 4
	}
	elapsed := time.Since(m.streamStart).Seconds()
	if m.streamStart.IsZero() {
		return
	}
	if elapsed < 0.001 {
		elapsed = 0.001
	}
	m.lines[len(m.lines)-1].model = m.currentModelLabel()
	m.lines[len(m.lines)-1].toks = toks
	m.lines[len(m.lines)-1].rate = float64(toks) / elapsed
}

func (m *model) reloadSessions() {
	hash := ""
	if m.h != nil && m.h.Workspace != nil {
		hash = harness.HashWorkspace(m.h.Workspace.Root)
	}
	metas, err := harness.ListSessionMetas(m.home, hash)
	if err != nil {
		m.sessions = nil
		m.sessionTitles = nil
		return
	}
	ids := make([]string, 0, len(metas))
	titles := make(map[string]string, len(metas))
	for i, meta := range metas {
		ids = append(ids, meta.ID)
		titles[meta.ID] = sessionTitle(meta.Title)
		if m.h != nil && m.h.Session != nil && meta.ID == m.h.Session.ID {
			m.sessIdx = i
		}
	}
	m.sessions = ids
	m.sessionTitles = titles
}

func (m *model) applyClient() {
	if m.cfg == nil || m.g == nil || m.g.Engine == nil {
		return
	}
	p := m.cfg.Active()
	m.g.Engine.LLM = llm.NewHTTP(p.URL, p.APIKey, m.cfg.Model)
	if m.cfg.ContextWindow > 0 {
		m.g.Engine.ContextWindow = m.cfg.ContextWindow
	}
}

func (m *model) afterClientChange() tea.Cmd {
	m.modelHealth = "…"
	return m.probeHealth()
}

type pinger interface {
	Ping(context.Context) string
}

func (m model) probeHealth() tea.Cmd {
	var client llm.Client
	if m.g != nil && m.g.Engine != nil {
		client = m.g.Engine.LLM
	}
	return func() tea.Msg {
		if client == nil {
			return healthMsg{status: "…"}
		}
		p, ok := client.(pinger)
		if !ok {
			return healthMsg{status: "…"}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return healthMsg{status: p.Ping(ctx)}
	}
}

func workspaceName(h *harness.Harness) string {
	if h == nil || h.Workspace == nil {
		return "."
	}
	return filepath.Base(h.Workspace.Root)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func visibleStart(n, vis, off int) int {
	if vis < 1 {
		return 0
	}
	return clamp(off, 0, max(n-vis, 0))
}

func scrollOff(off, n, vis, dir int) int {
	return visibleStart(n, vis, off+dir)
}

func ensureVisible(sel, vis, off int) int {
	if vis < 1 {
		return off
	}
	if sel < off {
		return sel
	}
	if sel >= off+vis {
		return sel - vis + 1
	}
	return off
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func containsStr(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func (m model) start(user string) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.status = "thinking"
	m.evCh = make(chan loop.Event, 32)
	go func() {
		_ = m.g.Run(ctx, user, func(ev loop.Event) {
			select {
			case m.evCh <- ev:
			case <-ctx.Done():
			}
		})
		close(m.evCh)
	}()
	m.syncView()
	return m, waitEvent(m.evCh)
}

func (m model) submit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.ta.Value())
	if text == "" || m.running {
		return m, nil
	}
	m.ta.Reset()
	m.selAnchor, m.selCaret = 0, 0
	m.closeFloat()
	if strings.HasPrefix(text, "/") {
		return m.command(text)
	}
	m.lines = append(m.lines, line{kind: "user", text: text})
	m.refresh()
	return m.start(expandMentions(m.h, text))
}

func (m model) command(text string) (tea.Model, tea.Cmd) {
	cmd, arg, _ := strings.Cut(text, " ")
	arg = strings.TrimSpace(arg)
	switch cmd {
	case "/help":
		m.lines = append(m.lines, line{kind: "sys", text: " /help /new /resume /plan /act /allow /providers /connect /compress /compress-fast /export /restore /branch /rewind /remember /forget /memory /dream /quit"})
	case "/new":
		return m.activate(Target{Kind: KindSidebarNew})
	case "/resume":
		m.reloadSessions()
		m.openFloat(floatSessions)
	case "/plan":
		m.g.SetMode(harness.ModePlan)
		m.lines = append(m.lines, line{kind: "sys", text: "mode → plan"})
	case "/act":
		m.g.SetMode(harness.ModeAct)
		m.lines = append(m.lines, line{kind: "sys", text: "mode → act"})
	case "/allow":
		m.h.Approvals.SetAuto(true)
		m.lines = append(m.lines, line{kind: "sys", text: "auto-approve on"})
	case "/providers", "/connect":
		m.leaveSessionPage()
		m.page = pageSettings
		m.tab = settingsTabProviders
		m.provIdx = 0
		m.provOff = 0
	case "/compress":
		m.runCompact(false)
	case "/compress-fast":
		m.runCompact(true)
	case "/export":
		m.exportSession(arg)
	case "/restore":
		if len(m.checkpoints()) == 0 {
			m.lines = append(m.lines, line{kind: "sys", text: "no file checkpoints"})
		} else {
			m.openFloat(floatRestore)
		}
	case "/branch":
		m.branchSession()
	case "/rewind":
		m.rewindSession(arg)
	case "/remember":
		m.rememberFact(arg)
	case "/forget":
		m.forgetMemory(arg)
	case "/memory":
		m.showMemory()
	case "/dream":
		m.dreamMemory()
	case "/quit":
		return m, tea.Quit
	default:
		m.lines = append(m.lines, line{kind: "err", text: "unknown command " + cmd})
	}
	m.syncView()
	return m, nil
}
