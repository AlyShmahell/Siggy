package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"siggy/src/internal/harness"
)

type grepTool struct {
	h *harness.Harness
}

func NewGrep(h *harness.Harness) Tool { return &grepTool{h: h} }

func (t *grepTool) Name() string { return "grep" }
func (t *grepTool) Description() string {
	return "Search workspace text files for a regex pattern. Uses ripgrep when available."
}
func (t *grepTool) Risk() harness.Risk { return harness.RiskRead }
func (t *grepTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"pattern": map[string]any{"type": "string"},
		"path":    map[string]any{"type": "string", "description": "Optional subdirectory or file"},
	}, []string{"pattern"})
}

func (t *grepTool) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}](raw)
	if err != nil {
		return "", err
	}
	root := t.h.Workspace.Root
	if args.Path != "" {
		root, err = t.h.Workspace.Resolve(args.Path)
		if err != nil {
			return "", err
		}
	}
	if _, err := exec.LookPath("rg"); err == nil {
		cmd := exec.CommandContext(ctx, "rg", "-n", "--hidden", "--glob", "!.git", "-m", "80", args.Pattern, root)
		cmd.Dir = t.h.Workspace.Root
		out, err := cmd.CombinedOutput()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
				return "(no matches)", nil
			}
			return "", fmt.Errorf("rg: %s", bytes.TrimSpace(out))
		}
		return string(out), nil
	}
	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return "", err
	}
	var hits []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && (d.Name() == ".git" || d.Name() == "node_modules") {
				return fs.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || !utf8.Valid(data) || len(data) > 1024*1024 {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if re.MatchString(line) {
				hits = append(hits, fmt.Sprintf("%s:%d:%s", t.h.Workspace.Rel(path), i+1, line))
				if len(hits) >= 80 {
					return fs.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return "(no matches)", nil
	}
	return strings.Join(hits, "\n"), nil
}
