package loop

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/tools"
)

func TestComputeThresholds(t *testing.T) {
	th := ComputeThresholds(128000)
	if th.Warn >= th.Auto || th.Auto > th.Hard || th.Hard > th.Window {
		t.Fatalf("ladder = %+v", th)
	}
	if th.Auto < int(0.7*128000) {
		t.Fatalf("auto too low: %d", th.Auto)
	}
}

func TestPruneKeepsMemoryReads(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Session.Close()
	mem := harness.MemoryDir(home, harness.HashWorkspace(root))
	big := strings.Repeat("tool-output ", 40)
	if err := h.Session.Append(harness.Record{Type: "user", Text: "go"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 7; i++ {
		call := fmt.Sprintf("c%d", i)
		if err := h.Session.Append(harness.Record{
			Type: "assistant", ToolCalls: []harness.ToolCallRec{{ID: call, Name: "read_file", Args: `{"path":"a.txt"}`}},
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Session.Append(harness.Record{
			Type: "tool", Tool: "read_file", CallID: call, Args: `{"path":"a.txt"}`, Result: big,
		}); err != nil {
			t.Fatal(err)
		}
	}
	memArgs := `{"path":"` + mem + `/MEMORY.md"}`
	if err := h.Session.Append(harness.Record{
		Type: "assistant", ToolCalls: []harness.ToolCallRec{{ID: "mem", Name: "read_file", Args: memArgs}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Session.Append(harness.Record{
		Type: "tool", Tool: "read_file", CallID: "mem", Args: memArgs, Result: big + " memory-unique",
	}); err != nil {
		t.Fatal(err)
	}
	eng := New(&llm.Scripted{}, tools.Builtins(h, nil), h, "")
	eng.pruneToolResults()
	var pruned, keptMem int
	for _, r := range h.Session.Records() {
		if r.Type == "prune" {
			pruned++
			if r.CallID == "mem" {
				t.Fatal("memory read was pruned")
			}
		}
		if r.Type == "tool" && r.CallID == "mem" && strings.Contains(r.Result, "memory-unique") {
			keptMem++
		}
	}
	if pruned == 0 {
		t.Fatal("expected prune events")
	}
	if keptMem != 1 {
		t.Fatal("memory read body should stay in the log")
	}
	msgs := DeriveMessages(h.Session.Records())
	joined := joinContents(msgs)
	if !strings.Contains(joined, "memory-unique") {
		t.Fatalf("memory read missing from surface: %s", joined)
	}
}

func TestCompactAppendsWithoutDeleting(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Session.Close()
	for _, rec := range []harness.Record{
		{Type: "user", Text: "first"},
		{Type: "assistant", Text: "ok1"},
		{Type: "user", Text: "second"},
		{Type: "assistant", Text: "ok2"},
	} {
		if err := h.Session.Append(rec); err != nil {
			t.Fatal(err)
		}
	}
	before := len(h.Session.Records())
	fake := &llm.Scripted{Steps: []llm.ScriptedStep{{Text: "## Primary Request and Intent\n- second"}}}
	eng := New(fake, tools.Builtins(h, nil), h, "sys")
	eng.Restore(h.Session.Records())
	if err := eng.summarize(context.Background()); err != nil {
		t.Fatal(err)
	}
	recs := h.Session.Records()
	if len(recs) <= before {
		t.Fatalf("compact should append, got %d from %d", len(recs), before)
	}
	var sawCompact, stillFirst bool
	for _, r := range recs {
		if r.Type == "user" && r.Text == "first" {
			stillFirst = true
		}
		if r.Type == "compact" {
			sawCompact = true
			if r.From == 0 || r.To == 0 {
				t.Fatalf("compact range missing: %#v", r)
			}
		}
	}
	if !sawCompact {
		t.Fatal("compact event missing")
	}
	if !stillFirst {
		t.Fatal("original user event was deleted")
	}
}

func TestIsContextOverflow(t *testing.T) {
	if !IsContextOverflow(errStr("context_length_exceeded")) {
		t.Fatal("expected overflow")
	}
	if IsContextOverflow(errStr("timeout")) {
		t.Fatal("timeout is not overflow")
	}
}

type errStr string

func (e errStr) Error() string { return string(e) }
