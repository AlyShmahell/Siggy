package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/tools"
)

type callState string

const (
	stateQueued    callState = "queued"
	stateAwaiting  callState = "awaiting_approval"
	stateExecuting callState = "executing"
	stateDone      callState = "done"
	stateDenied    callState = "denied"
)

type scheduled struct {
	call  llm.ToolCall
	state callState
}

type Scheduler struct {
	mu      sync.Mutex
	busy    bool
	reg     *tools.Registry
	harness *harness.Harness
}

func NewScheduler(reg *tools.Registry, h *harness.Harness) *Scheduler {
	return &Scheduler{reg: reg, harness: h}
}

func (s *Scheduler) Busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy
}

func (s *Scheduler) Run(ctx context.Context, calls []llm.ToolCall, emit func(Event)) ([]llm.Message, error) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return nil, fmt.Errorf("cannot schedule tools while another batch is in flight")
	}
	s.busy = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()

	var results []llm.Message
	for _, call := range calls {
		msg := s.execOne(ctx, call, emit)
		results = append(results, msg)
	}
	return results, nil
}

func (s *Scheduler) execOne(ctx context.Context, call llm.ToolCall, emit func(Event)) llm.Message {
	tool, ok := s.reg.Get(call.Name)
	if !ok {
		err := tools.Missing(call.Name)
		emit(Event{Kind: KindError, Tool: call.Name, CallID: call.ID, Err: err})
		return toolMessage(call, err.Error())
	}
	args := string(call.Args)
	if err := s.harness.Loops.Observe(call.Name, args); err != nil {
		emit(Event{Kind: KindError, Tool: call.Name, CallID: call.ID, Err: err})
		return toolMessage(call, err.Error())
	}
	if err := s.harness.Mode.Allows(tool.Risk()); err != nil {
		emit(Event{Kind: KindError, Tool: call.Name, CallID: call.ID, Err: err})
		return toolMessage(call, err.Error())
	}

	if tool.Risk() != harness.RiskRead {
		req := harness.ApprovalRequest{
			ID:      call.ID,
			Tool:    call.Name,
			Summary: summarize(call.Name, args),
			Risk:    string(tool.Risk()),
			Reply:   make(chan harness.Decision, 1),
		}
		if !s.harness.Approvals.Auto() {
			emit(Event{Kind: KindApproval, Tool: call.Name, CallID: call.ID, Args: args, Approval: &req})
		}
		d, err := s.harness.Approvals.Ask(ctx, req)
		if err != nil {
			return toolMessage(call, err.Error())
		}
		if !d.Allowed() {
			emit(Event{Kind: KindToolEnd, Tool: call.Name, CallID: call.ID, Text: "denied"})
			return toolMessage(call, "denied by user")
		}
	}

	emit(Event{Kind: KindToolStart, Tool: call.Name, CallID: call.ID, Args: args})
	if v, ok := tool.(tools.Visual); ok {
		out, images, err := v.RunVisual(ctx, call.Args)
		if err != nil {
			text := err.Error()
			if out != "" {
				text = out + "\n" + text
			}
			emit(Event{Kind: KindToolEnd, Tool: call.Name, CallID: call.ID, Text: text, Err: err})
			_ = s.harness.Session.Append(harness.Record{Type: "tool", Tool: call.Name, CallID: call.ID, Args: args, Result: text})
			return toolMessage(call, text)
		}
		emit(Event{Kind: KindToolEnd, Tool: call.Name, CallID: call.ID, Text: out})
		_ = s.harness.Session.Append(harness.Record{Type: "tool", Tool: call.Name, CallID: call.ID, Args: args, Result: out})
		msg := toolMessage(call, out)
		msg.Parts = images
		return msg
	}
	out, err := tool.Run(ctx, call.Args)
	if err != nil {
		text := err.Error()
		if out != "" {
			text = out + "\n" + text
		}
		emit(Event{Kind: KindToolEnd, Tool: call.Name, CallID: call.ID, Text: text, Err: err})
		_ = s.harness.Session.Append(harness.Record{Type: "tool", Tool: call.Name, CallID: call.ID, Args: args, Result: text})
		return toolMessage(call, text)
	}
	emit(Event{Kind: KindToolEnd, Tool: call.Name, CallID: call.ID, Text: out})
	_ = s.harness.Session.Append(harness.Record{Type: "tool", Tool: call.Name, CallID: call.ID, Args: args, Result: out})
	return toolMessage(call, out)
}

func toolMessage(call llm.ToolCall, content string) llm.Message {
	return llm.Message{Role: llm.RoleTool, ToolCallID: call.ID, Name: call.Name, Content: content}
}

func summarize(name, args string) string {
	if len(args) > 240 {
		args = args[:240] + "…"
	}
	if !json.Valid([]byte(args)) && args != "" {
		return name + " " + args
	}
	return fmt.Sprintf("%s %s", name, args)
}
