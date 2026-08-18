package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"siggy/src/internal/harness"
)

type todoItem struct {
	ID     string `json:"id"`
	Content string `json:"content"`
	Status string `json:"status"`
}

type todoTool struct {
	h     *harness.Harness
	mu    sync.Mutex
	items []todoItem
}

func NewTodo(h *harness.Harness) Tool { return &todoTool{h: h} }

func (t *todoTool) Name() string { return "todo_write" }
func (t *todoTool) Description() string {
	return "Replace the in-session task list. Status is pending, in_progress, or completed."
}
func (t *todoTool) Risk() harness.Risk { return harness.RiskRead }
func (t *todoTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"items": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string"},
					"content": map[string]any{"type": "string"},
					"status":  map[string]any{"type": "string"},
				},
				"required": []string{"id", "content", "status"},
			},
		},
	}, []string{"items"})
}

func (t *todoTool) Run(_ context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		Items []todoItem `json:"items"`
	}](raw)
	if err != nil {
		return "", err
	}
	t.mu.Lock()
	t.items = args.Items
	t.mu.Unlock()
	if t.h != nil && t.h.Session != nil {
		var snap []harness.TodoSnap
		for _, it := range args.Items {
			snap = append(snap, harness.TodoSnap{ID: it.ID, Content: it.Content, Status: it.Status})
		}
		_ = t.h.Session.Append(harness.Record{Type: "todo", Todos: snap})
	}
	var b strings.Builder
	for _, it := range args.Items {
		fmt.Fprintf(&b, "[%s] %s (%s)\n", it.Status, it.Content, it.ID)
	}
	if b.Len() == 0 {
		return "(empty todo list)", nil
	}
	return b.String(), nil
}
