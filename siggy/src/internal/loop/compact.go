package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/prompt"
)

const (
	defaultWindow   = 128000
	summaryReserve  = 20000
	autoBuffer      = 13000
	warnBuffer      = 20000
	hardBuffer      = 3000
	defaultPct      = 0.7
	keepToolResults = 5
	pruneSentinel   = "[Old tool result content cleared]"
	maxCompactFail  = 3
)

var bulkyTools = map[string]bool{
	"read_file":  true,
	"grep":       true,
	"glob":       true,
	"list_dir":   true,
	"shell":      true,
	"web_fetch":  true,
	"web_search": true,
	"read_pdf":   true,
}

type Thresholds struct {
	Warn, Auto, Hard, Window int
}

func ComputeThresholds(window int) Thresholds {
	if window <= 0 {
		window = defaultWindow
	}
	effective := window - summaryReserve
	if effective < 0 {
		effective = 0
	}
	auto := int(defaultPct * float64(window))
	if abs := effective - autoBuffer; abs > auto {
		auto = abs
	}
	warn := int((defaultPct - 0.1) * float64(window))
	if abs := auto - warnBuffer; abs > warn {
		warn = abs
	}
	hard := effective - hardBuffer
	if hard < auto {
		hard = auto
	}
	return Thresholds{Warn: warn, Auto: auto, Hard: hard, Window: window}
}

func (e *Engine) window() int {
	if e.ContextWindow > 0 {
		return e.ContextWindow
	}
	return defaultWindow
}

func (e *Engine) tokenUsed() int {
	if e.LastPrompt > 0 {
		return e.LastPrompt
	}
	var specs []llm.ToolSpec
	if e.Tools != nil {
		specs = e.Tools.Specs()
	}
	return EstimateRequest(e.Messages, specs)
}

func (e *Engine) maybeCompact(ctx context.Context, emit func(Event), forceFast, forceLLM bool) {
	th := ComputeThresholds(e.window())
	used := e.tokenUsed()
	if !forceFast && !forceLLM && used < th.Auto {
		return
	}
	emit(Event{Kind: KindNode, Node: "Compact"})
	e.pruneToolResults()
	e.Messages = DeriveMessages(e.Harness.Session.Records())
	used = EstimateTokens(e.Messages)
	e.LastPrompt = used
	if forceFast && !forceLLM && used < th.Auto {
		return
	}
	if e.CompactFails >= maxCompactFail && !forceLLM {
		return
	}
	if forceLLM || used >= th.Auto {
		if err := e.summarize(ctx, emit); err != nil {
			e.CompactFails++
			return
		}
		e.Messages = DeriveMessages(e.Harness.Session.Records())
		next := EstimateTokens(e.Messages)
		if next >= used {
			e.CompactFails++
			return
		}
		e.CompactFails = 0
		e.LastPrompt = next
	}
}

func (e *Engine) pruneToolResults() {
	if e.Harness == nil || e.Harness.Session == nil {
		return
	}
	recs := e.Harness.Session.Records()
	mem := ""
	if e.Harness.Workspace != nil {
		mem = harness.MemoryDir(e.Harness.Home, harness.HashWorkspace(e.Harness.Workspace.Root))
	}
	var bulky []harness.Record
	shadowed := map[int]bool{}
	for _, r := range recs {
		if r.Type == "compact" || r.Type == "rewind" {
			from, to := r.From, r.To
			if to < from {
				from, to = to, from
			}
			for i := from; i <= to; i++ {
				shadowed[i] = true
			}
		}
		if r.ReplacesSeq > 0 {
			shadowed[r.ReplacesSeq] = true
		}
	}
	for _, r := range recs {
		if r.Type != "tool" || shadowed[r.Seq] {
			continue
		}
		if !bulkyTools[r.Tool] {
			continue
		}
		if strings.Contains(r.Result, pruneSentinel) {
			continue
		}
		if strings.HasPrefix(strings.ToLower(r.Result), "denied") || strings.Contains(strings.ToLower(r.Result), "error") {
			continue
		}
		if isMemoryRead(r, mem) {
			continue
		}
		if utf8.RuneCountInString(r.Result) < 200 {
			continue
		}
		bulky = append(bulky, r)
	}
	drop := len(bulky) - keepToolResults
	if drop <= 0 {
		return
	}
	for _, r := range bulky[:drop] {
		_ = e.Harness.Session.Append(harness.Record{
			Type:        "prune",
			Tool:        r.Tool,
			CallID:      r.CallID,
			Args:        r.Args,
			Result:      pruneSentinel,
			ReplacesSeq: r.Seq,
		})
	}
}

func isMemoryRead(r harness.Record, memDir string) bool {
	if r.Tool != "read_file" {
		return false
	}
	low := strings.ToLower(r.Args + r.Path)
	if strings.Contains(low, "/memory/") || strings.Contains(low, `\memory\`) {
		return true
	}
	if memDir != "" && strings.Contains(r.Args, memDir) {
		return true
	}
	var args struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(r.Args), &args)
	p := strings.ToLower(args.Path)
	return strings.Contains(p, "memory/") || strings.HasPrefix(p, "memory")
}

func (e *Engine) summarize(ctx context.Context, emit func(Event)) error {
	if emit == nil {
		emit = func(Event) {}
	}
	recs := e.Harness.Session.Records()
	msgs := DeriveMessages(recs)
	if len(msgs) < 4 {
		return fmt.Errorf("nothing to compact")
	}
	from, to := compactRange(recs)
	if from == 0 || to == 0 || to < from {
		return fmt.Errorf("no compactable range")
	}
	instr := strings.TrimSpace(prompt.Compact(e.Harness.Home))
	if instr == "" {
		instr = defaultCompactInstr
	}
	req := append(append([]llm.Message{}, msgs...), llm.Message{Role: llm.RoleUser, Content: instr})
	ch, err := e.LLM.Stream(ctx, llm.Request{Messages: req})
	if err != nil {
		return err
	}
	var text string
	var got llm.Usage
	for chunk := range ch {
		if chunk.Err != nil {
			return chunk.Err
		}
		text += chunk.Text
		if usageFromChunk(chunk.Usage) {
			got = chunk.Usage
		}
	}
	e.finishUsage(got, nil, emit)
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("empty compact summary")
	}
	return e.Harness.Session.Append(harness.Record{
		Type: "compact",
		Text: text,
		From: from,
		To:   to,
	})
}

func compactRange(recs []harness.Record) (from, to int) {
	var live []harness.Record
	shadowed := map[int]bool{}
	for _, r := range recs {
		if r.Type == "compact" || r.Type == "rewind" {
			a, b := r.From, r.To
			if b < a {
				a, b = b, a
			}
			for i := a; i <= b; i++ {
				shadowed[i] = true
			}
		}
		if r.ReplacesSeq > 0 {
			shadowed[r.ReplacesSeq] = true
		}
	}
	for _, r := range recs {
		if r.Type == "system" || r.Type == "compact" || r.Type == "rewind" || r.Type == "checkpoint" || r.Type == "todo" || r.Type == "prune" || r.Type == "usage" {
			continue
		}
		if shadowed[r.Seq] {
			continue
		}
		live = append(live, r)
	}
	lastUser := -1
	for i, r := range live {
		if r.Type == "user" {
			lastUser = i
		}
	}
	if lastUser <= 0 {
		return 0, 0
	}
	head := live[0]
	to = live[lastUser].Seq - 1
	if to < head.Seq {
		return 0, 0
	}
	return head.Seq, to
}

func IsContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, k := range []string{"context_length_exceeded", "context window", "too many tokens", "maximum context", "prompt is too long"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

const defaultCompactInstr = `You are now acting as a compaction engine for this AI coding assistant. Condense the conversation ABOVE into a structured checkpoint that lets another model resume the work with no loss of essential context.

Output EXACTLY the Markdown structure below: keep every section, in order. Use terse bullets, not prose paragraphs. Write "(none)" for an empty section — never drop a section.

## Primary Request and Intent
## Key Technical Concepts
## Files and Code
## Errors and Fixes
## Pending Jobs
## Current Work
## Next Step
## Critical Context

Rules:
- Write concise English engineering prose. Preserve exact file paths, commands, error strings, identifiers, numeric values, function signatures, and syntax fragments.
- Capture user feedback and explicit instructions faithfully, especially corrections.
- Do NOT mention this summarization request or that the context was compacted.
- Output only the checkpoint text: do not call any tool or take any other action.
`
