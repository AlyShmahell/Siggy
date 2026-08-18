package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTranscriptClickDragCopies(t *testing.T) {
	m := testModel(t)
	_ = m.View()
	tr, ok := firstKind(&m, KindTranscript)
	if !ok {
		t.Fatal("missing transcript")
	}
	x, y := tr.Rect.X, tr.Rect.Y
	next, _ := m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: x, Y: y})
	m = next.(model)
	if !m.focusTrans {
		t.Fatal("click should focus transcript")
	}
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion, X: x + 7, Y: y})
	m = next.(model)
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease, X: x + 7, Y: y})
	m = next.(model)
	if got := m.transSelectedText(); got != "session" {
		t.Fatalf("selected %q anchor=%d caret=%d plain=%q", got, m.transAnchor, m.transCaret, m.transJoined())
	}
	next, cmd := m.onKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(model)
	if isQuitCmd(cmd) {
		t.Fatal("ctrl+c with transcript selection should not quit")
	}
	if m.editClip != "session" {
		t.Fatalf("clip = %q", m.editClip)
	}
}

func TestTranscriptCtrlCNoSelDoesNotQuit(t *testing.T) {
	m := testModel(t)
	_ = m.View()
	tr, ok := firstKind(&m, KindTranscript)
	if !ok {
		t.Fatal("missing transcript")
	}
	next, _ := m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: tr.Rect.X, Y: tr.Rect.Y})
	m = next.(model)
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease, X: tr.Rect.X, Y: tr.Rect.Y})
	m = next.(model)
	if !m.focusTrans {
		t.Fatal("expected transcript focus")
	}
	_, cmd := m.onKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if isQuitCmd(cmd) {
		t.Fatal("ctrl+c without transcript selection should not quit")
	}
}

func TestComposerCtrlCWithoutSelStillQuits(t *testing.T) {
	m := testModel(t)
	m.ta.SetValue("keep")
	m.selCaret, m.selAnchor = 2, 2
	_ = m.View()
	prompt, ok := firstKind(&m, KindPrompt)
	if !ok {
		t.Fatal("missing prompt")
	}
	next, _ := m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: prompt.Rect.X, Y: prompt.Rect.Y})
	m = next.(model)
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease, X: prompt.Rect.X, Y: prompt.Rect.Y})
	m = next.(model)
	if m.focusTrans {
		t.Fatal("composer click should leave transcript")
	}
	_, cmd := m.onKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !isQuitCmd(cmd) {
		t.Fatal("composer ctrl+c with no selection should quit")
	}
}

func TestTranscriptArrowsMoveCaretWithoutSelecting(t *testing.T) {
	m := testModel(t)
	_ = m.View()
	tr, ok := firstKind(&m, KindTranscript)
	if !ok {
		t.Fatal("missing transcript")
	}
	next, _ := m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: tr.Rect.X, Y: tr.Rect.Y})
	m = next.(model)
	next, _ = m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease, X: tr.Rect.X, Y: tr.Rect.Y})
	m = next.(model)
	next, _ = m.onKey(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(model)
	next, _ = m.onKey(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(model)
	if m.hasTransSel() {
		t.Fatalf("arrows created selection %q", m.transSelectedText())
	}
	if m.transCaret != 2 {
		t.Fatalf("caret = %d", m.transCaret)
	}
}

func TestTranscriptCtrlASelectsAll(t *testing.T) {
	m := testModel(t)
	_ = m.View()
	tr, ok := firstKind(&m, KindTranscript)
	if !ok {
		t.Fatal("missing transcript")
	}
	next, _ := m.onMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: tr.Rect.X, Y: tr.Rect.Y})
	m = next.(model)
	next, _ = m.onKey(tea.KeyMsg{Type: tea.KeyCtrlA})
	m = next.(model)
	got := m.transSelectedText()
	if !strings.Contains(got, "session ready") {
		t.Fatalf("select all = %q", got)
	}
}
