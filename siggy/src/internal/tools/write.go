package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"siggy/src/internal/harness"
)

type writeTool struct {
	h *harness.Harness
}

func NewWrite(h *harness.Harness) Tool { return &writeTool{h: h} }

func (t *writeTool) Name() string { return "write_file" }
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(args.Content), 0o644); err != nil {
		return "", err
	}
	return "wrote " + t.h.Workspace.Rel(path), nil
}
