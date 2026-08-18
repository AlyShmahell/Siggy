package graph

import (
	"context"
	"testing"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/loop"
	"siggy/src/internal/tools"
)

func TestGraphTextTurn(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.Builtins(h, nil)
	g := New(&llm.Scripted{Steps: []llm.ScriptedStep{{Text: "hello"}}}, reg, h, "")
	var nodes []string
	if err := g.Run(context.Background(), "hi", func(e loop.Event) {
		if e.Kind == loop.KindNode {
			nodes = append(nodes, e.Node)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if g.Node != NodeDone {
		t.Fatalf("node = %s", g.Node)
	}
	if len(nodes) < 2 {
		t.Fatalf("nodes = %v", nodes)
	}
}

func TestSetMode(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	g := New(&llm.Scripted{}, tools.Builtins(h, nil), h, "")
	g.SetMode(harness.ModePlan)
	if h.Mode != harness.ModePlan {
		t.Fatal(h.Mode)
	}
}
