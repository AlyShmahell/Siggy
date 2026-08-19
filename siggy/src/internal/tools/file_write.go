package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"siggy/src/internal/harness"
	"siggy/src/internal/tools/utils"
)

type writeTool struct {
	h *harness.Harness
}

func NewWrite(h *harness.Harness) Tool { return &writeTool{h: h} }

func (t *writeTool) Name() string { return "file_write" }
func (t *writeTool) Description() string {
	return "Create or overwrite a text file in the workspace."
}
func (t *writeTool) Risk() harness.Risk { return harness.RiskWrite }
func (t *writeTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"path":    map[string]any{"type": "string"},
		"content": map[string]any{"type": "string"},
	}, []string{"path", "content"})
}

func (t *writeTool) Run(_ context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}](raw)
	if err != nil {
		return "", err
	}
	path, err := t.h.Workspace.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	utils.SnapshotFile(t.h, args.Path, path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(args.Content), 0o644); err != nil {
		return "", err
	}
	return formatWriteHunk(t.h.Workspace.Rel(path), args.Content), nil
}

func formatWriteHunk(rel, content string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "wrote %s\n", rel)
	if content == "" {
		return strings.TrimRight(b.String(), "\n")
	}
	for i, ln := range strings.Split(content, "\n") {
		fmt.Fprintf(&b, "+ %4d | %s\n", i+1, ln)
	}
	return strings.TrimRight(b.String(), "\n")
}
