package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"siggy/src/internal/config"
	"siggy/src/internal/harness"
	"siggy/src/internal/tools"
)

type mcpTool struct {
	client *Client
	info   ToolInfo
	name   string
}

func (t *mcpTool) Name() string        { return t.name }
func (t *mcpTool) Description() string { return t.info.Description }
func (t *mcpTool) Risk() harness.Risk  { return harness.RiskNetwork }
func (t *mcpTool) Schema() json.RawMessage {
	if len(t.info.InputSchema) > 0 {
		return t.info.InputSchema
	}
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (t *mcpTool) Run(ctx context.Context, args json.RawMessage) (string, error) {
	return t.client.Call(ctx, t.info.Name, args)
}

func Register(ctx context.Context, servers []config.MCPServer, reg *tools.Registry) ([]*Client, error) {
	var clients []*Client
	for _, s := range servers {
		if s.Name == "" || s.Command == "" {
			continue
		}
		c, err := Start(ctx, s.Name, s.Command, s.Args, s.Env)
		if err != nil {
			return clients, fmt.Errorf("mcp %s: %w", s.Name, err)
		}
		listed, err := c.ListTools(ctx)
		if err != nil {
			_ = c.Close()
			return clients, fmt.Errorf("mcp %s list: %w", s.Name, err)
		}
		for _, info := range listed {
			safe := sanitize(s.Name) + "_" + sanitize(info.Name)
			reg.Register(&mcpTool{client: c, info: info, name: "mcp_" + safe})
		}
		clients = append(clients, c)
	}
	return clients, nil
}

func sanitize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
