package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"siggy/src/internal/harness"
)

type listTool struct {
	h *harness.Harness
}

func NewList(h *harness.Harness) Tool { return &listTool{h: h} }

func (t *listTool) Name() string        { return "dir_list" }
func (t *listTool) Description() string { return "List entries in a workspace directory." }
func (t *listTool) Risk() harness.Risk  { return harness.RiskRead }
func (t *listTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"path": map[string]any{"type": "string", "description": "Directory path, default workspace root"},
	}, nil)
}

func (t *listTool) Run(_ context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		Path string `json:"path"`
	}](raw)
	if err != nil {
		return "", err
	}
	path, err := t.h.Workspace.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range entries {
		kind := "file"
		if e.IsDir() {
			kind = "dir"
		}
		fmt.Fprintf(&b, "%s\t%s\n", kind, e.Name())
	}
	if b.Len() == 0 {
		return "(empty)", nil
	}
	return b.String(), nil
}
