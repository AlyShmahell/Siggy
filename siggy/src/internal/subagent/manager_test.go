package subagent

import (
	"context"
	"encoding/json"
	"testing"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/tools"
)

func TestDelegateExplore(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.Builtins(h, nil)
	fake := &llm.Scripted{Steps: []llm.ScriptedStep{
		{Calls: []llm.ToolCall{{ID: "1", Name: "dir_list", Args: json.RawMessage(`{}`)}}},
		{Text: "empty workspace"},
	}}
	mgr := &Manager{Parent: h, Client: fake, Tools: reg}
	out, err := mgr.Delegate(context.Background(), "explore", "map it")
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty result")
	}
}

func TestUnknownAgent(t *testing.T) {
	_, err := Resolve(t.TempDir(), t.TempDir(), "nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDepthCap(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	h.Depth = 2
	mgr := &Manager{Parent: h, Client: &llm.Scripted{}, Tools: tools.Builtins(h, nil)}
	if _, err := mgr.Delegate(context.Background(), "explore", "x"); err == nil {
		t.Fatal("expected depth cap")
	}
}
