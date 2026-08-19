package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
	"siggy/src/internal/prompt"
)

const (
	defaultWindow     = 128000
	summaryReserve    = 20000
	autoBuffer        = 13000
	warnBuffer        = 20000
	hardBuffer        = 3000
	defaultPct        = 0.7
	keepDumpResults   = 5
	keepFileResults   = 5
	prunePreviewRunes = 300
	pruneCleared      = "[cleared; call this tool again if you need the rest]"
	pruneSentinel     = pruneCleared
	maxCompactFail    = 3
)

var bulkyTools = map[string]bool{
	"file_read":  true,
	"grep":       true,
	"glob":       true,
	"dir_list":   true,
	"shell":      true,
	"web_fetch":  true,
	"web_search": true,
	"pdf_read":   true,
}

var dumpTools = map[string]bool{
	"web_search": true,
	"web_fetch":  true,
}

var reHTTPLink = regexp.MustCompile(`https?://[^\s<>"']+`)

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
	e.applyPrune()
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

func (e *Engine) applyPrune() {
	e.pruneToolResults()
	if e.Harness != nil && e.Harness.Session != nil {
		e.Messages = DeriveMessages(e.Harness.Session.Records())
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
	var dump, files []harness.Record
	for _, r := range recs {
		if r.Type != "tool" || shadowed[r.Seq] {
			continue
		}
		if !bulkyTools[r.Tool] {
			continue
		}
		if strings.Contains(r.Result, pruneCleared) {
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
		if !followedByAssistant(recs, r.Seq) {
			continue
		}
		if dumpTools[r.Tool] {
			dump = append(dump, r)
		} else {
			files = append(files, r)
		}
	}
	dump = e.dropSuperseded(dump)
	files = e.dropSuperseded(files)
	e.dropOldest(dump, keepDumpResults)
	e.dropOldest(files, keepFileResults)
}

func pruneKey(r harness.Record) string {
	var m map[string]any
	_ = json.Unmarshal([]byte(r.Args), &m)
	arg := func(k string) string {
		if m == nil {
			return ""
		}
		v, ok := m[k]
		if !ok || v == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(v))
	}
	id := func(parts ...string) string {
		for _, p := range parts {
			if p != "" {
				return r.Tool + "\t" + strings.Join(parts, "\t")
			}
		}
		return r.Tool + "\t" + r.CallID
	}
	switch r.Tool {
	case "file_read", "pdf_read", "dir_list":
		return id(arg("path"))
	case "web_fetch":
		return id(arg("url"))
	case "web_search":
		return id(arg("query"))
	case "grep", "glob":
		p, path := arg("pattern"), arg("path")
		if p == "" && path == "" {
			return r.Tool + "\t" + r.CallID
		}
		return r.Tool + "\t" + p + "\t" + path
	case "shell":
		return id(arg("command"))
	default:
		return r.Tool + "\t" + r.CallID
	}
}

func (e *Engine) dropSuperseded(bulky []harness.Record) []harness.Record {
	seen := map[string]bool{}
	var keep []harness.Record
	for i := len(bulky) - 1; i >= 0; i-- {
		r := bulky[i]
		k := pruneKey(r)
		if seen[k] {
			e.stubRecord(r)
			continue
		}
		seen[k] = true
		keep = append(keep, r)
	}
	for i, j := 0, len(keep)-1; i < j; i, j = i+1, j-1 {
		keep[i], keep[j] = keep[j], keep[i]
	}
	return keep
}

func followedByAssistant(recs []harness.Record, seq int) bool {
	for _, r := range recs {
		if r.Type == "assistant" && r.Seq > seq {
			return true
		}
	}
	return false
}

func (e *Engine) dropOldest(bulky []harness.Record, keep int) {
	drop := len(bulky) - keep
	if drop <= 0 {
		return
	}
	for _, r := range bulky[:drop] {
		e.stubRecord(r)
	}
}

func (e *Engine) stubRecord(r harness.Record) {
	_ = e.Harness.Session.Append(harness.Record{
		Type:        "prune",
		Tool:        r.Tool,
		CallID:      r.CallID,
		Args:        r.Args,
		Result:      pruneStub(r),
		ReplacesSeq: r.Seq,
	})
}

func pruneStub(r harness.Record) string {
	var b strings.Builder
	b.WriteString(r.Tool)
	if arg := pruneArgSummary(r.Args); arg != "" {
		b.WriteByte(' ')
		b.WriteString(arg)
	}
	b.WriteByte('\n')
	b.WriteString(prunePreview(r.Tool, r.Result))
	b.WriteByte('\n')
	b.WriteString(pruneCleared)
	return b.String()
}

func pruneArgSummary(args string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil {
		s := strings.TrimSpace(args)
		return truncateRunes(s, 80)
	}
	for _, k := range []string{"query", "url", "path", "command", "pattern"} {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" {
			continue
		}
		return k + "=" + truncateRunes(s, 80)
	}
	return ""
}

func prunePreview(tool, result string) string {
	if tool == "web_search" || tool == "web_fetch" {
		if links := httpLinks(result); len(links) > 0 {
			return strings.Join(links, " ")
		}
	}
	return truncateRunes(strings.TrimSpace(result), prunePreviewRunes)
}

func httpLinks(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range reHTTPLink.FindAllString(s, 8) {
		if seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func isMemoryRead(r harness.Record, memDir string) bool {
	if r.Tool != "file_read" {
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
