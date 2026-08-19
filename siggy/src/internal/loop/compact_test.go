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
	for i := 0; i < 6; i++ {
		call := fmt.Sprintf("c%d", i)
		args := fmt.Sprintf(`{"path":"f%d.txt"}`, i)
		if err := h.Session.Append(harness.Record{
			Type: "assistant", ToolCalls: []harness.ToolCallRec{{ID: call, Name: "file_read", Args: args}},
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Session.Append(harness.Record{
			Type: "tool", Tool: "file_read", CallID: call, Args: args, Result: big,
		}); err != nil {
			t.Fatal(err)
		}
	}
	memArgs := `{"path":"` + mem + `/MEMORY.md"}`
	if err := h.Session.Append(harness.Record{
		Type: "assistant", ToolCalls: []harness.ToolCallRec{{ID: "mem", Name: "file_read", Args: memArgs}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Session.Append(harness.Record{
		Type: "tool", Tool: "file_read", CallID: "mem", Args: memArgs, Result: big + " memory-unique",
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
	full := 0
	for _, m := range msgs {
		if m.Role == llm.RoleTool && strings.Contains(m.Content, big) && !strings.Contains(m.Content, pruneCleared) && !strings.Contains(m.Content, "memory-unique") {
			full++
		}
	}
	if full != keepFileResults {
		t.Fatalf("kept full file_read results = %d want %d", full, keepFileResults)
	}
}

func TestPruneSupersedesSamePath(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Session.Close()
	big := strings.Repeat("tool-output ", 40)
	if err := h.Session.Append(harness.Record{Type: "user", Text: "go"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		call := fmt.Sprintf("c%d", i)
		if err := h.Session.Append(harness.Record{
			Type: "assistant", ToolCalls: []harness.ToolCallRec{{ID: call, Name: "file_read", Args: `{"path":"a.txt"}`}},
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Session.Append(harness.Record{
			Type: "tool", Tool: "file_read", CallID: call, Args: `{"path":"a.txt"}`, Result: big + fmt.Sprintf(" v%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Session.Append(harness.Record{Type: "assistant", Text: "done"}); err != nil {
		t.Fatal(err)
	}
	eng := New(&llm.Scripted{}, tools.Builtins(h, nil), h, "")
	eng.pruneToolResults()
	msgs := DeriveMessages(h.Session.Records())
	var full, stubbed int
	for _, m := range msgs {
		if m.Role != llm.RoleTool {
			continue
		}
		if strings.Contains(m.Content, pruneCleared) {
			stubbed++
			continue
		}
		if strings.Contains(m.Content, big) {
			full++
			if !strings.Contains(m.Content, "v2") {
				t.Fatalf("kept stale read: %q", m.Content)
			}
		}
	}
	if stubbed != 2 || full != 1 {
		t.Fatalf("stubbed=%d full=%d", stubbed, full)
	}
}

func TestPruneDumpKeepFiveWithStub(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Session.Close()
	dump := strings.Repeat("search-hit ", 40) + " https://example.com/a"
	if err := h.Session.Append(harness.Record{Type: "user", Text: "find"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		call := fmt.Sprintf("s%d", i)
		if err := h.Session.Append(harness.Record{
			Type: "assistant", ToolCalls: []harness.ToolCallRec{{ID: call, Name: "web_search", Args: fmt.Sprintf(`{"query":"q%d"}`, i)}},
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Session.Append(harness.Record{
			Type: "tool", Tool: "web_search", CallID: call, Args: fmt.Sprintf(`{"query":"q%d"}`, i), Result: dump,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Session.Append(harness.Record{Type: "assistant", Text: "use https://example.com/a"}); err != nil {
		t.Fatal(err)
	}
	eng := New(&llm.Scripted{}, tools.Builtins(h, nil), h, "")
	eng.applyPrune()
	msgs := eng.Messages
	derived := DeriveMessages(h.Session.Records())
	if joinContents(msgs) != joinContents(derived) {
		t.Fatal("Messages not rebuilt from derived surface")
	}
	var full, stubbed int
	for _, m := range derived {
		if m.Role != llm.RoleTool {
			continue
		}
		if strings.Contains(m.Content, pruneCleared) {
			stubbed++
			if !strings.Contains(m.Content, "query=q") {
				t.Fatalf("stub missing args: %q", m.Content)
			}
			if !strings.Contains(m.Content, "https://example.com/a") {
				t.Fatalf("stub missing link: %q", m.Content)
			}
			if strings.Count(m.Content, "search-hit") > 5 && !strings.Contains(m.Content, pruneCleared) {
				t.Fatalf("full dump in stub: %q", m.Content)
			}
			continue
		}
		if strings.Contains(m.Content, dump) {
			full++
		}
	}
	if stubbed != 1 || full != keepDumpResults {
		t.Fatalf("stubbed=%d full=%d", stubbed, full)
	}
}

func TestPruneSkipsUntilAssistantSpeaks(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Session.Close()
	dump := strings.Repeat("tool-output ", 40)
	if err := h.Session.Append(harness.Record{Type: "user", Text: "go"}); err != nil {
		t.Fatal(err)
	}
	if err := h.Session.Append(harness.Record{
		Type: "assistant", ToolCalls: []harness.ToolCallRec{{ID: "c1", Name: "web_fetch", Args: `{"url":"https://x"}`}},
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := h.Session.Append(harness.Record{
			Type: "tool", Tool: "web_fetch", CallID: fmt.Sprintf("c%d", i), Args: `{"url":"https://x"}`, Result: dump,
		}); err != nil {
			t.Fatal(err)
		}
	}
	eng := New(&llm.Scripted{}, tools.Builtins(h, nil), h, "")
	eng.pruneToolResults()
	for _, r := range h.Session.Records() {
		if r.Type == "prune" {
			t.Fatalf("pruned before assistant follow-up: %#v", r)
		}
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
	if err := eng.summarize(context.Background(), nil); err != nil {
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
