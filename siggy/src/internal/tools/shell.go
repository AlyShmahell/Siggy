package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"siggy/src/internal/harness"
)

const (
	shellTimeout = 60 * time.Second
	shellMaxOut  = 64 * 1024
)

type shellTool struct {
	h *harness.Harness
}

func NewShell(h *harness.Harness) Tool { return &shellTool{h: h} }

func (t *shellTool) Name() string { return "shell" }
func (t *shellTool) Description() string {
	return "Run a bash command in the workspace. Output is capped. Do not use for interactive programs."
}
func (t *shellTool) Risk() harness.Risk { return harness.RiskShell }
func (t *shellTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"command": map[string]any{"type": "string"},
	}, []string{"command"})
}

func (t *shellTool) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		Command string `json:"command"`
	}](raw)
	if err != nil {
		return "", err
	}
	if args.Command == "" {
		return "", fmt.Errorf("command is required")
	}
	ctx, cancel := context.WithTimeout(ctx, shellTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-lc", args.Command)
	cmd.Dir = t.h.Workspace.Root
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TERM=" + os.Getenv("TERM"),
		"LANG=" + first(os.Getenv("LANG"), "C.UTF-8"),
	}
	var buf bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &buf, n: shellMaxOut}
	cmd.Stderr = &limitedWriter{w: &buf, n: shellMaxOut}
	err = cmd.Run()
	out := buf.String()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("command timed out after %s", shellTimeout)
	}
	if err != nil {
		return out, fmt.Errorf("%w\n%s", err, out)
	}
	if out == "" {
		return "(exit 0, no output)", nil
	}
	return out, nil
}

type limitedWriter struct {
	w *bytes.Buffer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	remain := l.n - l.w.Len()
	if remain <= 0 {
		return len(p), nil
	}
	if len(p) > remain {
		l.w.Write(p[:remain])
		l.w.WriteString("\n...[truncated]...")
		return len(p), nil
	}
	return l.w.Write(p)
}

func first(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
