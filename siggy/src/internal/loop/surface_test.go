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
		{Seq: 3, Type: "assistant", Text: "old answer", ToolCalls: []harness.ToolCallRec{{ID: "c1", Name: "file_read", Args: `{"path":"a"}`}}},
		{Seq: 4, Type: "tool", Tool: "file_read", CallID: "c1", Result: "file body"},
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
		{Seq: 2, Type: "assistant", Text: "", ToolCalls: []harness.ToolCallRec{{ID: "c1", Name: "dir_list", Args: `{}`}}},
		{Seq: 3, Type: "tool", Tool: "dir_list", CallID: "c1", Result: "."},
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
		{Seq: 2, Type: "assistant", Text: "", ToolCalls: []harness.ToolCallRec{{ID: "c1", Name: "file_read", Args: `{}`}}},
		{Seq: 3, Type: "tool", Tool: "file_read", CallID: "c1", Result: strings.Repeat("x", 400)},
		{Seq: 4, Type: "prune", Tool: "file_read", CallID: "c1", Result: pruneCleared, ReplacesSeq: 3},
	}
	msgs := DeriveMessages(recs)
	joined := joinContents(msgs)
	if strings.Contains(joined, strings.Repeat("x", 40)) {
		t.Fatalf("pruned body still on surface: %#v", msgs)
	}
	if !strings.Contains(joined, pruneCleared) {
		t.Fatalf("sentinel missing: %#v", msgs)
	}
}

func TestDeriveMessagesIgnoresUsage(t *testing.T) {
	recs := []harness.Record{
		{Seq: 1, Type: "user", Text: "hi"},
		{Seq: 2, Type: "usage", PromptTokens: 639, CompletionTokens: 4983, TotalTokens: 5622},
		{Seq: 3, Type: "assistant", Text: "hello"},
	}
	msgs := DeriveMessages(recs)
	joined := joinContents(msgs)
	if strings.Contains(joined, "639") || strings.Contains(joined, "usage") {
		t.Fatalf("usage leaked into messages: %#v", msgs)
	}
	if len(msgs) != 2 {
		t.Fatalf("msgs = %#v", msgs)
	}
}

func TestEstimateRequestIncludesTools(t *testing.T) {
	msgs := []llm.Message{{Role: llm.RoleUser, Content: "abcd"}}
	base := EstimateTokens(msgs)
	specs := []llm.ToolSpec{{
		Name:        "file_read",
		Description: strings.Repeat("d", 40),
		Parameters:  []byte(strings.Repeat("p", 40)),
	}}
	got := EstimateRequest(msgs, specs)
	if got <= base {
		t.Fatalf("tools not counted: base=%d got=%d", base, got)
	}
}

func TestEstimatePartsSplitsBuckets(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: strings.Repeat("s", 40)},
		{Role: llm.RoleUser, Content: strings.Repeat("u", 20)},
	}
	specs := []llm.ToolSpec{{
		Name:        "file_read",
		Description: strings.Repeat("d", 40),
		Parameters:  []byte(strings.Repeat("p", 40)),
	}}
	p := EstimateParts(msgs, specs, strings.Repeat("x", 12))
	if p.System != 10 {
		t.Fatalf("system = %d", p.System)
	}
	if p.Chat != 5 {
		t.Fatalf("chat = %d", p.Chat)
	}
	if p.Tools < 1 {
		t.Fatalf("tools = %d", p.Tools)
	}
	if p.Draft != 3 {
		t.Fatalf("draft = %d", p.Draft)
	}
	if p.Total() != p.System+p.Tools+p.Chat+p.Draft {
		t.Fatalf("total = %d", p.Total())
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
