package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"siggy/src/internal/config"
	"siggy/src/internal/version"
)

func (m *model) refresh() {
	var b strings.Builder
	w := max(m.reg.transcript.W-2, 20)
	for _, ln := range m.lines {
		b.WriteString(m.renderLine(ln, w))
		b.WriteByte('\n')
	}
	content := b.String()
	atBottom := m.vp.AtBottom()
	m.vp.SetContent(content)
	m.syncTransPlain(content)
	if atBottom || m.followBottom {
		m.vp.GotoBottom()
	}
	m.followBottom = false
}

func (m model) renderLine(ln line, width int) string {
	inner := max(width-4, 12)
	switch ln.kind {
	case "user":
		body := renderRich(ln.text, inner, false)
		return lipgloss.PlaceHorizontal(width, lipgloss.Right, body)
	case "asst-live":
		body := renderRich(ln.text, inner, true)
		if ln.model != "" {
			rate := int(ln.rate + 0.5)
			foot := stMuted.Render(fmt.Sprintf("%s  ·  %d tok/s", ln.model, rate))
			return lipgloss.JoinVertical(lipgloss.Left, body, foot)
		}
		return body
	case "tool":
		head := truncate(formatToolCard(ln.tool, ln.text), inner)
		return stToolCard.Width(min(inner+2, width)).Render(head)
	case "diff":
		return renderDiff(ln.text, inner)
	case "ok":
		return stOk.Render("  ↳ " + truncate(ln.text, inner))
	case "err":
		return stErr.Render("  ↳ " + truncate(ln.text, inner))
	default:
		return stSys.Render(ln.text)
	}
}

func wrap(s string, width int) string {
	if width < 8 {
		return s
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		r := []rune(para)
		for len(r) > width {
			lines = append(lines, string(r[:width]))
			r = r[width:]
		}
		lines = append(lines, string(r))
	}
	return strings.Join(lines, "\n")
}

func (m model) View() string {
	if m.width == 0 {
		return "siggy"
	}
	m.layout()
	return m.paint()
}

func (m *model) ensureFrame() {
	if m.hits == nil {
		m.hits = &HitMap{}
	}
	if m.scr == nil {
		m.scr = &frame{}
	}
	m.scr.hits = m.hits
}

func (m *model) paint() string {
	m.ensureFrame()
	w, h := m.width, m.height
	m.scr.reset(w, h)

	bodyY := navRows
	bodyH := h - statusRows - navRows
	if bodyH < 1 {
		bodyH = 1
	}
	sideW := m.reg.sidebar.W
	m.ensureFills(w, h, sideW)
	for y := 0; y < h; y++ {
		m.scr.blit(0, y, m.fillBg)
	}
	m.scr.blit(0, 0, m.fillNav)
	m.paintNav()

	if sideW > 0 {
		for y := bodyY; y < bodyY+bodyH; y++ {
			m.scr.blit(0, y, m.fillPanel)
		}
		m.paintSettingsSidebar(0, bodyY, sideW, bodyH)
	}

	mx, mw := sideW, w-sideW
	for y := bodyY; y < bodyY+bodyH; y++ {
		m.scr.blit(mx, y, m.fillMain)
	}
	switch m.page {
	case pageSettings:
		if m.tab == settingsTabVersion {
			m.paintVersion(mx, bodyY, mw, bodyH)
		} else {
			m.paintProviders(mx, bodyY, mw, bodyH)
		}
	case pageProviderForm:
		m.paintForm(mx, bodyY, mw, bodyH)
	default:
		m.paintSession(mx, bodyY, mw, bodyH)
	}
	if m.modal() {
		r := m.reg.transcript
		if r.W <= 0 || r.H <= 0 {
			r = Rect{mx, bodyY, mw, max(bodyH-composerRows, 3)}
		}
		m.paintModal(r.X, r.Y, r.W, r.H)
	}
	return m.scr.String()
}

func (m *model) ensureFills(w, h, sideW int) {
	mw := max(w-sideW, 0)
	if m.fillW == w && m.fillH == h && m.fillSide == sideW && m.fillBg != "" {
		return
	}
	m.fillW, m.fillH, m.fillSide = w, h, sideW
	m.fillBg = lipgloss.NewStyle().Background(colBg).Render(strings.Repeat(" ", max(w, 1)))
	m.fillNav = lipgloss.NewStyle().Background(colPanel).Render(strings.Repeat(" ", max(w, 1)))
	m.fillPanel = lipgloss.NewStyle().Background(colPanel).Render(strings.Repeat(" ", max(sideW, 1)))
	m.fillMain = lipgloss.NewStyle().Background(colBg).Render(strings.Repeat(" ", max(mw, 1)))
}

func (m *model) paintUsageFloat() {
	used := m.usageUsed()
	limit := m.contextWindow()
	label := fmt.Sprintf(" %d/%d ", used, limit)
	col := usageColor(used, limit)
	st := lipgloss.NewStyle().Foreground(col).Background(colPanel)
	inner := label
	w := lipgloss.Width(inner)
	if w < 7 {
		inner = padPlain(inner, 7)
		w = 7
	}
	top := st.Render("┌" + strings.Repeat("─", w) + "┐")
	mid := st.Render("│") + st.Bold(true).Render(inner) + st.Render("│")
	bot := st.Render("└" + strings.Repeat("─", w) + "┘")
	boxW := lipgloss.Width(mid)
	boxH := 3
	ux := m.reg.usage.X + m.reg.usage.W - boxW
	if ux < 0 {
		ux = 0
	}
	uy := m.reg.usage.Y - boxH
	if uy < navRows {
		uy = navRows
	}
	if ux+boxW > m.width {
		ux = max(m.width-boxW, 0)
	}
	m.reg.modal = Rect{ux, uy, boxW, boxH}
	m.hits.Add(Target{Kind: KindModalDismiss, Rect: m.reg.transcript})
	m.hits.Add(Target{Kind: KindNone, Rect: m.reg.modal})
	m.hits.Add(Target{Kind: KindUsage, Rect: m.reg.usage})
	m.scr.blit(ux, uy, top)
	m.scr.blit(ux, uy+1, mid)
	m.scr.blit(ux, uy+2, bot)
}

func (m *model) paintNav() {
	y := 0
	cx := 1
	wsName := workspaceName(m.h)
	wsSt := stMuted.Background(colPanel)
	if m.hovered.Kind == KindNavWorkspace || m.float == floatWorkspace {
		wsSt = stSel
	}
	ws := wsSt.Render(wsName)
	ww := lipgloss.Width(ws)
	m.reg.navWorkspace = Rect{cx, y, ww, 1}
	m.hits.Add(Target{Kind: KindNavWorkspace, Rect: m.reg.navWorkspace})
	m.scr.blit(cx, y, ws)
	cx += ww + 2

	label := untitledSession
	if m.h != nil && m.h.Session != nil {
		label = sessionTitle(m.h.Session.Meta.Title)
	}
	trunc := truncate(label, 22)
	titleSt := stSideTitle
	if m.hovered.Kind == KindNavTitle {
		titleSt = stSel
	}
	title := titleSt.Render(trunc)
	tw := lipgloss.Width(title)
	m.reg.navTitle = Rect{cx, y, tw, 1}
	m.hits.Add(Target{Kind: KindNavTitle, Rect: m.reg.navTitle})
	m.scr.blit(cx, y, title)
	cx += tw

	plus := m.navBtnCell(glyphPlus, KindSidebarNew, false)
	clock := m.navBtnCell(glyphClock, KindNavClock, m.float == floatSessions)
	gear := m.navBtnCell(glyphGear, KindNavGear, m.onSettings())
	quit := stQuit.Render(glyphQuit)
	if m.hovered.Kind == KindNavQuit {
		quit = stSel.Render(glyphQuit)
	}
	pw, cw, gw, qw := lipgloss.Width(plus), lipgloss.Width(clock), lipgloss.Width(gear), lipgloss.Width(quit)
	qx := m.width - 1 - qw
	gx := qx - 1 - gw
	clkx := gx - 1 - cw
	px := clkx - 1 - pw
	m.placeNavBtn(px, y, plus, KindSidebarNew)
	m.placeNavBtn(clkx, y, clock, KindNavClock)
	m.placeNavBtn(gx, y, gear, KindNavGear)
	m.placeNavBtn(qx, y, quit, KindNavQuit)

	brand := stSideTitle.Render("siggy")
	bw := lipgloss.Width(brand)
	bx := (m.width - bw) / 2
	leftEnd := cx + 1
	rightStart := px
	if bx < leftEnd {
		bx = leftEnd
	}
	if bx+bw > rightStart-1 {
		bx = max(rightStart-1-bw, leftEnd)
	}
	if bx+bw <= rightStart {
		m.scr.blit(bx, y, brand)
	}
}

func (m *model) navBtnCell(glyph string, k Kind, open bool) string {
	st := stChip.Background(colPanel)
	if k == KindSidebarNew {
		st = stAdd.Background(colPanel)
	}
	cell := st.Render(glyph)
	if m.hovered.Kind == k || open {
		cell = stSel.Render(glyph)
	}
	return cell
}

func (m *model) placeNavBtn(x, y int, cell string, k Kind) {
	w := lipgloss.Width(cell)
	r := Rect{x, y, w, 1}
	switch k {
	case KindSidebarNew:
		m.reg.navPlus = r
	case KindNavClock:
		m.reg.navClock = r
	case KindNavGear:
		m.reg.navGear = r
	case KindNavQuit:
		m.reg.navQuit = r
	}
	m.hits.Add(Target{Kind: k, Rect: r})
	m.scr.blit(x, y, cell)
}

func (m *model) paintSettingsSidebar(x, y, w, h int) {
	cy := y
	fill := lipgloss.NewStyle().Background(colPanel)
	left := stSideTitle.Background(colPanel).Render(" settings")
	back := stMuted.Background(colPanel).Render(glyphBack)
	if m.hovered.Kind == KindFormBack {
		back = stSel.Render(glyphBack)
	}
	lw, bw := lipgloss.Width(left), lipgloss.Width(back)
	gap := max(w-lw-bw, 1)
	row := left + fill.Render(strings.Repeat(" ", gap)) + back
	if pad := w - lipgloss.Width(row); pad > 0 {
		row += fill.Render(strings.Repeat(" ", pad))
	}
	m.scr.blit(x, cy, row)
	m.hits.Add(Target{Kind: KindFormBack, Rect: Rect{x + w - bw, cy, bw, 1}})
	cy++
	cy += m.scr.blit(x, cy, sideRule(w))
	m.hits.Add(Target{Kind: KindSidebarProviders, Rect: Rect{x, cy, w, 1}})
	cy += m.scr.blit(x, cy, m.sideRow("Providers", KindSidebarProviders, w))
	cy += m.scr.blit(x, cy, sideRule(w))
	m.hits.Add(Target{Kind: KindSidebarVersion, Rect: Rect{x, cy, w, 1}})
	cy += m.scr.blit(x, cy, m.sideRow("Version", KindSidebarVersion, w))
	_ = h
}

func sideRule(w int) string {
	return stMuted.Background(colPanel).Render(strings.Repeat("─", max(w, 1)))
}

func (m model) sideRow(label string, k Kind, w int) string {
	active := (k == KindSidebarProviders && m.tab == settingsTabProviders) ||
		(k == KindSidebarVersion && m.tab == settingsTabVersion)
	if m.hovered.Kind == k || active {
		return stHover.Width(w).Background(colPanel).Render(" " + label)
	}
	return stItem.Width(w).Background(colPanel).Render(" " + label)
}

func (m *model) paintSession(x, y, w, h int) {
	compH := composerRows
	if compH > h-3 {
		compH = max(h-3, 0)
	}
	transH := h - compH
	m.reg.transcript = Rect{x, y, w, transH}
	m.reg.composer = Rect{x, y + transH, w, compH}
	m.vp.Width = max(w, 8)
	m.vp.Height = max(transH, 3)

	body := m.vp.View()
	if parts := strings.Split(body, "\n"); len(parts) > transH {
		body = strings.Join(parts[:transH], "\n")
	}
	body = m.paintTransSelection(body, transH)
	m.scr.blit(x, y, body)
	m.hits.Add(Target{Kind: KindTranscript, Rect: m.reg.transcript})
	if compH > 0 {
		m.paintComposer(x, y+transH, w, compH)
	}
}

func (m *model) paintComposer(x, y, w, h int) {
	if w < 12 || h < 4 {
		return
	}
	b := lipgloss.NormalBorder()
	border := lipgloss.NewStyle().Foreground(colBorder).Background(colPanel)
	fill := lipgloss.NewStyle().Background(colPanel)
	top := border.Render(b.TopLeft + strings.Repeat(b.Top, max(w-2, 0)) + b.TopRight)
	bot := border.Render(b.BottomLeft + strings.Repeat(b.Bottom, max(w-2, 0)) + b.BottomRight)
	side := border.Render(b.Left)

	m.scr.blit(x, y, cutVis(top, 0, w))
	innerY := y + 1
	innerW := max(w-2, 1)

	stop := stStop.Render(glyphStop)
	if m.hovered.Kind == KindCancel {
		stop = stSel.Render(glyphStop)
	}
	boxTop, boxMid, _ := m.usageBox()
	railW := max(lipgloss.Width(stop), lipgloss.Width(boxMid))
	railX := x + w - 1 - railW
	inputW := max(railX-(x+1), 8)
	m.ta.SetWidth(max(inputW-1, 8))
	m.ta.SetHeight(3)
	textW := max(inputW, 1)
	raw := m.ta.Value()
	wrapped := wrapField(raw, textW)
	cLine := caretLine(raw, textW, m.selCaret)
	if cLine < m.promptOff {
		m.promptOff = cLine
	}
	if cLine >= m.promptOff+3 {
		m.promptOff = max(cLine-2, 0)
	}
	m.promptOff = visibleStart(len(wrapped), 3, m.promptOff)

	for i := 0; i < 3 && innerY+i < y+h-2; i++ {
		var text string
		if raw == "" && i == 0 {
			if m.ta.Placeholder != "" && !(m.cursorOn && m.page == pageSession) {
				text = padPlain(m.ta.Placeholder, inputW)
			} else {
				text = m.paintEditLine(nil, 0, inputW)
			}
		} else {
			li := m.promptOff + i
			if li < len(wrapped) {
				ln := wrapped[li]
				text = m.paintEditLine(ln.runes, ln.start, inputW)
			} else {
				text = padPlain("", inputW)
			}
		}
		right := fill.Width(railW).Render("")
		switch i {
		case 0:
			right = fill.Width(railW).Render(stop)
		case 1:
			right = fill.Width(railW).Render(boxTop)
		case 2:
			right = fill.Width(railW).Render(boxMid)
		}
		gap := max(innerW-inputW-lipgloss.Width(right), 0)
		row := side + text + fill.Render(strings.Repeat(" ", gap)) + right + side
		m.scr.blit(x, innerY+i, cutVis(row, 0, w))
	}

	m.reg.prompt = Rect{x + 1, innerY, inputW, 3}
	m.reg.send = Rect{}
	m.reg.cancel = Rect{railX, innerY, railW, 1}
	m.reg.usage = Rect{railX, innerY + 1, railW, 2}
	m.hits.Add(Target{Kind: KindPrompt, Rect: m.reg.prompt})
	m.hits.Add(Target{Kind: KindCancel, Rect: m.reg.cancel})
	m.hits.Add(Target{Kind: KindUsage, Rect: m.reg.usage})

	hintY := innerY + 3
	if hintY >= y+h-1 {
		hintY = y + h - 2
	}
	m.scr.blit(x, hintY, side)
	cx := x + 1
	phase := stMuted.Background(colPanel).Render(m.phaseLabel())
	m.scr.blit(cx, hintY, phase)
	cx += lipgloss.Width(phase) + 1
	cx += m.paintChip(cx, hintY, m.currentMode(), KindComposerMode, m.float == floatMode)
	cx++
	cx += m.paintChip(cx, hintY, truncate(m.currentModelLabel(), 24), KindComposerModel, m.float == floatModel)
	cx++
	health := m.modelHealth
	if health == "" {
		health = "…"
	}
	hs := stMuted.Background(colPanel)
	switch health {
	case "ok":
		hs = stOk.Background(colPanel)
	case "err":
		hs = stErr.Background(colPanel)
	}
	word := hs.Render(health)
	m.scr.blit(cx, hintY, word)
	cx += lipgloss.Width(word)
	remain := x + 1 + innerW - cx
	if remain > 0 {
		m.scr.blit(cx, hintY, fill.Render(strings.Repeat(" ", remain)))
	}
	m.scr.blit(x+w-1, hintY, side)
	m.scr.blit(x, hintY+1, cutVis(bot, 0, w))
}

func (m *model) paintChipCell(label string, k Kind, open bool) string {
	cell := stChip.Background(colPanel).Render(" " + label + " ")
	if m.hovered.Kind == k || open {
		cell = stSel.Render(" " + label + " ")
	}
	return cell
}

func (m *model) paintChip(x, y int, label string, k Kind, open bool) int {
	cell := m.paintChipCell(label, k, open)
	w := lipgloss.Width(cell)
	m.hits.Add(Target{Kind: k, Rect: Rect{x, y, w, 1}})
	m.scr.blit(x, y, cell)
	return w
}

func (m *model) paintModal(tx, ty, tw, th int) {
	if m.float == floatUsage {
		m.paintUsageFloat()
		return
	}
	if m.float == floatSessions {
		m.paintSessionsFloat(tx, ty, tw, th)
		return
	}
	if m.float == floatWorkspace {
		m.paintWorkspaceFloat(tx, ty, tw, th)
		return
	}
	if m.float == floatMentions {
		m.paintMentionsFloat(tx, ty, tw, th)
		return
	}
	mw := min(tw-4, 56)
	if mw < 20 {
		mw = min(tw, 20)
	}
	title := "menu"
	items := m.floatItems()
	idx := m.palIdx
	kind := KindModalItem
	switch {
	case m.approval != nil:
		title = "approve " + m.approval.Tool
		items = []string{"allow once", "allow session", "deny"}
		idx = m.choice
		kind = KindApprove
	case m.float == floatMode:
		title = "mode"
	case m.float == floatModel:
		title = "model"
	}
	innerH := 1 + len(items)
	if m.approval != nil {
		innerH++
	}
	mh := innerH + 2
	if mh > th {
		mh = th
	}
	mx := tx + 2
	my := ty + max(th-mh, 0)
	m.reg.modal = Rect{mx, my, mw, mh}
	m.hits.Add(Target{Kind: KindModalDismiss, Rect: m.reg.transcript})
	m.hits.Add(Target{Kind: KindNone, Rect: m.reg.modal})

	b := lipgloss.NormalBorder()
	border := lipgloss.NewStyle().Foreground(colAccent).Background(colPanel)
	fill := lipgloss.NewStyle().Background(colPanel)
	top := border.Render(b.TopLeft + strings.Repeat(b.Top, max(mw-2, 0)) + b.TopRight)
	bot := border.Render(b.BottomLeft + strings.Repeat(b.Bottom, max(mw-2, 0)) + b.BottomRight)
	side := border.Render(b.Left)

	cy := my
	m.scr.blit(mx, cy, top)
	cy++
	cy += m.blitModalRow(mx, cy, mw, side, fill, stChip.Background(colPanel).Render(title))
	if m.approval != nil {
		sum := stMuted.Background(colPanel).Render(truncate(m.approval.Summary, max(mw-4, 4)))
		cy += m.blitModalRow(mx, cy, mw, side, fill, sum)
	}
	m.reg.listVis = max(my+mh-1-cy, 0)
	m.listOff = visibleStart(len(items), m.reg.listVis, m.listOff)
	for i := m.listOff; i < len(items) && cy < my+mh-1; i++ {
		it := items[i]
		var cell string
		if i == idx || (m.hovered.Kind == kind && m.hovered.Index == i) {
			cell = stSel.Render(" " + it + " ")
		} else {
			cell = stItem.Background(colPanel).Render(" " + it + " ")
		}
		m.hits.Add(Target{Kind: kind, Index: i, Rect: Rect{mx + 2, cy, max(mw-4, 1), 1}})
		cy += m.blitModalRow(mx, cy, mw, side, fill, cell)
	}
	m.scr.blit(mx, my+mh-1, bot)
}

func (m *model) paintSessionsFloat(_, _, tw, _ int) {
	mw := min(max(tw-4, 20), 56)
	if mw > m.width {
		mw = max(m.width, 8)
	}
	n := len(m.sessions)
	innerH := 2 + max(n, 1)
	mh := innerH + 2
	maxH := m.height - navRows
	if mh > maxH {
		mh = max(maxH, 3)
	}
	my := navRows
	clockR := m.reg.navClock
	mx := clockR.X + clockR.W - mw
	if mx < 0 {
		mx = 0
	}
	if mx+mw > m.width {
		mx = max(m.width-mw, 0)
	}
	m.reg.modal = Rect{mx, my, mw, mh}
	m.hits.Add(Target{Kind: KindModalDismiss, Rect: m.reg.transcript})
	m.hits.Add(Target{Kind: KindNone, Rect: m.reg.modal})

	b := lipgloss.NormalBorder()
	border := lipgloss.NewStyle().Foreground(colAccent).Background(colPanel)
	fill := lipgloss.NewStyle().Background(colPanel)
	top := border.Render(b.TopLeft + strings.Repeat(b.Top, max(mw-2, 0)) + b.TopRight)
	bot := border.Render(b.BottomLeft + strings.Repeat(b.Bottom, max(mw-2, 0)) + b.BottomRight)
	side := border.Render(b.Left)

	cy := my
	m.scr.blit(mx, cy, top)
	cy++
	cy += m.blitModalRow(mx, cy, mw, side, fill, stChip.Background(colPanel).Render("sessions"))

	del := "delete all"
	var delCell string
	if m.palIdx == 0 || m.hovered.Kind == KindSidebarDeleteAll {
		delCell = stSel.Render(" " + del + " ")
	} else {
		delCell = stItem.Background(colPanel).Render(" " + del + " ")
	}
	m.hits.Add(Target{Kind: KindSidebarDeleteAll, Rect: Rect{mx + 2, cy, max(mw-4, 1), 1}})
	cy += m.blitModalRow(mx, cy, mw, side, fill, delCell)

	if n == 0 {
		if cy < my+mh-1 {
			cy += m.blitModalRow(mx, cy, mw, side, fill, stMuted.Background(colPanel).Render(" no sessions"))
		}
	}
	labelW := max(mw-5, 1)
	m.reg.listVis = max(my+mh-1-cy, 0)
	m.listOff = visibleStart(n, m.reg.listVis, m.listOff)
	for i := m.listOff; i < n && cy < my+mh-1; i++ {
		id := m.sessions[i]
		label := truncate(m.sessionLabel(id), max(labelW-1, 1))
		sel := m.palIdx == i+1 || (m.hovered.Kind == KindSidebarSession && m.hovered.Index == i)
		var row string
		if sel {
			row = stSel.Width(labelW).Render(" " + label)
		} else {
			row = stItem.Width(labelW).Background(colPanel).Render(" " + label)
		}
		inner := row + fill.Render(" ") + m.trash(KindSidebarDelete, i)
		m.hits.Add(Target{Kind: KindSidebarSession, Index: i, Rect: Rect{mx + 2, cy, labelW, 1}})
		m.hits.Add(Target{Kind: KindSidebarDelete, Index: i, Rect: Rect{mx + mw - 2, cy, 1, 1}})
		cy += m.blitModalRow(mx, cy, mw, side, fill, inner)
	}
	m.scr.blit(mx, my+mh-1, bot)
}

func (m *model) paintWorkspaceFloat(_, _, tw, _ int) {
	rows := m.workspaceRows()
	mw := min(max(tw-4, 20), 56)
	if mw > m.width {
		mw = max(m.width, 8)
	}
	innerH := 1 + max(len(rows), 1)
	mh := innerH + 2
	maxH := m.height - navRows
	if mh > maxH {
		mh = max(maxH, 3)
	}
	my := navRows
	wsR := m.reg.navWorkspace
	mx := wsR.X
	if mx < 0 {
		mx = 0
	}
	if mx+mw > m.width {
		mx = max(m.width-mw, 0)
	}
	m.reg.modal = Rect{mx, my, mw, mh}
	m.hits.Add(Target{Kind: KindModalDismiss, Rect: m.reg.transcript})
	m.hits.Add(Target{Kind: KindNone, Rect: m.reg.modal})

	b := lipgloss.NormalBorder()
	border := lipgloss.NewStyle().Foreground(colAccent).Background(colPanel)
	fill := lipgloss.NewStyle().Background(colPanel)
	top := border.Render(b.TopLeft + strings.Repeat(b.Top, max(mw-2, 0)) + b.TopRight)
	bot := border.Render(b.BottomLeft + strings.Repeat(b.Bottom, max(mw-2, 0)) + b.BottomRight)
	side := border.Render(b.Left)

	cy := my
	m.scr.blit(mx, cy, top)
	cy++
	cy += m.blitModalRow(mx, cy, mw, side, fill, stChip.Background(colPanel).Render("workspace"))
	labelW := max(mw-5, 1)
	m.reg.listVis = max(my+mh-1-cy, 0)
	m.listOff = visibleStart(len(rows), m.reg.listVis, m.listOff)
	for i := m.listOff; i < len(rows) && cy < my+mh-1; i++ {
		r := rows[i]
		label := truncate(r.label, max(labelW-1, 1))
		sel := m.palIdx == i || (m.hovered.Kind == r.kind && (r.kind != KindWorkspaceDir || m.hovered.Index == r.index))
		var cell string
		if sel {
			cell = stSel.Width(labelW).Render(" " + label)
		} else {
			cell = stItem.Width(labelW).Background(colPanel).Render(" " + label)
		}
		m.hits.Add(Target{Kind: r.kind, Index: r.index, Rect: Rect{mx + 2, cy, max(mw-4, 1), 1}})
		cy += m.blitModalRow(mx, cy, mw, side, fill, cell)
	}
	m.scr.blit(mx, my+mh-1, bot)
}

func (m *model) paintMentionsFloat(_, _, tw, _ int) {
	items := m.mentions
	mw := min(max(tw-4, 24), 56)
	if mw > m.width {
		mw = max(m.width, 8)
	}
	innerH := 1 + max(len(items), 1)
	mh := innerH + 2
	maxH := max(m.reg.composer.Y-navRows, 3)
	if mh > maxH {
		mh = maxH
	}
	mx := m.reg.composer.X + 2
	if mx+mw > m.width {
		mx = max(m.width-mw, 0)
	}
	my := m.reg.composer.Y - mh
	if my < navRows {
		my = navRows
	}
	m.reg.modal = Rect{mx, my, mw, mh}
	m.hits.Add(Target{Kind: KindModalDismiss, Rect: m.reg.transcript})
	m.hits.Add(Target{Kind: KindNone, Rect: m.reg.modal})

	b := lipgloss.NormalBorder()
	border := lipgloss.NewStyle().Foreground(colAccent).Background(colPanel)
	fill := lipgloss.NewStyle().Background(colPanel)
	top := border.Render(b.TopLeft + strings.Repeat(b.Top, max(mw-2, 0)) + b.TopRight)
	bot := border.Render(b.BottomLeft + strings.Repeat(b.Bottom, max(mw-2, 0)) + b.BottomRight)
	side := border.Render(b.Left)

	cy := my
	m.scr.blit(mx, cy, top)
	cy++
	cy += m.blitModalRow(mx, cy, mw, side, fill, stChip.Background(colPanel).Render("files"))
	if len(items) == 0 {
		if cy < my+mh-1 {
			cy += m.blitModalRow(mx, cy, mw, side, fill, stMuted.Background(colPanel).Render(" no files"))
		}
	}
	m.reg.listVis = max(my+mh-1-cy, 0)
	m.listOff = visibleStart(len(items), m.reg.listVis, m.listOff)
	for i := m.listOff; i < len(items) && cy < my+mh-1; i++ {
		it := items[i]
		label := truncate(it, max(mw-5, 1))
		var cell string
		if i == m.palIdx || (m.hovered.Kind == KindMention && m.hovered.Index == i) {
			cell = stSel.Render(" " + label + " ")
		} else {
			cell = stItem.Background(colPanel).Render(" " + label + " ")
		}
		m.hits.Add(Target{Kind: KindMention, Index: i, Rect: Rect{mx + 2, cy, max(mw-4, 1), 1}})
		cy += m.blitModalRow(mx, cy, mw, side, fill, cell)
	}
	m.scr.blit(mx, my+mh-1, bot)
}

func (m *model) blitModalRow(x, y, w int, side string, fill lipgloss.Style, inner string) int {
	pad := max(w-2-lipgloss.Width(inner), 0)
	row := side + inner + fill.Render(strings.Repeat(" ", pad)) + side
	m.scr.blit(x, y, cutVis(row, 0, w))
	return 1
}

func (m *model) paintVersion(x, y, w, h int) {
	cx := x + 2
	cy := y
	cy += m.scr.blit(cx, cy, stSideTitle.Render(" siggy"))
	cy++
	_ = m.scr.blit(cx, cy, stMuted.Render(" "+version.Value))
	_ = w
	_ = h
}

func (m *model) paintProviders(x, y, w, h int) {
	cx := x + 2
	cy := y
	cy += m.scr.blit(cx, cy, stSideTitle.Render(" providers"))
	plus := stAdd.Render("  +  ")
	if m.hovered.Kind == KindProviderNew {
		plus = stSel.Render("  +  ")
	}
	m.hits.Add(Target{Kind: KindProviderNew, Rect: Rect{cx, cy, max(lipgloss.Width(plus), 5), 1}})
	cy += m.scr.blit(cx, cy, plus)
	n := 0
	if m.cfg != nil {
		n = len(m.cfg.Providers)
	}
	if n == 0 {
		cy += m.scr.blit(cx, cy, stMuted.Render(" no connections"))
	}
	m.reg.provVis = max(y+h-cy, 0)
	m.provOff = visibleStart(n, m.reg.provVis, m.provOff)
	for i := m.provOff; i < n && cy < y+h; i++ {
		p := m.cfg.Providers[i]
		ex := cx
		edit := m.provIcon(glyphEdit, KindProviderEdit, i, stChip)
		ew := lipgloss.Width(edit)
		m.hits.Add(Target{Kind: KindProviderEdit, Index: i, Rect: Rect{ex, cy, ew, 1}})
		m.scr.blit(ex, cy, edit)
		ex += ew + 1
		del := m.provIcon(glyphQuit, KindProviderDelete, i, stQuit)
		dw := lipgloss.Width(del)
		m.hits.Add(Target{Kind: KindProviderDelete, Index: i, Rect: Rect{ex, cy, dw, 1}})
		m.scr.blit(ex, cy, del)
		ex += dw + 1
		mark := " "
		if p.Name == m.cfg.ActiveProvider {
			mark = "*"
		}
		nameW := max(x+w-1-ex, 4)
		row := fmt.Sprintf("%s %-12s  %-8s  %s", mark, truncate(p.Name, 12), firstProto(p), truncate(strings.Join(p.Models, ","), 24))
		var cell string
		if i == m.provIdx || (m.hovered.Kind == KindProviderRow && m.hovered.Index == i) {
			cell = stSel.Width(nameW).Render(row)
		} else {
			cell = stItem.Width(nameW).Render(row)
		}
		m.hits.Add(Target{Kind: KindProviderRow, Index: i, Rect: Rect{ex, cy, nameW, 1}})
		m.scr.blit(ex, cy, cell)
		cy++
	}
}

func (m *model) paintForm(x, y, w, h int) {
	cx := x + 2
	fw := min(max(w-4, 16), 60)
	saveY := y + h - 1
	bodyLimit := saveY
	if m.form.err != "" {
		bodyLimit = y + h - 2
	}
	cy := y
	title := "new provider"
	if m.form.original != "" {
		title = "edit " + m.form.original
	}
	if cy < bodyLimit {
		cy += m.scr.blit(cx, cy, stSideTitle.Render(" "+title))
		cy++
	}
	if cy < bodyLimit {
		cy += m.paintField(cx, cy, "name", m.form.name, 0, fw, bodyLimit)
	}
	if cy < bodyLimit {
		cy += m.paintField(cx, cy, "url", m.form.url, 1, fw, bodyLimit)
	}
	key := m.form.apiKey
	if m.form.field != 2 && key != "" {
		key = config.MaskKey(key)
	}
	if cy < bodyLimit {
		cy += m.paintField(cx, cy, "api key", key, 2, fw, bodyLimit)
	}

	if cy < bodyLimit {
		cy += m.scr.blit(cx, cy, stMuted.Render(" models"))
	}
	labelW := max(fw-2, 1)
	n := len(m.form.models)
	m.reg.formVis = max(bodyLimit-cy, 0)
	m.formOff = visibleStart(n, m.reg.formVis, m.formOff)
	for i := m.formOff; i < n && cy < bodyLimit; i++ {
		mod := m.form.models[i]
		prefix := "  "
		var cell string
		focused := i == m.form.modelIdx && m.form.field == 3
		if focused {
			cell = prefix + m.paintEditLine([]rune(mod), 0, max(labelW-2, 1))
		} else if m.hovered.Kind == KindFormListItem && m.hovered.Index == i {
			cell = stHover.Width(labelW).Render(prefix + mod)
		} else {
			cell = stItem.Width(labelW).Render(prefix + mod)
		}
		m.hits.Add(Target{Kind: KindFormListItem, Index: i, Rect: Rect{cx + 2, cy, max(labelW-2, 1), 1}})
		m.scr.blit(cx, cy, cell)
		m.hits.Add(Target{Kind: KindFormDeleteModel, Index: i, Rect: Rect{cx + fw - 1, cy, 1, 1}})
		m.scr.blit(cx+fw-1, cy, m.trash(KindFormDeleteModel, i))
		cy++
	}
	if cy < bodyLimit {
		add := " + add model"
		if m.hovered.Kind == KindFormAdd {
			add = stSel.Render(add)
		} else {
			add = stMuted.Render(add)
		}
		m.hits.Add(Target{Kind: KindFormAdd, Rect: Rect{cx, cy, max(lipgloss.Width(add), 14), 1}})
		cy += m.scr.blit(cx, cy, add)
		if cy < bodyLimit {
			cy++
		}
	}

	if cy < bodyLimit {
		cy += m.scr.blit(cx, cy, stMuted.Render(" protocols"))
	}
	for i, opt := range protocolOptions {
		if cy >= bodyLimit {
			break
		}
		box := "[ ]"
		if containsStr(m.form.protocols, opt.id) {
			box = "[x]"
		}
		row := " " + box + " " + opt.label
		focused := m.form.field == 10+i || (m.hovered.Kind == KindFormField && m.hovered.Index == 10+i)
		var cell string
		switch {
		case !opt.enabled:
			cell = stMuted.Render(row)
		case focused:
			cell = stSel.Render(row)
		default:
			cell = stItem.Render(row)
		}
		m.hits.Add(Target{Kind: KindFormField, Index: 10 + i, Rect: Rect{cx, cy, fw, 1}})
		cy += m.scr.blit(cx, cy, cell)
	}
	m.paintBtnRow(cx, saveY, []struct {
		k     Kind
		label string
	}{
		{KindFormSave, " save "},
		{KindFormCancel, " cancel "},
	})
	if m.form.err != "" {
		m.scr.blit(cx, y+h-2, stErr.Render(m.form.err))
	}
}

func (m *model) paintField(x, y int, label, value string, idx, w, limitY int) int {
	if y >= limitY {
		return 0
	}
	cy := y
	cy += m.scr.blit(x, cy, stMuted.Render(label))
	if cy >= limitY {
		return cy - y
	}
	inner := padPlain(value, w)
	if m.form.field == idx {
		inner = m.paintEditLine([]rune(value), 0, w)
	}
	cy += m.scr.blit(x, cy, inner)
	m.hits.Add(Target{Kind: KindFormField, Index: idx, Rect: Rect{x, cy - 1, max(w, 1), 1}})
	return cy - y + 1
}

func (m *model) paintBtnRow(x, y int, btns []struct {
	k     Kind
	label string
}) {
	cx := x
	for _, b := range btns {
		cell := m.btn(b.label, b.k)
		cw := lipgloss.Width(cell)
		m.hits.Add(Target{Kind: b.k, Rect: Rect{cx, y, cw, 1}})
		m.scr.blit(cx, y, cell)
		cx += cw + 1
	}
}

func (m model) trash(k Kind, index int) string {
	if m.hovered.Kind == k && m.hovered.Index == index {
		return stSel.Render("✕")
	}
	return stMuted.Background(colPanel).Render("✕")
}

func (m model) provIcon(glyph string, k Kind, index int, st lipgloss.Style) string {
	if m.hovered.Kind == k && m.hovered.Index == index {
		return stSel.Render(glyph)
	}
	return st.Background(colPanel).Render(glyph)
}

func (m model) btn(label string, k Kind) string {
	if m.hovered.Kind == k {
		return stSel.Render(label)
	}
	return stBtn.Render(label)
}

func firstProto(p config.Provider) string {
	if len(p.Protocols) == 0 {
		return config.ProtocolOpenAI
	}
	return p.Protocols[0]
}
