package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"siggy/src/internal/harness"
)

type delegateTool struct {
	d Delegator
}

func NewDelegate(d Delegator) Tool { return &delegateTool{d: d} }

func (t *delegateTool) Name() string { return "delegate" }
func (t *delegateTool) Description() string {
	return "Spawn a subagent (explore, implement, or review) with an isolated transcript and a restricted tool set."
}
func (t *delegateTool) Risk() harness.Risk { return harness.RiskRead }
func (t *delegateTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"agent": map[string]any{"type": "string", "description": "explore | implement | review | project agent name"},
		"task":  map[string]any{"type": "string"},
	}, []string{"agent", "task"})
}

func (t *delegateTool) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		Agent string `json:"agent"`
		Task  string `json:"task"`
	}](raw)
	if err != nil {
		return "", err
	}
	if t.d == nil {
		return "", fmt.Errorf("no subagent runtime configured")
	}
	return t.d.Delegate(ctx, args.Agent, args.Task)
}
