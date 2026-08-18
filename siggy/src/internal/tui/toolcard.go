package tui

import (
	"bytes"
	"encoding/json"
	"strings"
)

func formatToolCard(name, args string) string {
	args = strings.TrimSpace(args)
	if args == "" || args == "{}" {
		return name
	}
	raw := mergeJSONObjects([]byte(args))
	switch name {
	case "shell":
		var v struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(raw, &v) == nil && v.Command != "" {
			return "shell  " + v.Command
		}
	case "glob":
		var v struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if json.Unmarshal(raw, &v) == nil && v.Pattern != "" {
			if v.Path != "" {
				return "glob  " + v.Pattern + " in " + v.Path
			}
			return "glob  " + v.Pattern
		}
	}
	compact := string(bytes.TrimSpace(raw))
	if compact == "" || compact == "{}" {
		return name
	}
	return name + "  " + compact
}

func mergeJSONObjects(raw []byte) []byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return []byte("{}")
	}
	if json.Valid(raw) {
		return raw
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	merged := map[string]any{}
	found := false
	for {
		var obj map[string]any
		if err := dec.Decode(&obj); err != nil {
			break
		}
		found = true
		for k, v := range obj {
			merged[k] = v
		}
	}
	if !found {
		return raw
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return raw
	}
	return out
}
