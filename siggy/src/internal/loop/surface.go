package loop

import (
	"encoding/json"
	"unicode/utf8"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
)

func DeriveMessages(records []harness.Record) []llm.Message {
	shadowed := map[int]bool{}
	replace := map[int]harness.Record{}
	var checkpoints []harness.Record
	maxSeq := 0
	for _, r := range records {
		if r.Seq > maxSeq {
			maxSeq = r.Seq
		}
		switch r.Type {
		case "compact", "rewind":
			from, to := r.From, r.To
			if to == 0 && from == 0 {
				continue
			}
			if to < from {
				from, to = to, from
			}
			for i := from; i <= to; i++ {
				shadowed[i] = true
			}
			if r.Type == "compact" && r.Text != "" {
				checkpoints = append(checkpoints, r)
			}
		}
		if r.ReplacesSeq > 0 {
			shadowed[r.ReplacesSeq] = true
			replace[r.ReplacesSeq] = r
		}
	}

	var msgs []llm.Message
	for _, r := range records {
		if r.Type == "system" {
			msgs = append(msgs, llm.Message{Role: llm.RoleSystem, Content: r.Text})
			continue
		}
		if r.Type == "compact" && r.Text != "" && r.From == 0 && r.To == 0 {
			msgs = append(msgs, llm.Message{Role: llm.RoleSystem, Content: r.Text})
			continue
		}
		if r.Type == "compact" && r.Text != "" {
			continue
		}
		if r.Type == "rewind" || r.Type == "checkpoint" || r.Type == "todo" || r.Type == "prune" || r.Type == "usage" {
			continue
		}
		if shadowed[r.Seq] {
			if repl, ok := replace[r.Seq]; ok {
				msgs = append(msgs, recordMessage(repl))
			}
			continue
		}
		if m := recordMessage(r); m.Role != "" {
			msgs = append(msgs, m)
		}
	}
	if len(checkpoints) > 0 {
		last := checkpoints[len(checkpoints)-1]
		insert := llm.Message{Role: llm.RoleSystem, Content: last.Text}
		sys := 0
		for sys < len(msgs) && msgs[sys].Role == llm.RoleSystem && !isCompactNote(msgs[sys].Content) {
			sys++
		}
		out := append([]llm.Message{}, msgs[:sys]...)
		out = append(out, insert)
		out = append(out, msgs[sys:]...)
		msgs = out
	}
	return dropEmpty(msgs)
}

func recordMessage(r harness.Record) llm.Message {
	switch r.Type {
	case "user":
		return llm.Message{Role: llm.RoleUser, Content: r.Text}
	case "assistant":
		var calls []llm.ToolCall
		for _, tc := range r.ToolCalls {
			calls = append(calls, llm.ToolCall{ID: tc.ID, Name: tc.Name, Args: json.RawMessage(tc.Args)})
		}
		return llm.Message{Role: llm.RoleAssistant, Content: r.Text, ToolCalls: calls}
	case "tool", "prune":
		return llm.Message{Role: llm.RoleTool, Name: r.Tool, ToolCallID: r.CallID, Content: r.Result}
	case "system":
		return llm.Message{Role: llm.RoleSystem, Content: r.Text}
	default:
		return llm.Message{}
	}
}

func isCompactNote(s string) bool {
	return len(s) > 20 && (containsFold(s, "checkpoint") || containsFold(s, "Primary Request") || containsFold(s, "compacted"))
}

func containsFold(s, sub string) bool {
	return utf8.RuneCountInString(s) >= utf8.RuneCountInString(sub) && (len(s) >= len(sub) && (stringIndexFold(s, sub) >= 0))
}

func stringIndexFold(s, sub string) int {
	ls, lsub := len(s), len(sub)
	if lsub == 0 {
		return 0
	}
	for i := 0; i+lsub <= ls; i++ {
		match := true
		for j := 0; j < lsub; j++ {
			a, b := s[i+j], sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func dropEmpty(msgs []llm.Message) []llm.Message {
	out := msgs[:0]
	for _, m := range msgs {
		if m.Role == "" {
			continue
		}
		if m.Role == llm.RoleAssistant && m.Content == "" && len(m.ToolCalls) == 0 {
			continue
		}
		out = append(out, m)
	}
	return out
}

func EstimateTokens(msgs []llm.Message) int {
	return EstimateRequest(msgs, nil)
}

func EstimateRequest(msgs []llm.Message, tools []llm.ToolSpec) int {
	n := 0
	for _, m := range msgs {
		n += utf8.RuneCountInString(m.Content)
		for _, tc := range m.ToolCalls {
			n += utf8.RuneCountInString(tc.Name)
			n += utf8.RuneCountInString(string(tc.Args))
		}
	}
	for _, t := range tools {
		n += utf8.RuneCountInString(t.Name)
		n += utf8.RuneCountInString(t.Description)
		n += utf8.RuneCountInString(string(t.Parameters))
	}
	return n / 4
}
