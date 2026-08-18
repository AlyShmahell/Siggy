package loop

import (
	"strings"
	"testing"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
)

func TestDeriveMessagesShadowsCompact(t *testing.T) {
	recs := []harness.Record{
		{Seq: 1, Type: "system", Text: "sys"},
		{Seq: 2, Type: "user", Text: "old question"},
		{Seq: 3, Type: "assistant", Text: "old answer", ToolCalls: []harness.ToolCallRec{{ID: "c1", Name: "read_file", Args: `{"path":"a"}`}}},
		{Seq: 4, Type: "tool", Tool: "read_file", CallID: "c1", Result: "file body"},
		{Seq: 5, Type: "user", Text: "new question"},
		{Seq: 6, Type: "compact", Text: "checkpoint: keep going", From: 2, To: 4},
	}
	msgs := DeriveMessages(recs)
	joined := joinContents(msgs)
	if strings.Contains(joined, "old question") || strings.Contains(joined, "file body") {
		t.Fatalf("shadowed text leaked: %#v", msgs)
	}
	if !strings.Contains(joined, "sys") || !strings.Contains(joined, "checkpoint") || !strings.Contains(joined, "new question") {
		t.Fatalf("surface missing checkpoint/tail: %#v", msgs)
	}
}

func TestDeriveMessagesRestoresToolCalls(t *testing.T) {
	recs := []harness.Record{
		{Seq: 1, Type: "user", Text: "list it"},
		{Seq: 2, Type: "assistant", Text: "", ToolCalls: []harness.ToolCallRec{{ID: "c1", Name: "list_dir", Args: `{}`}}},
		{Seq: 3, Type: "tool", Tool: "list_dir", CallID: "c1", Result: "."},
	}
	msgs := DeriveMessages(recs)
	if len(msgs) != 3 {
		t.Fatalf("msgs = %#v", msgs)
	}
	if msgs[1].Role != llm.RoleAssistant || len(msgs[1].ToolCalls) != 1 || msgs[1].ToolCalls[0].ID != "c1" {
		t.Fatalf("tool_calls = %#v", msgs[1])
	}
	if msgs[2].Role != llm.RoleTool || msgs[2].ToolCallID != "c1" {
		t.Fatalf("tool result = %#v", msgs[2])
	}
}

func TestDerivePruneReplacement(t *testing.T) {
	recs := []harness.Record{
		{Seq: 1, Type: "user", Text: "read"},
		{Seq: 2, Type: "assistant", Text: "", ToolCalls: []harness.ToolCallRec{{ID: "c1", Name: "read_file", Args: `{}`}}},
		{Seq: 3, Type: "tool", Tool: "read_file", CallID: "c1", Result: strings.Repeat("x", 400)},
		{Seq: 4, Type: "prune", Tool: "read_file", CallID: "c1", Result: pruneSentinel, ReplacesSeq: 3},
	}
	msgs := DeriveMessages(recs)
	joined := joinContents(msgs)
	if strings.Contains(joined, strings.Repeat("x", 40)) {
		t.Fatalf("pruned body still on surface: %#v", msgs)
	}
	if !strings.Contains(joined, pruneSentinel) {
		t.Fatalf("sentinel missing: %#v", msgs)
	}
}

func joinContents(msgs []llm.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(m.Content)
		b.WriteByte('\n')
	}
	return b.String()
}
