package tui

const (
	sidebarWidth = 22
	statusRows   = 0
	composerRows = 6
	navRows      = 1
)

const (
	glyphPlus  = "+"
	glyphClock = "◷"
	glyphGear  = "⚙"
	glyphQuit  = "✕"
	glyphStop  = "⏹"
	glyphBack  = "←"
	glyphEdit  = "✎"
)

type regions struct {
	nav          Rect
	navPlus      Rect
	navClock     Rect
	navGear      Rect
	navQuit      Rect
	navWorkspace Rect
	navTitle     Rect
	sidebar      Rect
	transcript   Rect
	composer     Rect
	prompt       Rect
	send         Rect
	cancel       Rect
	usage        Rect
	slash        Rect
	status       Rect
	statusMode   Rect
	statusProv   Rect
	modal        Rect
	listVis      int
	provVis      int
	formVis      int
}

func (m *model) layout() {
	if m.reg == nil {
		m.reg = &regions{}
	}
	w, h := m.width, m.height
	if w < 20 {
		w = 80
	}
	if h < 10 {
		h = 24
	}
	side := m.onSettings()
	if m.laidW == w && m.laidH == h && m.width == w && m.height == h && m.laidPage == m.page && m.laidSettings == side {
		return
	}
	m.width, m.height = w, h
	m.laidW, m.laidH = w, h
	m.laidPage = m.page
	m.laidSettings = side

	navH := navRows
	bodyY := navH
	bodyH := h - statusRows - navH
	if bodyH < 1 {
		bodyH = 1
	}

	sideW := 0
	if m.onSettings() {
		sideW = sidebarWidth
		if w < 80 {
			sideW = min(sidebarWidth, max(12, w/4))
		}
	}
	mainW := w - sideW
	compH := 0
	if m.page == pageSession {
		compH = composerRows
	}

	m.reg.nav = Rect{0, 0, w, navH}
	m.reg.navPlus = Rect{}
	m.reg.navClock = Rect{}
	m.reg.navGear = Rect{}
	m.reg.navQuit = Rect{}
	m.reg.sidebar = Rect{0, bodyY, sideW, bodyH}
	m.reg.transcript = Rect{sideW, bodyY, mainW, bodyH - compH}
	m.reg.composer = Rect{sideW, bodyY + bodyH - compH, mainW, compH}
	m.reg.status = Rect{}
	m.reg.prompt = Rect{}
	m.reg.send = Rect{}
	m.reg.cancel = Rect{}
	m.reg.usage = Rect{}
	m.reg.slash = Rect{}
	m.reg.modal = Rect{}
	m.reg.statusMode = Rect{}
	m.reg.statusProv = Rect{}

	if m.page == pageSession {
		m.vp.Width = max(mainW, 8)
		m.vp.Height = max(m.reg.transcript.H, 3)
		m.ta.SetWidth(max(mainW-4, 8))
		m.ta.SetHeight(3)
	}
}
