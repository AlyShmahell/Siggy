package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *model) focusTranscript() {
	m.focusTrans = true
	m.ta.Blur()
}

func (m *model) blurTranscript() {
	m.focusTrans = false
	m.transCollapse()
	m.ta.Focus()
}

func (m *model) transJoined() string {
	return strings.Join(m.transPlain, "\n")
}

func (m *model) syncTransPlain(content string) {
	m.transPlain = splitPlainRows(strings.TrimRight(content, "\n"))
	n := utf8.RuneCountInString(m.transJoined())
	m.transCaret = clamp(m.transCaret, 0, n)
	m.transAnchor = clamp(m.transAnchor, 0, n)
}

func (m *model) hasTransSel() bool {
	return m.transAnchor != m.transCaret
}

func (m *model) transRange() (lo, hi int) {
	lo, hi = m.transAnchor, m.transCaret
	if lo > hi {
		lo, hi = hi, lo
	}
	n := utf8.RuneCountInString(m.transJoined())
	return clamp(lo, 0, n), clamp(hi, 0, n)
}

func (m *model) transSelectAll() {
	m.transAnchor = 0
	m.transCaret = utf8.RuneCountInString(m.transJoined())
}

func (m *model) transCollapse() {
	m.transAnchor = m.transCaret
}

func (m *model) transSelectedText() string {
	if !m.hasTransSel() {
		return ""
	}
	lo, hi := m.transRange()
	r := []rune(m.transJoined())
	if lo >= hi || lo >= len(r) {
		return ""
	}
	hi = min(hi, len(r))
	return string(r[lo:hi])
}

func (m *model) transCopy() tea.Cmd {
	s := m.transSelectedText()
	if s == "" {
		return nil
	}
	m.editClip = s
	_ = clipboard.WriteAll(s)
	return oscSetClipboard(s)
}

func transOffset(rows []string, row, col int) int {
	if len(rows) == 0 {
		return 0
	}
	row = clamp(row, 0, len(rows)-1)
	off := 0
	for i := 0; i < row; i++ {
		off += utf8.RuneCountInString(rows[i]) + 1
	}
	n := utf8.RuneCountInString(rows[row])
	if col > n {
		col = n
	}
	if col < 0 {
		col = 0
	}
	return off + col
}

func transRowCol(rows []string, off int) (row, col int) {
	if len(rows) == 0 {
		return 0, 0
	}
	if off < 0 {
		off = 0
	}
	for i, s := range rows {
		n := utf8.RuneCountInString(s)
		if off <= n {
			return i, off
		}
		off -= n + 1
		if off < 0 {
			return i, n
		}
	}
	last := rows[len(rows)-1]
	return len(rows) - 1, utf8.RuneCountInString(last)
}

func splitPlainRows(s string) []string {
	parts := strings.Split(s, "\n")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = stripANSI(p)
	}
	return out
}

func (m *model) transcriptClickOffset(x, y int) int {
	relY := y - m.reg.transcript.Y
	relX := x - m.reg.transcript.X
	row := m.vp.YOffset + relY
	return transOffset(m.transPlain, row, relX)
}

func (m *model) moveTransCaret(dx, dy int) {
	if len(m.transPlain) == 0 {
		return
	}
	row, col := transRowCol(m.transPlain, m.transCaret)
	row = clamp(row+dy, 0, len(m.transPlain)-1)
	n := utf8.RuneCountInString(m.transPlain[row])
	col = clamp(col+dx, 0, n)
	m.transCaret = transOffset(m.transPlain, row, col)
	m.transCollapse()
	vis := max(m.vp.Height, 1)
	if row < m.vp.YOffset {
		m.vp.SetYOffset(row)
	}
	if row >= m.vp.YOffset+vis {
		m.vp.SetYOffset(row - vis + 1)
	}
}

func (m *model) paintTransSelection(body string, transH int) string {
	if !m.hasTransSel() || transH < 1 {
		return body
	}
	styled := strings.Split(body, "\n")
	lo, hi := m.transRange()
	var out []string
	for i := 0; i < transH && i < len(styled); i++ {
		row := m.vp.YOffset + i
		if row < 0 || row >= len(m.transPlain) {
			out = append(out, styled[i])
			continue
		}
		start := transOffset(m.transPlain, row, 0)
		plain := m.transPlain[row]
		end := start + utf8.RuneCountInString(plain)
		if hi <= start || lo >= end {
			out = append(out, styled[i])
			continue
		}
		out = append(out, paintTransRow(plain, start, lo, hi))
	}
	return strings.Join(out, "\n")
}

func paintTransRow(plain string, start, lo, hi int) string {
	var b strings.Builder
	for i, ch := range []rune(plain) {
		off := start + i
		g := string(ch)
		if off >= lo && off < hi {
			b.WriteString(stSel.Render(g))
		} else {
			b.WriteString(g)
		}
	}
	return b.String()
}

func wrapVisual(s string, width int) string {
	if width < 8 {
		return s
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		rest := para
		for lipgloss.Width(rest) > width {
			lines = append(lines, cutVis(rest, 0, width))
			rest = cutVis(rest, width, lipgloss.Width(rest))
			if rest == "" {
				return strings.Join(lines, "\n")
			}
		}
		lines = append(lines, rest)
	}
	return strings.Join(lines, "\n")
}
