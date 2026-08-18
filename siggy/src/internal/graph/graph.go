package graph

import (
	"context"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/loop"
	"siggy/src/internal/prompt"
	"siggy/src/internal/tools"
)

type Node string

const (
	NodeThink    Node = "Think"
	NodeSchedule Node = "ScheduleTools"
	NodeApprove  Node = "AwaitApproval"
	NodeExecute  Node = "Execute"
	NodeSpawn    Node = "Spawn"
	NodeCompact  Node = "Compact"
	NodeDone     Node = "Done"
)

type Graph struct {
	Engine *loop.Engine
	Node   Node
	Mode   harness.Mode
}

func New(client llm.Client, reg *tools.Registry, h *harness.Harness, extra string) *Graph {
	sys := prompt.System(h, reg, extra)
	return &Graph{
		Engine: loop.New(client, reg, h, sys),
		Node:   NodeThink,
		Mode:   h.Mode,
	}
}

func FromSession(client llm.Client, reg *tools.Registry, h *harness.Harness) *Graph {
	g := &Graph{
		Engine: &loop.Engine{LLM: client, Tools: reg, Harness: h, ContextWindow: 128000},
		Node:   NodeThink,
		Mode:   h.Mode,
	}
	g.Resume(h.Session.Records())
	return g
}

func (g *Graph) SetMode(m harness.Mode) {
	g.Mode = m
	g.Engine.Harness.Mode = m
}

func (g *Graph) Reseed() {
	if g == nil || g.Engine == nil || g.Engine.Harness == nil {
		return
	}
	sys := prompt.System(g.Engine.Harness, g.Engine.Tools, "")
	g.Engine.Messages = []llm.Message{{Role: llm.RoleSystem, Content: sys}}
}

func (g *Graph) Resume(records []harness.Record) {
	g.Engine.Restore(records)
}

func (g *Graph) Run(ctx context.Context, user string, emit func(loop.Event)) error {
	if emit == nil {
		emit = func(loop.Event) {}
	}
	emit(loop.Event{Kind: loop.KindMode, Mode: string(g.Mode)})
	wrapped := func(ev loop.Event) {
		if ev.Kind == loop.KindNode {
			g.Node = Node(ev.Node)
		}
		if ev.Kind == loop.KindApproval {
			g.Node = NodeApprove
			ev.Node = string(NodeApprove)
		}
		if ev.Kind == loop.KindToolStart {
			g.Node = NodeExecute
			if ev.Tool == "delegate" {
				g.Node = NodeSpawn
				ev.Node = string(NodeSpawn)
			}
		}
		emit(ev)
	}
	return g.Engine.Run(ctx, user, wrapped)
}
