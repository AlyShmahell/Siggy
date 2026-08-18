package prompt

import (
	"os"
	"path/filepath"
	"strings"

	"siggy/src/internal/harness"
	"siggy/src/internal/tools"
)

func System(h *harness.Harness, reg *tools.Registry, extra string) string {
	var b strings.Builder
	b.WriteString("You are Siggy, a local coding agent. You work only inside the given workspace.\n")
	b.WriteString("Be concrete. Prefer small, reversible edits. Use tools instead of guessing file contents.\n")
	b.WriteString("Never exfiltrate secrets. Never run destructive commands unless the user asked.\n")
	b.WriteString("When a task is done, stop calling tools and summarize what changed.\n\n")
	b.WriteString("Workspace: ")
	b.WriteString(h.Workspace.Root)
	b.WriteString("\nMode: ")
	b.WriteString(string(h.Mode))
	b.WriteString("\n")
	if h.Mode == harness.ModePlan {
		b.WriteString("Plan mode is on: you may only read, search, and list. Do not write, edit, fetch, or run shell.\n")
	}
	if reg != nil {
		b.WriteString("\nTools:\n")
		for _, t := range reg.List() {
			b.WriteString("- ")
			b.WriteString(t.Name())
			b.WriteString(" [")
			b.WriteString(string(t.Risk()))
			b.WriteString("]: ")
			b.WriteString(t.Description())
			b.WriteString("\n")
		}
	}
	if extra != "" {
		b.WriteString("\n")
		b.WriteString(extra)
		b.WriteString("\n")
	}
	if project := projectInstructions(h.Workspace.Root); project != "" {
		b.WriteString("\nProject instructions:\n")
		b.WriteString(project)
		b.WriteString("\n")
	}
	return b.String()
}

func projectInstructions(root string) string {
	candidates := []string{
		filepath.Join(root, ".siggy", "instructions.md"),
		filepath.Join(root, "AGENTS.md"),
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		text := strings.TrimSpace(string(data))
		if text != "" {
			if len(text) > 16*1024 {
				text = text[:16*1024] + "\n…[truncated]…"
			}
			return text
		}
	}
	return ""
}
