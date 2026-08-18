package tools

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"siggy/src/internal/harness"
)

type globTool struct {
	h *harness.Harness
}

func NewGlob(h *harness.Harness) Tool { return &globTool{h: h} }

func (t *globTool) Name() string { return "glob" }
func (t *globTool) Description() string {
	return "Find workspace files matching a glob pattern (e.g. **/*.go). Optional path limits the search to a directory."
}
func (t *globTool) Risk() harness.Risk { return harness.RiskRead }
func (t *globTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"pattern": map[string]any{"type": "string", "description": "Glob relative to path or workspace, e.g. **/*.go"},
		"path":    map[string]any{"type": "string", "description": "Directory to search, default workspace root"},
	}, []string{"pattern"})
}

func (t *globTool) Run(_ context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}](raw)
	if err != nil {
		return "", err
	}
	pattern := args.Pattern
	if pattern == "" {
		return "", nil
	}
	start, err := t.h.Workspace.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(start); err == nil && !info.IsDir() {
		start = filepath.Dir(start)
	}
	var matches []string
	err = filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := t.h.Workspace.Rel(path)
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" {
				return fs.SkipDir
			}
			return nil
		}
		ok, merr := filepath.Match(filepath.ToSlash(pattern), filepath.ToSlash(rel))
		if merr != nil {
			return merr
		}
		if !ok {
			ok, _ = filepath.Match(pattern, filepath.Base(rel))
		}
		if ok || matchStarStar(pattern, rel) {
			matches = append(matches, rel)
		}
		if len(matches) >= 200 {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "(no matches)", nil
	}
	return strings.Join(matches, "\n"), nil
}

func matchStarStar(pattern, rel string) bool {
	p := filepath.ToSlash(pattern)
	r := filepath.ToSlash(rel)
	if !strings.Contains(p, "**") {
		return false
	}
	parts := strings.Split(p, "**")
	if len(parts) != 2 {
		return false
	}
	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")
	if prefix != "" && !strings.HasPrefix(r, prefix) {
		return false
	}
	if suffix == "" {
		return true
	}
	ok, _ := filepath.Match(suffix, filepath.Base(r))
	if ok {
		return true
	}
	ok, _ = filepath.Match(suffix, r)
	return ok
}
