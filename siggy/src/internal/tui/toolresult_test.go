package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"siggy/src/internal/harness"
	"siggy/src/internal/loop"
)

func TestToolResultKind(t *testing.T) {
	if toolResultKind("status 200\nok", nil) != "ok" {
		t.Fatal("200 should be ok")
	}
	if toolResultKind("status 403\nForbidden", nil) != "err" {
		t.Fatal("403 should be err")
	}
	if toolResultKind("status 404 content-type image/png (4 bytes)", nil) != "err" {
		t.Fatal("404 receipt should be err")
	}
	if toolResultKind("denied", nil) != "err" {
		t.Fatal("denied should be err")
	}
	if toolResultKind("hello", errors.New("exit status 8")) != "err" {
		t.Fatal("Go error should be err")
	}
	wget := "Connecting to pngimg.com (104.26.4.108:443)\nwget: server returned error: HTTP/1.1 404 Not Found\nexit status 8"
	if toolResultKind(wget, nil) != "err" {
		t.Fatal("wget exit status 8 should be err")
	}
	if toolResultKind("status 200\nexit status mentioned in a page", nil) != "ok" {
		t.Fatal("status 200 body should stay ok")
	}
}

func TestToolEndHTTPErrorIsRed(t *testing.T) {
	forceColor(t)
	m := testModel(t)
	next, _ := m.onEvent(loop.Event{
		Kind: loop.KindToolEnd,
		Tool: "web_fetch",
		Text: "status 403\nForbidden",
	})
	m = next.(model)
	last := m.lines[len(m.lines)-1]
	if last.kind != "err" {
		t.Fatalf("kind = %s want err", last.kind)
	}
	got := m.renderLine(last, 40)
	want := stErr.Render("  ↳ " + truncate(last.text, 40))
	if !strings.Contains(got, want) {
		t.Fatalf("not red: %q want %q", got, want)
	}

	next, _ = m.onEvent(loop.Event{
		Kind: loop.KindToolEnd,
		Tool: "web_fetch",
		Text: "status 200\n{\"ok\":true}",
	})
	m = next.(model)
	last = m.lines[len(m.lines)-1]
	if last.kind != "ok" {
		t.Fatalf("200 kind = %s", last.kind)
	}
}

func TestTranscriptReloadHTTPError(t *testing.T) {
	lines := transcriptFromRecords([]harness.Record{
		{Type: "tool", Tool: "web_fetch", Args: `{"url":"https://x"}`, Result: "status 404\nmissing"},
		{Type: "tool", Tool: "web_search", Args: `{"query":"q"}`, Result: "status 200\nhits"},
	})
	var kinds []string
	for _, ln := range lines {
		if ln.kind == "ok" || ln.kind == "err" {
			kinds = append(kinds, ln.kind+":"+ln.text[:10])
		}
	}
	if len(kinds) != 2 || !strings.HasPrefix(kinds[0], "err:") || !strings.HasPrefix(kinds[1], "ok:") {
		t.Fatalf("kinds = %v", kinds)
	}
}

func TestTranscriptReloadShellExit(t *testing.T) {
	wget := "Connecting to pngimg.com (104.26.4.108:443)\nwget: server returned error: HTTP/1.1 404 Not Found\nexit status 8"
	lines := transcriptFromRecords([]harness.Record{
		{Type: "tool", Tool: "shell", Args: `{"command":"wget -O duck.png https://x"}`, Result: wget},
		{Type: "tool", Tool: "web_search", Args: `{"query":"q"}`, Result: "status 200\nhits"},
	})
	var kinds []string
	for _, ln := range lines {
		if ln.kind == "ok" || ln.kind == "err" {
			kinds = append(kinds, ln.kind)
		}
	}
	if len(kinds) != 2 || kinds[0] != "err" || kinds[1] != "ok" {
		t.Fatalf("kinds = %v", kinds)
	}
}

func TestShellResultBoxWraps(t *testing.T) {
	forceColor(t)
	m := testModel(t)
	long := strings.Repeat("abcdefghij ", 20)
	next, _ := m.onEvent(loop.Event{Kind: loop.KindToolEnd, Tool: "shell", Text: long})
	m = next.(model)
	last := m.lines[len(m.lines)-1]
	if last.kind != "ok" || !strings.Contains(last.text, "abcdefghij") {
		t.Fatalf("store = kind=%s len=%d", last.kind, len(last.text))
	}
	got := m.renderLine(last, m.width)
	plain := stripANSI(got)
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected wrap: %q", plain)
	}
	if strings.Contains(plain, "  ↳ ") {
		t.Fatalf("used arrow: %q", plain)
	}
	for i, ln := range strings.Split(got, "\n") {
		if w := lipgloss.Width(ln); w > m.width {
			t.Fatalf("line %d width %d > %d", i, w, m.width)
		}
	}
}

func TestShellExitBoxIsRed(t *testing.T) {
	forceColor(t)
	m := testModel(t)
	text := "wget: server returned error: HTTP/1.1 404 Not Found\nexit status 8"
	next, _ := m.onEvent(loop.Event{Kind: loop.KindToolEnd, Tool: "shell", Text: text})
	m = next.(model)
	last := m.lines[len(m.lines)-1]
	if last.kind != "err" {
		t.Fatalf("kind = %s", last.kind)
	}
	got := m.renderLine(last, 40)
	plain := stripANSI(got)
	if strings.Contains(plain, "  ↳ ") {
		t.Fatalf("used arrow: %q", plain)
	}
	if !strings.Contains(plain, "exit status 8") {
		t.Fatalf("missing output: %q", plain)
	}
	want := renderShellOut(text, max(40-4, 12), true)
	if got != want {
		t.Fatalf("not err box:\n got %q\nwant %q", got, want)
	}
}

func TestShellDisplayCap8K(t *testing.T) {
	m := testModel(t)
	big := strings.Repeat("x", 9000)
	next, _ := m.onEvent(loop.Event{Kind: loop.KindToolEnd, Tool: "shell", Text: big})
	m = next.(model)
	last := m.lines[len(m.lines)-1]
	if !strings.HasSuffix(last.text, "…") || len(last.text) != 8192+len("…") {
		t.Fatalf("cap = %d suffix=%q", len(last.text), last.text[len(last.text)-3:])
	}
}
