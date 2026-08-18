package tui

import (
	"strings"
)

func isDiffResult(s string) bool {
	for _, ln := range strings.Split(s, "\n") {
		if (strings.HasPrefix(ln, "- ") || strings.HasPrefix(ln, "+ ")) && strings.Contains(ln, " | ") {
			return true
		}
	}
	return false
}

func renderDiff(s string, inner int) string {
	var b strings.Builder
	for i, ln := range strings.Split(s, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		shown := truncate(ln, inner)
		switch {
		case strings.HasPrefix(ln, "- "):
			b.WriteString(stErr.Render(shown))
		case strings.HasPrefix(ln, "+ "):
			b.WriteString(stAdd.Render(shown))
		default:
			b.WriteString(stMuted.Render(shown))
		}
	}
	return b.String()
}
