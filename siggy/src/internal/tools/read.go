package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"unicode/utf8"

	"siggy/src/internal/harness"
)

type readTool struct {
	h *harness.Harness
}

func NewRead(h *harness.Harness) Tool { return &readTool{h: h} }

func (t *readTool) Name() string { return "read_file" }
func (t *readTool) Description() string {
	return "Read a UTF-8 text file from the workspace. Use offset/limit for large files (1-based lines)."
}
func (t *readTool) Risk() harness.Risk { return harness.RiskRead }
func (t *readTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"path":   map[string]any{"type": "string", "description": "Workspace-relative path"},
		"offset": map[string]any{"type": "integer", "description": "1-based start line"},
		"limit":  map[string]any{"type": "integer", "description": "Max lines to return"},
	}, []string{"path"})
}

func (t *readTool) Run(_ context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}](raw)
	if err != nil {
		return "", err
	}
	path, err := t.h.Workspace.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%s is not valid UTF-8", args.Path)
	}
	if len(data) > 512*1024 && args.Limit == 0 {
		return "", fmt.Errorf("%s is %d bytes; pass offset/limit", args.Path, len(data))
	}
	text := string(data)
	if args.Offset > 0 || args.Limit > 0 {
		text = sliceLines(text, args.Offset, args.Limit)
	}
	return text, nil
}

func sliceLines(text string, offset, limit int) string {
	lines := splitKeep(text)
	start := 0
	if offset > 0 {
		start = offset - 1
	}
	if start > len(lines) {
		return ""
	}
	end := len(lines)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	out := ""
	for i := start; i < end; i++ {
		out += fmt.Sprintf("%6d|%s\n", i+1, lines[i])
	}
	return out
}

func splitKeep(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start <= len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
