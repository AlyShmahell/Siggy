package subagent

import (
	"context"
	"fmt"
	"strings"

	"siggy/src/internal/graph"
	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/loop"
	"siggy/src/internal/tools"
)

const maxDepth = 2

type Manager struct {
	Parent  *harness.Harness
	Client  llm.Client
	Tools   *tools.Registry
}

func (m *Manager) Delegate(ctx context.Context, agent, task string) (string, error) {
	if m.Parent.Depth >= maxDepth {
		return "", fmt.Errorf("subagent depth cap %d reached", maxDepth)
	}
	spec, err := Resolve(m.Parent.Workspace.Root, m.Parent.Home, agent)
	if err != nil {
		return "", err
	}
	childSess, err := harness.NewSessionMeta(m.Parent.Home, harness.SessionMeta{
		CWD:           m.Parent.Workspace.Root,
		WorkspaceHash: harness.HashWorkspace(m.Parent.Workspace.Root),
		Depth:         m.Parent.Depth + 1,
		Origin:        "subagent",
		ParentID:      m.Parent.Session.ID,
	})
	if err != nil {
		return "", err
	}
	child := &harness.Harness{
		Workspace: m.Parent.Workspace,
		Approvals: m.Parent.Approvals,
		Session:   childSess,
		Mode:      m.Parent.Mode,
		Loops:     harness.NewLoopDetect(3),
		Home:      m.Parent.Home,
		Depth:     m.Parent.Depth + 1,
	}
	reg := m.Tools.Filter(spec.Tools)
	g := graph.New(m.Client, reg, child, spec.Prompt)
	var text strings.Builder
	err = g.Run(ctx, task, func(ev loop.Event) {
		if ev.Kind == loop.KindText {
			text.WriteString(ev.Text)
		}
	})
	_ = childSess.Close()
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(text.String())
	if out == "" {
		out = "(subagent finished with no text)"
	}
	return fmt.Sprintf("[%s] %s", spec.Name, out), nil
}
