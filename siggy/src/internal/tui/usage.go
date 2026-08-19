package tui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/loop"
)

func (m *model) contextWindow() int {
	if m.cfg != nil && m.cfg.ContextWindow > 0 {
		return m.cfg.ContextWindow
	}
	if m.g != nil && m.g.Engine != nil && m.g.Engine.ContextWindow > 0 {
		return m.g.Engine.ContextWindow
	}
	return 128000
}

func (m *model) usageParts() loop.UsageParts {
	var msgs []llm.Message
	var specs []llm.ToolSpec
	if m.g != nil && m.g.Engine != nil {
		msgs = m.g.Engine.Messages
		if m.g.Engine.Tools != nil {
			specs = m.g.Engine.Tools.Specs()
		}
	}
	return loop.EstimateParts(msgs, specs, m.ta.Value())
}

func (m *model) usageUsed() int {
	if m.tokensUsed > 0 {
		return m.tokensUsed
	}
	var msgs []llm.Message
	var specs []llm.ToolSpec
	if m.g != nil && m.g.Engine != nil {
		msgs = m.g.Engine.Messages
		if m.g.Engine.Tools != nil {
			specs = m.g.Engine.Tools.Specs()
		}
	}
	return loop.EstimateRequest(msgs, specs) + utf8.RuneCountInString(m.ta.Value())/4
}

func (m *model) applyUsageFromRecords(recs []harness.Record) {
	prompt, billed, est := loop.SumUsage(recs)
	m.tokensUsed = prompt
	m.billedTokens = billed
	m.billedEst = est
}

func usagePct(used, limit int) int {
	if limit <= 0 {
		return 0
	}
	if used < 0 {
		used = 0
	}
	p := used * 100 / limit
	if p > 100 {
		return 100
	}
	return p
}

func usageBadge(used, limit int) string {
	if limit <= 0 {
		return " — "
	}
	return fmt.Sprintf("%d%%", usagePct(used, limit))
}

func usageColor(used, limit int) lipgloss.Color {
	if limit <= 0 {
		return colMuted
	}
	th := loop.ComputeThresholds(limit)
	if used < th.Warn {
		return colAdd
	}
	if used >= th.Hard {
		return colQuit
	}
	if used < th.Auto {
		span := th.Auto - th.Warn
		if span <= 0 {
			return colAccent
		}
		f := float64(used-th.Warn) / float64(span)
		return lerpColor(colAdd, colAccent, f)
	}
	span := th.Hard - th.Auto
	if span <= 0 {
		return colErr
	}
	f := float64(used-th.Auto) / float64(span)
	return lerpColor(colAccent, colErr, f)
}

func lerpColor(a, b lipgloss.Color, f float64) lipgloss.Color {
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	ca, cb := hexRGB(a), hexRGB(b)
	return rgbColor([3]int{
		ca[0] + int(float64(cb[0]-ca[0])*f),
		ca[1] + int(float64(cb[1]-ca[1])*f),
		ca[2] + int(float64(cb[2]-ca[2])*f),
	})
}

func hexRGB(c lipgloss.Color) [3]int {
	s := strings.TrimPrefix(string(c), "#")
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil || len(s) != 6 {
		return [3]int{0x7d, 0x9a, 0x6e}
	}
	return [3]int{int((n >> 16) & 0xff), int((n >> 8) & 0xff), int(n & 0xff)}
}

func rgbColor(c [3]int) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", clampByte(c[0]), clampByte(c[1]), clampByte(c[2])))
}

func clampByte(n int) int {
	if n < 0 {
		return 0
	}
	if n > 255 {
		return 255
	}
	return n
}

func formatTok(n int) string {
	if n < 0 {
		n = 0
	}
	if n < 1000 {
		return strconv.Itoa(n)
	}
	if n < 10000 {
		t := (n + 50) / 100
		if t%10 == 0 {
			return fmt.Sprintf("%dk", t/10)
		}
		return fmt.Sprintf("%d.%dk", t/10, t%10)
	}
	return fmt.Sprintf("%dk", (n+500)/1000)
}

func usageBreakdownLine(p loop.UsageParts, maxW int) string {
	var parts []string
	if p.System > 0 {
		parts = append(parts, "s"+formatTok(p.System))
	}
	if p.Tools > 0 {
		parts = append(parts, "t"+formatTok(p.Tools))
	}
	if p.Chat > 0 {
		parts = append(parts, "c"+formatTok(p.Chat))
	}
	if p.Draft > 0 {
		parts = append(parts, "d"+formatTok(p.Draft))
	}
	s := strings.Join(parts, " ")
	if maxW > 0 {
		r := []rune(s)
		if len(r) > maxW {
			s = string(r[:maxW])
		}
	}
	return s
}

func padUsageInner(s string, w int) string {
	n := lipgloss.Width(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

func centerUsageInner(s string, w int) string {
	n := lipgloss.Width(s)
	if n >= w {
		return s
	}
	left := (w - n) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", w-n-left)
}

func (m *model) usageBox() (top, mid, bot string) {
	used := m.usageUsed()
	limit := m.contextWindow()
	label := usageBadge(used, limit)
	br := usageBreakdownLine(m.usageParts(), 0)
	if br == "" {
		br = "—"
	}
	col := usageColor(used, limit)
	brInner := " " + br + " "
	pctInner := " " + label + " "
	w := lipgloss.Width(brInner)
	if pw := lipgloss.Width(pctInner); pw > w {
		w = pw
	}
	if w < 5 {
		w = 5
	}
	brInner = padUsageInner(brInner, w)
	pctInner = centerUsageInner(pctInner, w)
	stFg, stBg := col, colPanel
	if m.hovered.Kind == KindUsage || m.float == floatUsage {
		stFg, stBg = colBg, col
	}
	st := lipgloss.NewStyle().Foreground(stFg).Background(stBg)
	top = st.Render("┌") + st.Render(brInner) + st.Render("┐")
	mid = st.Render("│") + st.Bold(true).Render(pctInner) + st.Render("│")
	bot = st.Render("└" + strings.Repeat("─", w) + "┘")
	return top, mid, bot
}

func hexGreenFamily(c lipgloss.Color) bool {
	s := strings.TrimPrefix(string(c), "#")
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil || len(s) != 6 {
		return false
	}
	g := int((n >> 8) & 0xff)
	r := int((n >> 16) & 0xff)
	return g > r+30
}

func hexRedFamily(c lipgloss.Color) bool {
	s := strings.TrimPrefix(string(c), "#")
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil || len(s) != 6 {
		return false
	}
	r := int((n >> 16) & 0xff)
	g := int((n >> 8) & 0xff)
	return r > g+20
}
