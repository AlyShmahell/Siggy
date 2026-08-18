package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func forceColor(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

func TestClickIndexFrontAndBehind(t *testing.T) {
	s := "abcdef"
	if got := clickIndex(s, 80, 0, 0); got != 0 {
		t.Fatalf("in front of first: %d", got)
	}
	if got := clickIndex(s, 80, 1, 0); got != 1 {
		t.Fatalf("in front of second: %d", got)
	}
	if got := clickIndex(s, 80, 6, 0); got != 6 {
		t.Fatalf("behind last: %d", got)
	}
	if got := clickIndex(s, 80, 20, 0); got != 6 {
		t.Fatalf("past end: %d", got)
	}
}

func TestClickIndexWrapped(t *testing.T) {
	s := "abcdefgh"
	if got := clickIndex(s, 4, 0, 1); got != 4 {
		t.Fatalf("second wrap line start: %d", got)
	}
}

func TestSpliceRunes(t *testing.T) {
	if got := spliceRunes("abcdef", 1, 4, "XY"); got != "aXYef" {
		t.Fatalf("got %q", got)
	}
}

func TestSanitizeInsertStripsSGR(t *testing.T) {
	if got := sanitizeInsert("\x1b[1;30;47msecret\x1b[0m"); got != "secret" {
		t.Fatalf("got %q", got)
	}
	if got := sanitizeInsert("a\tb\x00c\nd"); got != "a bc\nd" {
		t.Fatalf("controls = %q", got)
	}
}

func TestStyleSegmentNoTrailingCaret(t *testing.T) {
	m := model{cursorOn: true, selCaret: 3}
	got := m.styleSegment([]rune("abc"), 0)
	if strings.Contains(got, stSel.Render(" ")) {
		t.Fatal("styleSegment must not append a trailing caret space")
	}
	if stripANSI(got) != "abc" {
		t.Fatalf("text = %q", stripANSI(got))
	}
}

func TestPaintEditLineOneCellCaret(t *testing.T) {
	forceColor(t)
	m := model{cursorOn: true, selCaret: 0}
	const w = 20
	line := m.paintEditLine(nil, 0, w)
	if lipgloss.Width(line) != w {
		t.Fatalf("width = %d", lipgloss.Width(line))
	}
	if n := strings.Count(line, stSel.Render(" ")); n != 1 {
		t.Fatalf("want one caret cell, got %d in %q", n, line)
	}
	if plain := stripANSI(line); strings.Contains(plain, "1;30;47") || strings.Contains(plain, "[0m") {
		t.Fatalf("visible SGR leak %q", plain)
	}
}

func TestPaintEditLineCaretOnlyOnOwnerLine(t *testing.T) {
	forceColor(t)
	m := model{cursorOn: true, selCaret: 3}
	line0 := m.paintEditLine([]rune("ab"), 0, 20)
	line1 := m.paintEditLine([]rune("cd"), 3, 20)
	if strings.Contains(line0, stSel.Render(" ")) || strings.Contains(line0, stSel.Render("a")) || strings.Contains(line0, stSel.Render("b")) {
		t.Fatalf("caret leaked onto line 0: %q", line0)
	}
	if n := strings.Count(line1, stSel.Render("c")); n != 1 {
		t.Fatalf("want one caret on line 1, got %d", n)
	}
	if strings.Contains(line1, stSel.Render("d")) {
		t.Fatal("caret stretched onto second rune")
	}
}
