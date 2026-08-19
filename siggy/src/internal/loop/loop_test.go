package loop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
		{Calls: []llm.ToolCall{{ID: "1", Name: "dir_list", Args: json.RawMessage(`{}`)}}},
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

func TestLoopAliasReadPDF(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.Builtins(h, nil)
	fake := &llm.Scripted{Steps: []llm.ScriptedStep{
		{Calls: []llm.ToolCall{{ID: "1", Name: "read_pdf", Args: json.RawMessage(`{"path":"missing.pdf"}`)}}},
		{Text: "done"},
	}}
	eng := New(fake, reg, h, "sys")
	var names []string
	err = eng.Run(context.Background(), "pdf", func(e Event) {
		if e.Tool != "" {
			names = append(names, e.Tool)
		}
		if e.Err != nil && strings.Contains(e.Err.Error(), "unknown tool") {
			t.Errorf("unknown tool: %v", e.Err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("no tool events")
	}
	for _, n := range names {
		if n != "pdf_read" {
			t.Fatalf("tool name = %q want pdf_read", n)
		}
	}
	var recs int
	for _, r := range h.Session.Records() {
		if r.Type == "tool" {
			recs++
			if r.Tool != "pdf_read" {
				t.Fatalf("record tool = %q", r.Tool)
			}
		}
	}
	if recs == 0 {
		t.Fatal("missing tool record")
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
		{Calls: []llm.ToolCall{{ID: "1", Name: "file_write", Args: json.RawMessage(`{"path":"x","content":"y"}`)}}},
		{Text: "blocked"},
	}}
	eng := New(fake, reg, h, "sys")
	if err := eng.Run(context.Background(), "write", func(Event) {}); err != nil {
		t.Fatal(err)
	}
}

func TestPlanBlocksFetchPath(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	h.Mode = harness.ModePlan
	reg := tools.Builtins(h, nil)
	fake := &llm.Scripted{Steps: []llm.ScriptedStep{
		{Calls: []llm.ToolCall{{ID: "1", Name: "web_fetch", Args: json.RawMessage(`{"url":"http://example.com","path":"x.png"}`)}}},
		{Text: "blocked"},
	}}
	eng := New(fake, reg, h, "sys")
	if err := eng.Run(context.Background(), "save", func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "x.png")); err == nil {
		t.Fatal("plan mode wrote path")
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
