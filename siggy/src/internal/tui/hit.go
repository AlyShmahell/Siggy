package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

type Kind int

const (
	KindNone Kind = iota
	KindSidebarSession
	KindSidebarNew
	KindSidebarProviders
	KindSidebarVersion
	KindTranscript
	KindPrompt
	KindSend
	KindCancel
	KindSlash
	KindComposerMode
	KindComposerModel
	KindStatusMode
	KindStatusProvider
	KindModalItem
	KindModalDismiss
	KindApprove
	KindFormField
	KindFormListItem
	KindFormAdd
	KindFormSave
	KindFormCancel
	KindFormBack
	KindProviderRow
	KindProviderNew
	KindProviderEdit
	KindSidebarDelete
	KindSidebarDeleteAll
	KindFormDeleteModel
	KindNavClock
	KindNavGear
	KindNavQuit
	KindMention
)

type Rect struct {
	X, Y, W, H int
}

func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H && r.W > 0 && r.H > 0
}

type Target struct {
	ID    string
	Kind  Kind
	Index int
	Rect  Rect
}

type HitMap struct {
	targets []Target
}

func (h *HitMap) Clear() {
	if h == nil {
		return
	}
	h.targets = h.targets[:0]
}

func (h *HitMap) Add(t Target) {
	if h == nil || t.Rect.W <= 0 || t.Rect.H <= 0 {
		return
	}
	h.targets = append(h.targets, t)
}

func (h *HitMap) At(x, y int) (Target, bool) {
	if h == nil {
		return Target{}, false
	}
	for i := len(h.targets) - 1; i >= 0; i-- {
		if h.targets[i].Rect.Contains(x, y) {
			return h.targets[i], true
		}
	}
	return Target{}, false
}

func (h *HitMap) OfKind(k Kind) []Target {
	if h == nil {
		return nil
	}
	var out []Target
	for _, t := range h.targets {
		if t.Kind == k {
			out = append(out, t)
		}
	}
	return out
}

// frame is a W×H cell canvas. Blit and AddHit share the same origin so
// painted glyphs and mouse targets stay aligned.
type frame struct {
	w, h int
	rows []string
	hits *HitMap
}

func (f *frame) reset(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	f.w, f.h = w, h
	f.rows = make([]string, h)
	blank := strings.Repeat(" ", w)
	for i := range f.rows {
		f.rows[i] = blank
	}
	if f.hits != nil {
		f.hits.Clear()
	}
}

func (f *frame) blit(x, y int, text string) int {
	if text == "" || f.h == 0 {
		return 0
	}
	parts := strings.Split(text, "\n")
	for i, part := range parts {
		f.put(x, y+i, part)
	}
	return len(parts)
}

func (f *frame) put(x, y int, src string) {
	if y < 0 || y >= f.h || x >= f.w {
		return
	}
	if x < 0 {
		src = cutVis(src, -x, lipgloss.Width(src))
		x = 0
	}
	src = cutVis(src, 0, f.w-x)
	srcW := lipgloss.Width(src)
	left := padVis(cutVis(f.rows[y], 0, x), x)
	right := cutVis(f.rows[y], x+srcW, f.w)
	f.rows[y] = padVis(left+"\x1b[0m"+src+"\x1b[0m"+right, f.w)
}

func (f *frame) addHit(k Kind, index int, r Rect) {
	if f.hits == nil {
		return
	}
	f.hits.Add(Target{Kind: k, Index: index, Rect: r})
}

func (f *frame) String() string {
	out := make([]string, len(f.rows))
	for i, row := range f.rows {
		out[i] = padVis(cutVis(row, 0, f.w), f.w)
	}
	return strings.Join(out, "\n")
}

func padVis(s string, w int) string {
	n := lipgloss.Width(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

func cutVis(s string, left, right int) string {
	if right <= left || s == "" {
		return ""
	}
	var b strings.Builder
	col := 0
	i := 0
	for i < len(s) && col < right {
		if s[i] == 0x1b {
			j := skipESC(s, i)
			if col >= left {
				b.WriteString(s[i:j])
			}
			i = j
			continue
		}
		_, sz := utf8.DecodeRuneInString(s[i:])
		if col >= left {
			b.WriteString(s[i : i+sz])
		}
		col++
		i += sz
	}
	return b.String()
}

func skipESC(s string, i int) int {
	n := len(s)
	if i >= n || s[i] != 0x1b {
		return i
	}
	i++
	if i >= n {
		return i
	}
	switch s[i] {
	case '[':
		i++
		for i < n {
			c := s[i]
			i++
			if c >= 0x40 && c <= 0x7e {
				break
			}
		}
	case ']':
		i++
		for i < n {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == 0x1b && i+1 < n && s[i+1] == '\\' {
				return i + 2
			}
			i++
		}
	default:
		i++
	}
	return i
}

func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b {
			i = skipESC(s, i)
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
