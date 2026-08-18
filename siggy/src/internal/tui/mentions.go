package tui

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"siggy/src/internal/harness"
)

func mentionQuery(s string) (q string, start int, ok bool) {
	i := strings.LastIndex(s, "@")
	if i < 0 {
		return "", -1, false
	}
	rest := s[i+1:]
	if strings.ContainsAny(rest, " \n\t") {
		return "", -1, false
	}
	return rest, i, true
}

func (m *model) syncMentions() {
	q, _, ok := mentionQuery(m.ta.Value())
	if !ok {
		if m.float == floatMentions {
			m.closeFloat()
		}
		m.mentions = nil
		return
	}
	m.mentions = listMentionFiles(m.h, q)
	if m.float != floatMentions {
		m.openFloat(floatMentions)
	}
	if len(m.mentions) == 0 {
		m.palIdx = 0
		return
	}
	m.palIdx = clamp(m.palIdx, 0, len(m.mentions)-1)
}

func listMentionFiles(h *harness.Harness, q string) []string {
	if h == nil || h.Workspace == nil {
		return nil
	}
	root := h.Workspace.Root
	q = strings.ToLower(q)
	var out []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || len(out) >= 80 {
			if len(out) >= 80 {
				return fs.SkipAll
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if q == "" || strings.Contains(strings.ToLower(rel), q) {
			out = append(out, rel)
		}
		return nil
	})
	return out
}

func (m *model) insertMention(path string) {
	v := m.ta.Value()
	_, start, ok := mentionQuery(v)
	if !ok {
		return
	}
	m.ta.SetValue(v[:start] + "@" + path + " ")
	m.selCaret = utf8.RuneCountInString(m.ta.Value())
	m.selAnchor = m.selCaret
	m.closeFloat()
	m.mentions = nil
}

func mentionPaths(text string) []string {
	var out []string
	for _, f := range strings.Fields(text) {
		if strings.HasPrefix(f, "@") && len(f) > 1 {
			out = append(out, strings.TrimPrefix(f, "@"))
		}
	}
	return out
}

func expandMentions(h *harness.Harness, text string) string {
	if h == nil || h.Workspace == nil {
		return text
	}
	var b strings.Builder
	b.WriteString(text)
	for _, rel := range mentionPaths(text) {
		resolved, err := h.Workspace.Resolve(rel)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			continue
		}
		if len(data) >= 4 && string(data[:4]) == "%PDF" {
			continue
		}
		if !utf8.Valid(data) {
			continue
		}
		if len(data) > 32*1024 {
			data = data[:32*1024]
		}
		b.WriteString("\n\n<file path=\"")
		b.WriteString(rel)
		b.WriteString("\">\n")
		b.Write(data)
		b.WriteString("\n</file>")
	}
	return b.String()
}
