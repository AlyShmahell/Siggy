package loop

import (
	"context"
	"fmt"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/tools"
)

const (
	MaxTurns     = 24
	CompactAfter = 40
)

type Engine struct {
	LLM      llm.Client
	Tools    *tools.Registry
	Harness  *harness.Harness
	Messages []llm.Message
}

func New(client llm.Client, reg *tools.Registry, h *harness.Harness, system string) *Engine {
	e := &Engine{LLM: client, Tools: reg, Harness: h}
	if system != "" {
		e.Messages = []llm.Message{{Role: llm.RoleSystem, Content: system}}
		_ = h.Session.Append(harness.Record{Type: "system", Text: system})
	}
	return e
}

func (e *Engine) Restore(records []harness.Record) {
	e.Messages = nil
	for _, r := range records {
		switch r.Type {
		case "system":
			e.Messages = append(e.Messages, llm.Message{Role: llm.RoleSystem, Content: r.Text})
		case "user":
			e.Messages = append(e.Messages, llm.Message{Role: llm.RoleUser, Content: r.Text})
		case "assistant":
			e.Messages = append(e.Messages, llm.Message{Role: llm.RoleAssistant, Content: r.Text})
		case "tool":
			e.Messages = append(e.Messages, llm.Message{Role: llm.RoleTool, Name: r.Tool, ToolCallID: r.CallID, Content: r.Result})
		case "compact":
			e.Messages = append(e.Messages, llm.Message{Role: llm.RoleSystem, Content: r.Text})
		}
	}
}

func (e *Engine) Compact() {
	if len(e.Messages) <= CompactAfter {
		return
	}
	keep := 12
	head := e.Messages[:1]
	tail := e.Messages[len(e.Messages)-keep:]
	note := llm.Message{Role: llm.RoleSystem, Content: fmt.Sprintf("Earlier turns were compacted (%d messages omitted).", len(e.Messages)-1-keep)}
	e.Messages = append(append(head, note), tail...)
	_ = e.Harness.Session.Append(harness.Record{Type: "compact", Text: note.Content})
}

func (e *Engine) Run(ctx context.Context, user string, emit func(Event)) error {
	if emit == nil {
		emit = func(Event) {}
	}
	e.Messages = append(e.Messages, llm.Message{Role: llm.RoleUser, Content: user})
	_ = e.Harness.Session.Append(harness.Record{Type: "user", Role: "user", Text: user})
	sched := NewScheduler(e.Tools, e.Harness)

	for turn := 0; turn < MaxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		e.Compact()
		emit(Event{Kind: KindNode, Node: "Think"})
		ch, err := e.LLM.Stream(ctx, llm.Request{Messages: e.Messages, Tools: e.Tools.Specs()})
		if err != nil {
			emit(Event{Kind: KindError, Err: err})
			return err
		}
		var text string
		var calls []llm.ToolCall
		for chunk := range ch {
			if chunk.Err != nil {
				emit(Event{Kind: KindError, Err: chunk.Err})
				return chunk.Err
			}
			if chunk.Text != "" {
				text += chunk.Text
				emit(Event{Kind: KindText, Text: chunk.Text})
			}
			if chunk.Usage.Total > 0 || chunk.Usage.Prompt > 0 {
				emit(Event{Kind: KindUsage, PromptTokens: chunk.Usage.Prompt, TotalTokens: chunk.Usage.Total})
			}
			if len(chunk.ToolCalls) > 0 {
				calls = append(calls, chunk.ToolCalls...)
			}
		}
		asst := llm.Message{Role: llm.RoleAssistant, Content: text, ToolCalls: calls}
		e.Messages = append(e.Messages, asst)
		if text != "" {
			_ = e.Harness.Session.Append(harness.Record{Type: "assistant", Role: "assistant", Text: text})
		}
		if len(calls) == 0 {
			emit(Event{Kind: KindNode, Node: "Done"})
			emit(Event{Kind: KindDone})
			return nil
		}
		emit(Event{Kind: KindNode, Node: "ScheduleTools"})
		results, err := sched.Run(ctx, calls, emit)
		if err != nil {
			emit(Event{Kind: KindError, Err: err})
			return err
		}
		e.Messages = append(e.Messages, results...)
	}
	err := fmt.Errorf("turn cap %d reached", MaxTurns)
	emit(Event{Kind: KindError, Err: err})
	return err
}
