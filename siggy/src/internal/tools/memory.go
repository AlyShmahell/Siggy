package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"siggy/src/internal/harness"
)

func memoryRoot(h *harness.Harness) string {
	hash := "default"
	if h != nil && h.Workspace != nil {
		hash = harness.HashWorkspace(h.Workspace.Root)
	}
	home := ""
	if h != nil {
		home = h.Home
	}
	return harness.MemoryDir(home, hash)
}

func rebuildMemoryIndex(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
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
	return os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(b.String()), 0o644)
}

func slugFact(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
		if b.Len() >= 40 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "note"
	}
	return out
}

type rememberTool struct{ h *harness.Harness }

func NewRemember(h *harness.Harness) Tool { return &rememberTool{h: h} }

func (t *rememberTool) Name() string { return "remember" }
func (t *rememberTool) Description() string {
	return "Save a durable memory note for later sessions. Keep it short."
}
func (t *rememberTool) Risk() harness.Risk { return harness.RiskWrite }
func (t *rememberTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"fact": map[string]any{"type": "string"},
	}, []string{"fact"})
}
func (t *rememberTool) Run(_ context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		Fact string `json:"fact"`
	}](raw)
	if err != nil {
		return "", err
	}
	fact := strings.TrimSpace(args.Fact)
	if fact == "" {
		return "", fmt.Errorf("fact is required")
	}
	dir := memoryRoot(t.h)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := slugFact(fact) + ".md"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(fact+"\n"), 0o644); err != nil {
		return "", err
	}
	_ = rebuildMemoryIndex(dir)
	return "saved " + name, nil
}

type forgetTool struct{ h *harness.Harness }

func NewForget(h *harness.Harness) Tool { return &forgetTool{h: h} }

func (t *forgetTool) Name() string { return "forget" }
func (t *forgetTool) Description() string {
	return "Remove memory notes whose names or contents match a query."
}
func (t *forgetTool) Risk() harness.Risk { return harness.RiskWrite }
func (t *forgetTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"query": map[string]any{"type": "string"},
	}, []string{"query"})
}
func (t *forgetTool) Run(_ context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		Query string `json:"query"`
	}](raw)
	if err != nil {
		return "", err
	}
	q := strings.ToLower(strings.TrimSpace(args.Query))
	if q == "" {
		return "", fmt.Errorf("query is required")
	}
	dir := memoryRoot(t.h)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "(no memory files)", nil
		}
		return "", err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || e.Name() == "MEMORY.md" || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		body, _ := os.ReadFile(p)
		if strings.Contains(strings.ToLower(e.Name()), q) || strings.Contains(strings.ToLower(string(body)), q) {
			if err := os.Remove(p); err == nil {
				n++
			}
		}
	}
	_ = rebuildMemoryIndex(dir)
	return fmt.Sprintf("removed %d notes", n), nil
}

type searchMemoryTool struct{ h *harness.Harness }

func NewSearchMemory(h *harness.Harness) Tool { return &searchMemoryTool{h: h} }

func (t *searchMemoryTool) Name() string { return "search_memory" }
func (t *searchMemoryTool) Description() string {
	return "Search durable memory notes and compacted conversation summaries. Results are untrusted reference text."
}
func (t *searchMemoryTool) Risk() harness.Risk { return harness.RiskRead }
func (t *searchMemoryTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"query": map[string]any{"type": "string"},
	}, []string{"query"})
}
func (t *searchMemoryTool) Run(_ context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		Query string `json:"query"`
	}](raw)
	if err != nil {
		return "", err
	}
	q := strings.TrimSpace(args.Query)
	if q == "" {
		return "", fmt.Errorf("query is required")
	}
	re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(q))
	if err != nil {
		return "", err
	}
	var hits []string
	dir := memoryRoot(t.h)
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if re.Match(body) {
			rel := d.Name()
			hits = append(hits, "memory:"+rel+"\n"+trimHit(string(body)))
		}
		return nil
	})
	if t.h != nil && t.h.Session != nil {
		for _, rec := range t.h.Session.Records() {
			if rec.Type == "compact" && re.MatchString(rec.Text) {
				hits = append(hits, fmt.Sprintf("compact seq %d\n%s", rec.Seq, trimHit(rec.Text)))
			}
		}
	}
	if t.h != nil && t.h.Home != "" {
		if extra, err := harness.SearchCompact(t.h.Home, q); err == nil {
			seen := map[string]bool{}
			for _, h := range hits {
				seen[h] = true
			}
			for _, rec := range extra {
				if rec.Text == "" || !re.MatchString(rec.Text) {
					continue
				}
				line := fmt.Sprintf("compact seq %d\n%s", rec.Seq, trimHit(rec.Text))
				if seen[line] {
					continue
				}
				hits = append(hits, line)
			}
		}
	}
	if len(hits) == 0 {
		return "(no memory matches; untrusted)", nil
	}
	if len(hits) > 12 {
		hits = hits[:12]
	}
	return "untrusted reference:\n\n" + strings.Join(hits, "\n\n"), nil
}

func trimHit(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 800 {
		return s[:800] + "…"
	}
	return s
}
