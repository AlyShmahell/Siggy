package tui

import "siggy/src/internal/harness"

func transcriptFromRecords(records []harness.Record) []line {
	lines := []line{{kind: "sys", text: "session ready"}}
	for _, r := range records {
		switch r.Type {
		case "user":
			if r.Text != "" {
				lines = append(lines, line{kind: "user", text: r.Text})
			}
		case "assistant":
			if r.Text != "" {
				lines = append(lines, line{kind: "asst-live", text: r.Text})
			}
		case "tool":
			lines = append(lines, line{kind: "tool", tool: r.Tool, text: r.Args})
			text := r.Result
			kind := "ok"
			if len(text) > 240 {
				text = text[:240] + "…"
			}
			if text != "" {
				lines = append(lines, line{kind: kind, tool: r.Tool, text: text})
			}
		case "compact":
			txt := r.Text
			if len(txt) > 160 {
				txt = txt[:160] + "…"
			}
			lines = append(lines, line{kind: "sys", text: "compacted " + txt})
		case "rewind":
			lines = append(lines, line{kind: "sys", text: r.Text})
		}
	}
	return lines
}

func (m *model) loadTranscript(note string) {
	m.err = ""
	m.streaming = ""
	m.followBottom = true
	m.blurTranscript()
	m.transAnchor, m.transCaret = 0, 0
	var recs []harness.Record
	if m.h != nil && m.h.Session != nil {
		recs = m.h.Session.Records()
	}
	m.lines = transcriptFromRecords(recs)
	m.applyUsageFromRecords(recs)
	if note != "" {
		m.lines = append(m.lines, line{kind: "sys", text: note})
	}
	if m.width > 0 {
		m.refresh()
	}
}
