package tools

import (
	"fmt"
	"strings"
)

func formatEditHunk(rel, before, old, new string) string {
	idx := strings.Index(before, old)
	if idx < 0 {
		return "edited " + rel
	}
	lineStart := strings.LastIndex(before[:idx], "\n") + 1
	end := idx + len(old)
	lineEnd := strings.Index(before[end:], "\n")
	if lineEnd < 0 {
		lineEnd = len(before)
	} else {
		lineEnd += end
	}
	prefix := before[lineStart:idx]
	suffix := before[end:lineEnd]
	oldBlock := prefix + old + suffix
	newBlock := prefix + new + suffix
	startLine := strings.Count(before[:lineStart], "\n") + 1

	var b strings.Builder
	fmt.Fprintf(&b, "edited %s\n", rel)
	lines := strings.Split(before, "\n")
	if startLine > 1 {
		fmt.Fprintf(&b, "  %4d | %s\n", startLine-1, lines[startLine-2])
	}
	oldLs := strings.Split(oldBlock, "\n")
	newLs := strings.Split(newBlock, "\n")
	for i, ln := range oldLs {
		fmt.Fprintf(&b, "- %4d | %s\n", startLine+i, ln)
	}
	for i, ln := range newLs {
		fmt.Fprintf(&b, "+ %4d | %s\n", startLine+i, ln)
	}
	afterN := startLine + len(oldLs)
	if afterN >= 1 && afterN <= len(lines) {
		fmt.Fprintf(&b, "  %4d | %s\n", afterN, lines[afterN-1])
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatWriteHunk(rel, content string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "wrote %s\n", rel)
	if content == "" {
		return strings.TrimRight(b.String(), "\n")
	}
	for i, ln := range strings.Split(content, "\n") {
		fmt.Fprintf(&b, "+ %4d | %s\n", i+1, ln)
	}
	return strings.TrimRight(b.String(), "\n")
}
