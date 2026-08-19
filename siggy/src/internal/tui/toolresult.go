package tui

import (
	"fmt"
	"strings"
)

func toolResultKind(text string, err error) string {
	if err != nil {
		return "err"
	}
	t := strings.TrimSpace(text)
	if t == "denied" || strings.HasPrefix(t, "denied by user") {
		return "err"
	}
	if code, ok := parseHTTPStatus(text); ok && code >= 400 {
		return "err"
	}
	if hasExitStatusFail(text) {
		return "err"
	}
	if isDiffResult(text) {
		return "diff"
	}
	return "ok"
}

func hasExitStatusFail(text string) bool {
	for _, ln := range strings.Split(text, "\n") {
		s := strings.TrimSpace(ln)
		if !strings.HasPrefix(s, "exit status ") {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(s[len("exit status "):], "%d", &n); err != nil {
			continue
		}
		if n != 0 {
			return true
		}
	}
	return false
}

func parseHTTPStatus(text string) (int, bool) {
	s := strings.TrimLeft(text, " \t\r\n")
	if !strings.HasPrefix(s, "status ") {
		return 0, false
	}
	var code int
	if _, err := fmt.Sscanf(s[len("status "):], "%d", &code); err != nil {
		return 0, false
	}
	return code, true
}

func capToolDisplay(tool, kind, text string) string {
	limit := 240
	if kind == "diff" || tool == "shell" {
		limit = 8192
	}
	if len(text) > limit {
		return text[:limit] + "…"
	}
	return text
}
