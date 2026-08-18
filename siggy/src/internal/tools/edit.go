package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"siggy/src/internal/harness"
)

type editTool struct {
	h *harness.Harness
}

func NewEdit(h *harness.Harness) Tool { return &editTool{h: h} }

func (t *editTool) Name() string { return "edit_file" }
func (t *editTool) Description() string {
	return "Replace exactly one occurrence of old_string with new_string in a workspace file."
}
func (t *editTool) Risk() harness.Risk { return harness.RiskWrite }
func (t *editTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"path":       map[string]any{"type": "string"},
		"old_string": map[string]any{"type": "string"},
		"new_string": map[string]any{"type": "string"},
	}, []string{"path", "old_string", "new_string"})
}

func (t *editTool) Run(_ context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}](raw)
	if err != nil {
		return "", err
	}
	if args.OldString == "" {
		return "", fmt.Errorf("old_string is required")
	}
	path, err := t.h.Workspace.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	snapshotFile(t.h, args.Path, path)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(data)
	count := strings.Count(text, args.OldString)
	if count == 0 {
		return "", fmt.Errorf("old_string not found in %s", args.Path)
	}
	if count > 1 {
		return "", fmt.Errorf("old_string matches %d times in %s; make it unique", count, args.Path)
	}
	next := strings.Replace(text, args.OldString, args.NewString, 1)
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return "", err
	}
	return formatEditHunk(t.h.Workspace.Rel(path), text, args.OldString, args.NewString), nil
}
