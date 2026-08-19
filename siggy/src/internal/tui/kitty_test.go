package tui

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestKittyTransmitAndPlace(t *testing.T) {
	raw := tinyPNG(t)
	got := kittyTransmitAndPlace(3, 10, 12, raw)
	if !strings.Contains(got, "f=100") || !strings.Contains(got, "a=T") {
		t.Fatalf("payload = %q", got)
	}
	if !strings.Contains(got, "\x1b_G") || !strings.Contains(got, "\x1b\\") {
		t.Fatalf("missing APC: %q", got)
	}
}

func TestViewImageNoAPCWhenDisabled(t *testing.T) {
	m := testModel(t)
	root := m.h.Workspace.Root
	if err := os.WriteFile(filepath.Join(root, "foo.png"), tinyPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	m.lines = append(m.lines, line{kind: "asst-live", text: "![diagram](foo.png)"})
	m.refresh()
	view := m.View()
	plain := stripANSI(view)
	if !strings.Contains(plain, "diagram") {
		t.Fatalf("missing caption:\n%s", plain)
	}
	if strings.Contains(view, "\x1b_G") {
		t.Fatalf("APC leaked:\n%q", view)
	}
}

func TestKittyImageReservesAndEncodes(t *testing.T) {
	t.Setenv("SIGGY_KITTY_IMAGES", "1")
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("KITTY_WINDOW_ID", "")
	m := testModel(t)
	if err := os.WriteFile(filepath.Join(m.h.Workspace.Root, "foo.png"), tinyPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	m.lines = append(m.lines, line{kind: "asst-live", text: "![diagram](foo.png)"})
	m.refresh()
	m.vp.SetYOffset(0)
	if len(m.imgSlots) != 1 || m.imgSlots[0].rows != imgReserveRows {
		t.Fatalf("slots = %#v", m.imgSlots)
	}
	view := m.View()
	plain := stripANSI(view)
	if !strings.Contains(plain, "diagram") {
		t.Fatalf("missing caption:\n%s", plain)
	}
	if !strings.Contains(view, "f=100") {
		t.Fatalf("missing kitty payload:\n%q", view)
	}
}
