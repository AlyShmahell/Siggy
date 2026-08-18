package tools

import (
	"os"

	"siggy/src/internal/harness"
)

func snapshotFile(h *harness.Harness, rel, abs string) {
	if h == nil || h.Session == nil {
		return
	}
	data, err := os.ReadFile(abs)
	text := ""
	if err == nil {
		text = string(data)
	}
	_ = h.Session.Append(harness.Record{Type: "checkpoint", Path: rel, Text: text})
}
