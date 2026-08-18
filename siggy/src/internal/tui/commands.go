package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"siggy/src/internal/harness"
	"siggy/src/internal/loop"
)

func (m *model) runCompact(fast bool) {
	if m.g == nil || m.g.Engine == nil {
		return
	}
	m.g.Engine.CompactNow(context.Background(), func(loop.Event) {}, fast)
	m.loadTranscript("compressed")
}

func (m *model) exportSession(format string) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "jsonl"
	}
	if m.h == nil || m.h.Session == nil || m.h.Workspace == nil {
		m.lines = append(m.lines, line{kind: "err", text: "no session"})
		return
	}
	recs := m.h.Session.Records()
	var body []byte
	ext := format
	switch format {
	case "md", "markdown":
		body = []byte(harness.ExportMarkdown(recs))
		ext = "md"
	default:
		raw, err := harness.ExportJSONL(recs)
		if err != nil {
			m.lines = append(m.lines, line{kind: "err", text: err.Error()})
			return
		}
		body = raw
		ext = "jsonl"
	}
	name := "siggy-export-" + m.h.Session.ID + "." + ext
	path := filepath.Join(m.h.Workspace.Root, name)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		m.lines = append(m.lines, line{kind: "err", text: err.Error()})
		return
	}
	m.lines = append(m.lines, line{kind: "sys", text: "exported " + name})
}

func (m *model) checkpoints() []harness.Record {
	if m.h == nil || m.h.Session == nil {
		return nil
	}
	var out []harness.Record
	for _, r := range m.h.Session.Records() {
		if r.Type == "checkpoint" && r.Path != "" {
			out = append(out, r)
		}
	}
	return out
}

func (m *model) restoreCheckpointAt(i int) {
	cps := m.checkpoints()
	if i < 0 || i >= len(cps) {
		return
	}
	if err := harness.RestoreCheckpoint(m.h.Workspace, cps[i]); err != nil {
		m.lines = append(m.lines, line{kind: "err", text: err.Error()})
		return
	}
	m.lines = append(m.lines, line{kind: "sys", text: "restored " + cps[i].Path})
}

func (m *model) branchSession() {
	if m.h == nil || m.h.Session == nil {
		return
	}
	child, err := harness.BranchSession(m.home, m.h.Session.ID)
	if err != nil {
		m.lines = append(m.lines, line{kind: "err", text: err.Error()})
		return
	}
	_ = m.h.Session.Close()
	m.h.Session = child
	m.g.Resume(child.Records())
	m.loadTranscript("branched " + child.ID)
	m.reloadSessions()
}

func (m *model) rewindSession(arg string) {
	if m.h == nil || m.h.Session == nil {
		return
	}
	recs := m.h.Session.Records()
	through := 0
	if arg != "" {
		n, err := strconv.Atoi(arg)
		if err != nil {
			m.lines = append(m.lines, line{kind: "err", text: "rewind needs a seq"})
			return
		}
		through = n
	} else {
		through = loop.LastUserSeq(recs)
		if through > 0 {
			through--
		}
	}
	rec := loop.RewindRecords(recs, through)
	if rec.To <= rec.From {
		m.lines = append(m.lines, line{kind: "sys", text: "nothing to rewind"})
		return
	}
	_ = m.h.Session.Append(rec)
	m.g.Resume(m.h.Session.Records())
	m.loadTranscript(rec.Text)
}

func (m *model) rememberFact(fact string) {
	if strings.TrimSpace(fact) == "" {
		m.lines = append(m.lines, line{kind: "err", text: "usage: /remember fact"})
		return
	}
	dir := m.memoryDir()
	_ = os.MkdirAll(dir, 0o755)
	name := strings.Map(func(r rune) rune {
		if r == ' ' {
			return '-'
		}
		return r
	}, fact)
	if len(name) > 40 {
		name = name[:40]
	}
	path := filepath.Join(dir, name+".md")
	_ = os.WriteFile(path, []byte(fact+"\n"), 0o644)
	m.rebuildMemoryIndex(dir)
	m.lines = append(m.lines, line{kind: "sys", text: "remembered " + filepath.Base(path)})
}

func (m *model) forgetMemory(q string) {
	if q == "" {
		m.lines = append(m.lines, line{kind: "err", text: "usage: /forget query"})
		return
	}
	dir := m.memoryDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.lines = append(m.lines, line{kind: "sys", text: "no memories"})
		return
	}
	n := 0
	ql := strings.ToLower(q)
	for _, e := range entries {
		if e.IsDir() || e.Name() == "MEMORY.md" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		body, _ := os.ReadFile(p)
		if strings.Contains(strings.ToLower(e.Name()), ql) || strings.Contains(strings.ToLower(string(body)), ql) {
			if os.Remove(p) == nil {
				n++
			}
		}
	}
	m.rebuildMemoryIndex(dir)
	m.lines = append(m.lines, line{kind: "sys", text: fmt.Sprintf("forgot %d notes", n)})
}

func (m *model) showMemory() {
	path := filepath.Join(m.memoryDir(), "MEMORY.md")
	body, err := os.ReadFile(path)
	if err != nil || len(body) == 0 {
		m.lines = append(m.lines, line{kind: "sys", text: "(empty memory index)"})
		return
	}
	m.lines = append(m.lines, line{kind: "sys", text: string(body)})
}

func (m *model) dreamMemory() {
	if m.h == nil || m.h.Session == nil {
		return
	}
	var last string
	for _, r := range m.h.Session.Records() {
		if r.Type == "compact" && r.Text != "" {
			last = r.Text
		}
	}
	if last == "" {
		m.lines = append(m.lines, line{kind: "sys", text: "no compact summaries to dream from"})
		return
	}
	dir := m.memoryDir()
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "last-compact.md"), []byte(last+"\n"), 0o644)
	m.rebuildMemoryIndex(dir)
	m.lines = append(m.lines, line{kind: "sys", text: "dreamed last compact into memory"})
}

func (m *model) memoryDir() string {
	hash := "default"
	if m.h != nil && m.h.Workspace != nil {
		hash = harness.HashWorkspace(m.h.Workspace.Root)
	}
	return harness.MemoryDir(m.home, hash)
}

func (m *model) rebuildMemoryIndex(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var b strings.Builder
	b.WriteString("# Memory index\n\n")
	for _, e := range entries {
		if e.IsDir() || e.Name() == "MEMORY.md" || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		first := strings.TrimSpace(strings.SplitN(string(raw), "\n", 2)[0])
		fmt.Fprintf(&b, "- %s — %s\n", e.Name(), first)
	}
	_ = os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(b.String()), 0o644)
}
