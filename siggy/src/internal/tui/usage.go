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

func (m *model) usageUsed() int {
	if m.tokensUsed > 0 {
		return m.tokensUsed
	}
	n := 0
	var specs []llm.ToolSpec
	if m.g != nil && m.g.Engine != nil {
		if m.g.Engine.Tools != nil {
			specs = m.g.Engine.Tools.Specs()
		}
		n = loop.EstimateRequest(m.g.Engine.Messages, specs)
	}
	n += utf8.RuneCountInString(m.ta.Value()) / 4
	return n
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

func (m *model) usageBox() (top, mid, bot string) {
	used := m.usageUsed()
	limit := m.contextWindow()
	label := usageBadge(used, limit)
	if utf8.RuneCountInString(label) < 4 {
		label = " " + label
	}
	col := usageColor(used, limit)
	if m.hovered.Kind == KindUsage || m.float == floatUsage {
		st := lipgloss.NewStyle().Foreground(colBg).Background(col)
		inner := " " + label + " "
		w := lipgloss.Width(inner)
		if w < 5 {
			inner = padPlain(inner, 5)
			w = 5
		}
		top = st.Render("┌" + strings.Repeat("─", w) + "┐")
		mid = st.Render("│") + st.Bold(true).Render(inner) + st.Render("│")
		bot = st.Render("└" + strings.Repeat("─", w) + "┘")
		return top, mid, bot
	}
	st := lipgloss.NewStyle().Foreground(col).Background(colPanel)
	inner := " " + label + " "
	w := lipgloss.Width(inner)
	if w < 5 {
		inner = padPlain(inner, 5)
		w = 5
	}
	top = st.Render("┌" + strings.Repeat("─", w) + "┐")
	mid = st.Render("│") + st.Bold(true).Render(inner) + st.Render("│")
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
