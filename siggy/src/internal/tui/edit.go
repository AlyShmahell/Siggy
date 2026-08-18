package tui

import (
	"encoding/base64"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	dblClickMax     = 200 * time.Millisecond
	cursorBlinkOn   = 530 * time.Millisecond
	cursorBlinkIdle = 2 * time.Second
)

type cursorTickMsg struct{}

type wrapLine struct {
	runes []rune
	start int
}

func wrapField(s string, width int) []wrapLine {
	if width < 1 {
		width = 1
	}
	r := []rune(s)
	var lines []wrapLine
	start := 0
	col := 0
	for i, ch := range r {
		if ch == '\n' {
			lines = append(lines, wrapLine{runes: r[start:i], start: start})
			start = i + 1
			col = 0
			continue
		}
		if col >= width && i > start {
			lines = append(lines, wrapLine{runes: r[start:i], start: start})
			start = i
			col = 0
		}
		col++
	}
	lines = append(lines, wrapLine{runes: r[start:], start: start})
	if len(lines) == 0 {
		lines = []wrapLine{{start: 0}}
	}
	return lines
}

func clickIndex(s string, width, relX, relY int) int {
	n := utf8.RuneCountInString(s)
	if relY < 0 {
		relY = 0
	}
	if relX < 0 {
		relX = 0
	}
	lines := wrapField(s, width)
	if relY >= len(lines) {
		return n
	}
	ln := lines[relY]
	if relX >= len(ln.runes) {
		end := ln.start + len(ln.runes)
		if end < n && []rune(s)[end] == '\n' {
			return end
		}
		return end
	}
	return ln.start + relX
}

func caretLine(s string, width, caret int) int {
	lines := wrapField(s, width)
	for i, ln := range lines {
		end := ln.start + len(ln.runes)
		if caret <= end {
			return i
		}
	}
	return max(len(lines)-1, 0)
}

func (m *model) hasSel() bool {
	return m.selAnchor != m.selCaret
}

func (m *model) selRange() (lo, hi int) {
	lo, hi = m.selAnchor, m.selCaret
	if lo > hi {
		lo, hi = hi, lo
	}
	n := utf8.RuneCountInString(m.editValue())
	return clamp(lo, 0, n), clamp(hi, 0, n)
}

func (m *model) collapseSel() {
	m.selAnchor = m.selCaret
}

func (m *model) selectAll() {
	n := utf8.RuneCountInString(m.editValue())
	m.selAnchor = 0
	m.selCaret = n
}

func isTextInput(t Target) bool {
	switch t.Kind {
	case KindPrompt, KindFormListItem:
		return true
	case KindFormField:
		return t.Index >= 0 && t.Index <= 2
	}
	return false
}

func (m *model) editValue() string {
	if m.page == pageProviderForm {
		return m.formValue()
	}
	return m.ta.Value()
}

func (m *model) formValue() string {
	switch {
	case m.form.field == 0:
		return m.form.name
	case m.form.field == 1:
		return m.form.url
	case m.form.field == 2:
		return m.form.apiKey
	case m.form.field == 3 && m.form.modelIdx >= 0 && m.form.modelIdx < len(m.form.models):
		return m.form.models[m.form.modelIdx]
	}
	return ""
}

func (m *model) setEditValue(s string) {
	if m.page == pageProviderForm {
		m.setFormValue(s)
		return
	}
	m.ta.SetValue(s)
}

func (m *model) setFormValue(s string) {
	switch {
	case m.form.field == 0:
		m.form.name = s
	case m.form.field == 1:
		m.form.url = s
	case m.form.field == 2:
		m.form.apiKey = s
	case m.form.field == 3 && m.form.modelIdx >= 0 && m.form.modelIdx < len(m.form.models):
		m.form.models[m.form.modelIdx] = s
	}
}

func spliceRunes(s string, lo, hi int, ins string) string {
	r := []rune(s)
	n := len(r)
	lo = clamp(lo, 0, n)
	hi = clamp(hi, 0, n)
	if hi < lo {
		lo, hi = hi, lo
	}
	out := make([]rune, 0, lo+(n-hi)+utf8.RuneCountInString(ins))
	out = append(out, r[:lo]...)
	out = append(out, []rune(ins)...)
	out = append(out, r[hi:]...)
	return string(out)
}

func (m *model) selectedText() string {
	if !m.hasSel() {
		return ""
	}
	lo, hi := m.selRange()
	r := []rune(m.editValue())
	if lo >= hi || lo >= len(r) {
		return ""
	}
	hi = min(hi, len(r))
	return string(r[lo:hi])
}

func sanitizeInsert(s string) string {
	s = stripANSI(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteRune('\n')
		case r == '\t':
			b.WriteByte(' ')
		case r < 32 || r == 127:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (m *model) insertText(s string) {
	s = sanitizeInsert(s)
	if s == "" {
		return
	}
	m.noteEdit()
	if m.hasSel() {
		lo, hi := m.selRange()
		m.setEditValue(spliceRunes(m.editValue(), lo, hi, s))
		m.selCaret = lo + utf8.RuneCountInString(s)
		m.collapseSel()
		return
	}
	off := clamp(m.selCaret, 0, utf8.RuneCountInString(m.editValue()))
	m.setEditValue(spliceRunes(m.editValue(), off, off, s))
	m.selCaret = off + utf8.RuneCountInString(s)
	m.collapseSel()
}

func (m *model) editBackspace() {
	m.noteEdit()
	if m.hasSel() {
		lo, hi := m.selRange()
		m.setEditValue(spliceRunes(m.editValue(), lo, hi, ""))
		m.selCaret = lo
		m.collapseSel()
		return
	}
	if m.selCaret <= 0 {
		return
	}
	m.setEditValue(spliceRunes(m.editValue(), m.selCaret-1, m.selCaret, ""))
	m.selCaret--
	m.collapseSel()
}

func (m *model) moveCaret(delta int, extend bool) {
	n := utf8.RuneCountInString(m.editValue())
	if !extend && m.hasSel() && delta != 0 {
		lo, hi := m.selRange()
		if delta < 0 {
			m.selCaret = lo
		} else {
			m.selCaret = hi
		}
		m.collapseSel()
		return
	}
	if extend && !m.hasSel() {
		m.selAnchor = m.selCaret
	}
	m.selCaret = clamp(m.selCaret+delta, 0, n)
	if !extend {
		m.collapseSel()
	}
}

func (m *model) placeCaret(off int, extend bool) {
	n := utf8.RuneCountInString(m.editValue())
	off = clamp(off, 0, n)
	if extend {
		if !m.hasSel() {
			m.selAnchor = m.selCaret
		}
		m.selCaret = off
		return
	}
	m.selCaret = off
	m.collapseSel()
}

func (m *model) editCopy() tea.Cmd {
	s := m.selectedText()
	if s == "" {
		return nil
	}
	m.editClip = s
	_ = clipboard.WriteAll(s)
	return oscSetClipboard(s)
}

func (m *model) editCut() tea.Cmd {
	cmd := m.editCopy()
	if !m.hasSel() {
		return cmd
	}
	m.noteEdit()
	lo, _ := m.selRange()
	m.setEditValue(spliceRunes(m.editValue(), lo, m.selRangeHi(), ""))
	m.selCaret = lo
	m.collapseSel()
	return cmd
}

func (m *model) selRangeHi() int {
	_, hi := m.selRange()
	return hi
}

func (m *model) editPaste(s string) {
	if s == "" {
		if clip, err := clipboard.ReadAll(); err == nil && clip != "" {
			s = clip
		}
	}
	if s == "" {
		s = m.editClip
	}
	if s == "" {
		return
	}
	m.insertText(s)
}

func oscSetClipboard(s string) tea.Cmd {
	return func() tea.Msg {
		b64 := base64.StdEncoding.EncodeToString([]byte(s))
		_, _ = os.Stdout.WriteString("\x1b]52;c;" + b64 + "\a")
		return nil
	}
}

func tickCursor() tea.Cmd {
	return tea.Tick(cursorBlinkOn, func(time.Time) tea.Msg { return cursorTickMsg{} })
}

func (m *model) noteEdit() {
	m.lastEdit = time.Now()
	m.cursorOn = true
}

func (m model) shouldBlink() bool {
	if m.page != pageSession || m.modal() || m.lastEdit.IsZero() {
		return false
	}
	return time.Since(m.lastEdit) < cursorBlinkIdle
}

func (m *model) startBlink() tea.Cmd {
	if !m.shouldBlink() || m.cursorTicking {
		return nil
	}
	m.cursorTicking = true
	return tickCursor()
}

func (m *model) styleSegment(r []rune, start int) string {
	lo, hi := m.selRange()
	var b strings.Builder
	for i, ch := range r {
		off := start + i
		g := string(ch)
		if ch == '\t' {
			g = " "
		}
		if off >= lo && off < hi {
			b.WriteString(stSel.Render(g))
		} else {
			b.WriteString(g)
		}
	}
	return b.String()
}

func padPlain(s string, w int) string {
	if w < 1 {
		return ""
	}
	r := []rune(s)
	plain := stItem.Background(colPanel)
	var b strings.Builder
	for i := 0; i < w; i++ {
		g := " "
		if i < len(r) {
			g = string(r[i])
			if r[i] == '\t' {
				g = " "
			}
		}
		b.WriteString(plain.Render(g))
	}
	return b.String()
}

func (m *model) paintEditLine(r []rune, start, w int) string {
	if w < 1 {
		return ""
	}
	lo, hi := m.selRange()
	caretCol := m.selCaret - start
	showCaret := m.cursorOn && caretCol >= 0 && caretCol <= len(r) && caretCol < w
	plain := stItem.Background(colPanel)
	var b strings.Builder
	for col := 0; col < w; col++ {
		g := " "
		inText := col < len(r)
		if inText {
			g = string(r[col])
			if r[col] == '\t' {
				g = " "
			}
		}
		off := start + col
		switch {
		case showCaret && col == caretCol:
			b.WriteString(stSel.Render(g))
		case inText && off >= lo && off < hi:
			b.WriteString(stSel.Render(g))
		default:
			b.WriteString(plain.Render(g))
		}
	}
	return b.String()
}

func (m *model) editWidth() int {
	if m.page == pageSession && m.reg.prompt.W > 0 {
		return m.reg.prompt.W
	}
	return max(m.ta.Width(), 1)
}

func (m *model) moveCaretLine(delta int, extend bool) {
	s := m.editValue()
	w := m.editWidth()
	lines := wrapField(s, w)
	li := caretLine(s, w, m.selCaret)
	if li < 0 || li >= len(lines) {
		return
	}
	col := m.selCaret - lines[li].start
	ni := clamp(li+delta, 0, len(lines)-1)
	if ni == li {
		if delta < 0 {
			m.placeCaret(0, extend)
		} else {
			m.placeCaret(utf8.RuneCountInString(s), extend)
		}
		return
	}
	m.placeCaret(clickIndex(s, w, col, ni), extend)
}

func (m *model) focusTextInput(t Target) {
	m.blurTranscript()
	switch t.Kind {
	case KindPrompt:
		m.ta.Focus()
	case KindFormField:
		if t.Index >= 0 && t.Index <= 2 {
			m.form.field = t.Index
		}
	case KindFormListItem:
		m.form.field = 3
		m.form.modelIdx = t.Index
	}
}

func (m *model) clickOffset(t Target, x, y int) int {
	s := m.editValue()
	switch t.Kind {
	case KindPrompt:
		w := max(t.Rect.W, 1)
		return clickIndex(s, w, x-t.Rect.X, y-t.Rect.Y)
	case KindFormField, KindFormListItem:
		relX := x - t.Rect.X
		n := utf8.RuneCountInString(s)
		if relX < 0 {
			return 0
		}
		if relX >= n {
			return n
		}
		return relX
	}
	return m.selCaret
}
