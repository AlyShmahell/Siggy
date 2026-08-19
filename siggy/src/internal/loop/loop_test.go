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

func TestLoopPersistsAPIUsage(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Session.Close()
	reg := tools.Builtins(h, nil)
	fake := &llm.Scripted{Steps: []llm.ScriptedStep{{
		Text:  "hello",
		Usage: llm.Usage{Prompt: 639, Completion: 4983, Total: 5622, Reasoning: 4000},
	}}}
	eng := New(fake, reg, h, "sys")
	var usage Event
	err = eng.Run(context.Background(), "hi", func(e Event) {
		if e.Kind == KindUsage {
			usage = e
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if usage.PromptTokens != 639 || usage.TotalTokens != 5622 || usage.Estimated {
		t.Fatalf("event = %#v", usage)
	}
	if usage.ReasoningTokens != 4000 {
		t.Fatalf("reasoning = %d", usage.ReasoningTokens)
	}
	prompt, billed, est := SumUsage(h.Session.Records())
	if prompt != 639 || billed != 5622 || est {
		t.Fatalf("sum prompt=%d billed=%d est=%v", prompt, billed, est)
	}
	id := h.Session.ID
	h.Session.Close()
	opened, err := harness.OpenSession(home, id)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	prompt, billed, est = SumUsage(opened.Records())
	if prompt != 639 || billed != 5622 || est {
		t.Fatalf("reopen prompt=%d billed=%d est=%v", prompt, billed, est)
	}
}

func TestLoopEstimatesMissingUsage(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Session.Close()
	reg := tools.Builtins(h, nil)
	fake := &llm.Scripted{Steps: []llm.ScriptedStep{{Text: "ok"}}}
	eng := New(fake, reg, h, "sys")
	var usage Event
	if err := eng.Run(context.Background(), "hi", func(e Event) {
		if e.Kind == KindUsage {
			usage = e
		}
	}); err != nil {
		t.Fatal(err)
	}
	if !usage.Estimated || usage.TotalTokens <= 0 {
		t.Fatalf("expected estimate, got %#v", usage)
	}
	_, billed, est := SumUsage(h.Session.Records())
	if !est || billed != usage.TotalTokens {
		t.Fatalf("persisted billed=%d est=%v", billed, est)
	}
}
