package tui

import (
	"strings"
	"testing"
)

func TestHitMapAtReverseOrder(t *testing.T) {
	var h HitMap
	h.Add(Target{Kind: KindTranscript, Rect: Rect{0, 0, 20, 10}})
	h.Add(Target{Kind: KindModalDismiss, Rect: Rect{0, 0, 20, 10}})
	h.Add(Target{Kind: KindApprove, Index: 1, Rect: Rect{2, 2, 4, 1}})
	got, ok := h.At(3, 2)
	if !ok || got.Kind != KindApprove || got.Index != 1 {
		t.Fatalf("got %#v ok=%v", got, ok)
	}
	got, ok = h.At(0, 0)
	if !ok || got.Kind != KindModalDismiss {
		t.Fatalf("outside item should be dismiss, got %#v", got)
	}
	if _, ok := h.At(50, 50); ok {
		t.Fatal("expected miss")
	}
}

func TestFrameBlitAndHitShareY(t *testing.T) {
	hits := &HitMap{}
	f := &frame{hits: hits}
	f.reset(20, 8)
	y := 0
	y += f.blit(0, y, "title")
	f.addHit(KindSidebarProviders, 0, Rect{0, y, 20, 1})
	y += f.blit(0, y, "Providers")
	f.addHit(KindFormSave, 0, Rect{0, y, 8, 1})
	_ = f.blit(0, y, " save ")

	plain := strings.Split(stripANSI(f.String()), "\n")
	provY := -1
	saveY := -1
	for i, line := range plain {
		if strings.Contains(line, "Providers") {
			provY = i
		}
		if strings.Contains(line, "save") {
			saveY = i
		}
	}
	if provY < 0 || saveY < 0 {
		t.Fatalf("missing glyphs: %q", plain)
	}
	if got, ok := hits.At(1, provY); !ok || got.Kind != KindSidebarProviders {
		t.Fatalf("providers y=%d hit %#v", provY, got)
	}
	if got, ok := hits.At(1, saveY); !ok || got.Kind != KindFormSave {
		t.Fatalf("save y=%d hit %#v", saveY, got)
	}
}

func TestRectContains(t *testing.T) {
	r := Rect{5, 5, 3, 2}
	if !r.Contains(5, 5) || !r.Contains(7, 6) {
		t.Fatal("expected inside")
	}
	if r.Contains(8, 5) || r.Contains(5, 7) || r.Contains(4, 5) {
		t.Fatal("expected outside")
	}
	if (Rect{0, 0, 0, 1}).Contains(0, 0) {
		t.Fatal("zero width")
	}
}
