package tui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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
	t.Setenv("SIGGY_KITTY_IMAGES", "0")
	t.Setenv("TERM", "dumb")
	t.Setenv("KITTY_WINDOW_ID", "")
	m := testModel(t)
	root := m.h.Workspace.Root
	if err := os.WriteFile(filepath.Join(root, "foo.png"), tinyPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	m.lines = append(m.lines, line{kind: "asst-live", text: "![diagram](foo.png)"})
	m.refresh()
	view := m.View()
	plain := stripANSI(view)
	if strings.Contains(plain, "![") {
		t.Fatalf("markdown leaked:\n%s", plain)
	}
	if strings.Contains(view, "\x1b_G") {
		t.Fatalf("APC leaked:\n%q", view)
	}
	if !strings.Contains(plain, "▀") {
		t.Fatalf("missing half-block preview:\n%s", plain)
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
	if len(m.imgSlots) != 1 || m.imgSlots[0].rows < 1 || m.imgSlots[0].cols < 1 {
		t.Fatalf("slots = %#v", m.imgSlots)
	}
	view := m.View()
	plain := stripANSI(view)
	if strings.Contains(plain, "![") {
		t.Fatalf("markdown leaked:\n%s", plain)
	}
	if !strings.Contains(view, "f=100") {
		t.Fatalf("missing kitty payload:\n%q", view)
	}
}

func TestKittyImageFromInlineCode(t *testing.T) {
	t.Setenv("SIGGY_KITTY_IMAGES", "1")
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("KITTY_WINDOW_ID", "")
	m := testModel(t)
	if err := os.WriteFile(filepath.Join(m.h.Workspace.Root, "foo.png"), tinyPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	m.lines = append(m.lines, line{kind: "asst-live", text: "Here's the duck: `![diagram](foo.png)`"})
	m.refresh()
	m.vp.SetYOffset(0)
	if len(m.imgSlots) != 1 || m.imgSlots[0].rows < 1 {
		t.Fatalf("slots = %#v", m.imgSlots)
	}
	view := m.View()
	plain := stripANSI(view)
	if strings.Contains(plain, "![") {
		t.Fatalf("markdown leaked:\n%s", plain)
	}
	if !strings.Contains(view, "f=100") {
		t.Fatalf("missing kitty payload:\n%q", view)
	}
}

func TestKittyEnabledFromTERM(t *testing.T) {
	t.Setenv("SIGGY_KITTY_IMAGES", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("TERM", "xterm-kitty")
	if !kittyImagesEnabled() {
		t.Fatal("expected kitty graphics from TERM")
	}
}

func TestKittyDisabledByEnv(t *testing.T) {
	t.Setenv("SIGGY_KITTY_IMAGES", "0")
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("KITTY_WINDOW_ID", "1")
	if kittyImagesEnabled() {
		t.Fatal("SIGGY_KITTY_IMAGES=0 should force off")
	}
}

func TestHalfBlockRendersColor(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, A: 255})
	got := halfBlock(img, 2, 1)
	if !strings.Contains(got, "▀") {
		t.Fatalf("missing block: %q", got)
	}
	if !strings.Contains(got, "38;2;") {
		t.Fatalf("missing truecolor: %q", got)
	}
	if pixelHex(color.RGBA{R: 255, A: 255}) != "#ff0000" {
		t.Fatalf("pixelHex = %q", pixelHex(color.RGBA{R: 255, A: 255}))
	}
}

func TestHalfBlockFitCapsToViewport(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 200))
	_, cols, rows := halfBlockFit(img, 80, 10)
	if rows != 10 {
		t.Fatalf("rows = %d want 10", rows)
	}
	if cols < 1 || cols > 80 {
		t.Fatalf("cols = %d", cols)
	}
}

func TestHalfBlockFitLandscapeAspect(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	img := image.NewRGBA(image.Rect(0, 0, 100, 50))
	got, cols, rows := halfBlockFit(img, 40, 40)
	if cols != 40 {
		t.Fatalf("cols = %d", cols)
	}
	if rows != 10 {
		t.Fatalf("rows = %d want 10", rows)
	}
	if !strings.Contains(got, "▀") {
		t.Fatalf("missing block: %q", got)
	}
}

func TestHalfBlockResampleMixesNeighbors(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{B: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	got := halfBlock(img, 8, 4)
	fgs := regexp.MustCompile(`38;2;\d+;\d+;\d+`).FindAllString(got, -1)
	uniq := map[string]struct{}{}
	for _, fg := range fgs {
		uniq[fg] = struct{}{}
	}
	if len(uniq) < 3 {
		t.Fatalf("resample looks like a nearest-neighbor stamp: %d unique fgs in %q", len(uniq), got)
	}
}

func TestRenderTableImageCellOnDisk(t *testing.T) {
	t.Setenv("SIGGY_KITTY_IMAGES", "0")
	t.Setenv("TERM", "dumb")
	t.Setenv("KITTY_WINDOW_ID", "")
	m := testModel(t)
	if err := os.WriteFile(filepath.Join(m.h.Workspace.Root, "foo.png"), tinyPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	m.lines = append(m.lines, line{kind: "asst-live", text: "| Name | Photo |\n|------|-------|\n| Duck | ![diagram](foo.png) |\n"})
	m.refresh()
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "Duck") {
		t.Fatalf("missing text cell:\n%s", plain)
	}
	if !strings.Contains(plain, "│") {
		t.Fatalf("missing table:\n%s", plain)
	}
	if strings.Contains(plain, "![") {
		t.Fatalf("markdown leaked:\n%s", plain)
	}
	if strings.Count(plain, "▀") < 2 {
		t.Fatalf("missing multi-line preview:\n%s", plain)
	}
}

func TestKittyTableImageHasColOffset(t *testing.T) {
	t.Setenv("SIGGY_KITTY_IMAGES", "1")
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("KITTY_WINDOW_ID", "")
	m := testModel(t)
	if err := os.WriteFile(filepath.Join(m.h.Workspace.Root, "foo.png"), tinyPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	m.lines = append(m.lines, line{kind: "asst-live", text: "| Name | Photo |\n|------|-------|\n| Duck | ![diagram](foo.png) |\n"})
	m.refresh()
	m.vp.SetYOffset(0)
	if len(m.imgSlots) != 1 {
		t.Fatalf("slots = %#v", m.imgSlots)
	}
	if m.imgSlots[0].col < 1 {
		t.Fatalf("expected image in second column, col=%d", m.imgSlots[0].col)
	}
	view := m.View()
	if !strings.Contains(view, "f=100") {
		t.Fatalf("missing kitty payload:\n%q", view)
	}
}
