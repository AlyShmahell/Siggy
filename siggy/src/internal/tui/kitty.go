package tui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	imgReserveRows = 12
	maxImageBytes  = 2 * 1024 * 1024
)

type imgSlot struct {
	path        string
	abs         string
	contentLine int
	cols        int
	rows        int
	live        bool
}

type imgCacheEntry struct {
	mod time.Time
	png []byte
	id  int
}

func kittyImagesEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("SIGGY_KITTY_IMAGES")))
	if v != "1" && v != "true" && v != "yes" {
		return false
	}
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return true
	}
	return strings.Contains(strings.ToLower(os.Getenv("TERM")), "kitty")
}

func imageExtOK(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif":
		return true
	default:
		return false
	}
}

func decodeImagePNG(r io.Reader) ([]byte, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func kittyDeleteAll() string {
	return "\x1b_Ga=d,d=a\x1b\\"
}

func kittyTransmitAndPlace(id, cols, rows int, png []byte) string {
	if id < 1 || len(png) == 0 {
		return ""
	}
	b64 := base64.StdEncoding.EncodeToString(png)
	const chunk = 4096
	var b strings.Builder
	for i := 0; i < len(b64); i += chunk {
		end := i + chunk
		if end > len(b64) {
			end = len(b64)
		}
		more := 1
		if end == len(b64) {
			more = 0
		}
		if i == 0 {
			fmt.Fprintf(&b, "\x1b_Gf=100,a=T,t=d,i=%d,c=%d,r=%d,m=%d;%s\x1b\\", id, cols, rows, more, b64[i:end])
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, b64[i:end])
		}
	}
	return b.String()
}

func kittyCUP(x, y int) string {
	return fmt.Sprintf("\x1b[%d;%dH", y+1, x+1)
}

func kittyID(path string) int {
	var h uint32 = 2166136261
	for i := 0; i < len(path); i++ {
		h ^= uint32(path[i])
		h *= 16777619
	}
	n := int(h & 0x7fffffff)
	if n == 0 {
		n = 1
	}
	return n
}

func (m model) resolveImage(rel string) (string, bool) {
	rel = strings.TrimSpace(rel)
	if rel == "" || remoteImagePath(rel) || !imageExtOK(rel) {
		return "", false
	}
	if m.h == nil || m.h.Workspace == nil {
		return "", false
	}
	abs, err := m.h.Workspace.Resolve(rel)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() || info.Size() <= 0 || info.Size() > maxImageBytes {
		return "", false
	}
	return abs, true
}

func (m *model) cachedPNG(abs string) ([]byte, int, bool) {
	if m.imgCache == nil {
		m.imgCache = map[string]imgCacheEntry{}
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, 0, false
	}
	if e, ok := m.imgCache[abs]; ok && e.mod.Equal(info.ModTime()) && len(e.png) > 0 {
		return e.png, e.id, true
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, 0, false
	}
	defer f.Close()
	png, err := decodeImagePNG(io.LimitReader(f, maxImageBytes+1))
	if err != nil {
		return nil, 0, false
	}
	id := kittyID(abs)
	m.imgCache[abs] = imgCacheEntry{mod: info.ModTime(), png: png, id: id}
	return png, id, true
}

func (m *model) kittyOverlay() string {
	if !kittyImagesEnabled() || len(m.imgSlots) == 0 || m.page != pageSession {
		return ""
	}
	tr := m.reg.transcript
	if tr.H < 1 || tr.W < 1 {
		return ""
	}
	off := m.vp.YOffset
	var b strings.Builder
	b.WriteString(kittyDeleteAll())
	placed := false
	for _, s := range m.imgSlots {
		if s.live || s.abs == "" || s.rows < 1 {
			continue
		}
		rel := s.contentLine - off
		if rel < 0 || rel >= tr.H {
			continue
		}
		png, id, ok := m.cachedPNG(s.abs)
		if !ok {
			continue
		}
		rows := s.rows
		if rel+rows > tr.H {
			rows = tr.H - rel
		}
		if rows < 1 {
			continue
		}
		x := tr.X + 1
		y := tr.Y + rel
		cols := s.cols
		if cols < 1 {
			cols = 8
		}
		if x+cols > tr.X+tr.W {
			cols = max(tr.X+tr.W-x, 1)
		}
		b.WriteString(kittyCUP(x, y))
		b.WriteString(kittyTransmitAndPlace(id, cols, rows, png))
		placed = true
	}
	if !placed {
		return kittyDeleteAll()
	}
	return b.String()
}
