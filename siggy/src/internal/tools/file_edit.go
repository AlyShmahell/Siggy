package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"siggy/src/internal/harness"
	"siggy/src/internal/tools/utils"
)

type editTool struct {
	h *harness.Harness
}

func NewEdit(h *harness.Harness) Tool { return &editTool{h: h} }

func (t *editTool) Name() string { return "file_edit" }
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
	utils.SnapshotFile(t.h, args.Path, path)
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

func formatEditHunk(rel, before, old, new string) string {
	idx := strings.Index(before, old)
	if idx < 0 {
		return "edited " + rel
	}
	lineStart := strings.LastIndex(before[:idx], "\n") + 1
	end := idx + len(old)
	lineEnd := strings.Index(before[end:], "\n")
	if lineEnd < 0 {
		lineEnd = len(before)
	} else {
		lineEnd += end
	}
	prefix := before[lineStart:idx]
	suffix := before[end:lineEnd]
	oldBlock := prefix + old + suffix
	newBlock := prefix + new + suffix
	startLine := strings.Count(before[:lineStart], "\n") + 1

	var b strings.Builder
	fmt.Fprintf(&b, "edited %s\n", rel)
	lines := strings.Split(before, "\n")
	if startLine > 1 {
		fmt.Fprintf(&b, "  %4d | %s\n", startLine-1, lines[startLine-2])
	}
	oldLs := strings.Split(oldBlock, "\n")
	newLs := strings.Split(newBlock, "\n")
	for i, ln := range oldLs {
		fmt.Fprintf(&b, "- %4d | %s\n", startLine+i, ln)
	}
	for i, ln := range newLs {
		fmt.Fprintf(&b, "+ %4d | %s\n", startLine+i, ln)
	}
	afterN := startLine + len(oldLs)
	if afterN >= 1 && afterN <= len(lines) {
		fmt.Fprintf(&b, "  %4d | %s\n", afterN, lines[afterN-1])
	}
	return strings.TrimRight(b.String(), "\n")
}
