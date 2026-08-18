package loop

import (
	"context"
	"fmt"
	"strings"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/tools"
)

const MaxTurns = 24

type Engine struct {
	LLM            llm.Client
	Tools          *tools.Registry
	Harness        *harness.Harness
	Messages       []llm.Message
	ContextWindow  int
	LastPrompt     int
	CompactFails   int
}

func New(client llm.Client, reg *tools.Registry, h *harness.Harness, system string) *Engine {
	e := &Engine{LLM: client, Tools: reg, Harness: h, ContextWindow: defaultWindow}
	if system != "" {
		e.Messages = []llm.Message{{Role: llm.RoleSystem, Content: system}}
		_ = h.Session.Append(harness.Record{Type: "system", Text: system})
	}
	return e
}

func (e *Engine) Restore(records []harness.Record) {
	e.Messages = DeriveMessages(records)
}

func (e *Engine) CompactNow(ctx context.Context, emit func(Event), fast bool) {
	if emit == nil {
		emit = func(Event) {}
	}
	e.maybeCompact(ctx, emit, fast, !fast)
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
		e.maybeCompact(ctx, emit, false, false)
		emit(Event{Kind: KindNode, Node: "Think"})
		text, calls, err := e.streamOnce(ctx, emit)
		if err != nil && IsContextOverflow(err) {
			e.maybeCompact(ctx, emit, false, true)
			text, calls, err = e.streamOnce(ctx, emit)
		}
		if err != nil {
			emit(Event{Kind: KindError, Err: err})
			return err
		}
		asst := llm.Message{Role: llm.RoleAssistant, Content: text, ToolCalls: calls}
		e.Messages = append(e.Messages, asst)
		rec := harness.Record{Type: "assistant", Role: "assistant", Text: text}
		for _, c := range calls {
			rec.ToolCalls = append(rec.ToolCalls, harness.ToolCallRec{ID: c.ID, Name: c.Name, Args: string(c.Args)})
		}
		if text != "" || len(calls) > 0 {
			_ = e.Harness.Session.Append(rec)
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

func (e *Engine) streamOnce(ctx context.Context, emit func(Event)) (string, []llm.ToolCall, error) {
	ch, err := e.LLM.Stream(ctx, llm.Request{Messages: e.Messages, Tools: e.Tools.Specs()})
	if err != nil {
		return "", nil, err
	}
	var text string
	var calls []llm.ToolCall
	for chunk := range ch {
		if chunk.Err != nil {
			return text, calls, chunk.Err
		}
		if chunk.Text != "" {
			text += chunk.Text
			emit(Event{Kind: KindText, Text: chunk.Text})
		}
		if chunk.Usage.Prompt > 0 {
			e.LastPrompt = chunk.Usage.Prompt
			emit(Event{Kind: KindUsage, PromptTokens: chunk.Usage.Prompt, CompletionTokens: chunk.Usage.Completion, TotalTokens: chunk.Usage.Total})
		} else if chunk.Usage.Total > 0 || chunk.Usage.Completion > 0 {
			emit(Event{Kind: KindUsage, PromptTokens: chunk.Usage.Prompt, CompletionTokens: chunk.Usage.Completion, TotalTokens: chunk.Usage.Total})
		}
		if len(chunk.ToolCalls) > 0 {
			calls = append(calls, chunk.ToolCalls...)
		}
	}
	return text, calls, nil
}

func RewindRecords(recs []harness.Record, throughSeq int) harness.Record {
	max := 0
	for _, r := range recs {
		if r.Seq > max {
			max = r.Seq
		}
	}
	return harness.Record{Type: "rewind", From: throughSeq + 1, To: max, Text: fmt.Sprintf("rewound to seq %d", throughSeq)}
}

func LastUserSeq(recs []harness.Record) int {
	seq := 0
	for _, r := range recs {
		if r.Type == "user" {
			seq = r.Seq
		}
	}
	return seq
}

func JoinKinds(evs []Kind) string {
	var b strings.Builder
	for _, k := range evs {
		b.WriteString(string(k))
		b.WriteByte(',')
	}
	return b.String()
}
