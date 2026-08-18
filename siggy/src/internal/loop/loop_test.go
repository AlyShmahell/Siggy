package loop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/tools"
)

func TestLoopReadThenDone(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.Builtins(h, nil)
	fake := &llm.Scripted{Steps: []llm.ScriptedStep{
		{Calls: []llm.ToolCall{{ID: "1", Name: "list_dir", Args: json.RawMessage(`{}`)}}},
		{Text: "listed the workspace"},
	}}
	eng := New(fake, reg, h, "sys")
	var evs []Kind
	err = eng.Run(context.Background(), "look around", func(e Event) {
		evs = append(evs, e.Kind)
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, k := range evs {
		joined += string(k) + ","
	}
	if !strings.Contains(joined, string(KindToolStart)) || !strings.Contains(joined, string(KindDone)) {
		t.Fatalf("events = %s", joined)
	}
}

func TestSchedulerRejectsOverlap(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	s := NewScheduler(tools.Builtins(h, nil), h)
	s.busy = true
	_, err = s.Run(context.Background(), nil, func(Event) {})
	if err == nil {
		t.Fatal("expected busy error")
	}
}

func TestPlanBlocksWrite(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	h.Mode = harness.ModePlan
	reg := tools.Builtins(h, nil)
	fake := &llm.Scripted{Steps: []llm.ScriptedStep{
		{Calls: []llm.ToolCall{{ID: "1", Name: "write_file", Args: json.RawMessage(`{"path":"x","content":"y"}`)}}},
		{Text: "blocked"},
	}}
	eng := New(fake, reg, h, "sys")
	if err := eng.Run(context.Background(), "write", func(Event) {}); err != nil {
		t.Fatal(err)
	}
}
