package prompt

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"siggy/src/internal/harness"
	"siggy/src/internal/tools"
)

const shareDir = "/usr/share/siggy/prompts"

func DefaultSource() string {
	if p := os.Getenv("SIGGY_PROMPTS"); p != "" {
		return p
	}
	if st, err := os.Stat(shareDir); err == nil && st.IsDir() {
		return shareDir
	}
	return ""
}

func DestDir(home string) string {
	return filepath.Join(home, "prompts")
}

func Seed(src, dest string) error {
	if src == "" || dest == "" {
		return nil
	}
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("prompt source %s is not a directory", src)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if st, err := os.Stat(target); err == nil && !st.IsDir() {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return nil
}

func System(h *harness.Harness, reg *tools.Registry, extra string) string {
	dir := DestDir(h.Home)
	body := readPrompt(filepath.Join(dir, "system.md"))
	toolsBlock := formatTools(reg)
	if body == "" {
		var b strings.Builder
		fmt.Fprintf(&b, "Workspace: %s\nMode: %s\n", h.Workspace.Root, h.Mode)
		if toolsBlock != "" {
			b.WriteString("\n")
			b.WriteString(toolsBlock)
			b.WriteString("\n")
		}
		body = b.String()
	} else {
		body = strings.ReplaceAll(body, "{{workspace}}", h.Workspace.Root)
		body = strings.ReplaceAll(body, "{{mode}}", string(h.Mode))
		body = strings.ReplaceAll(body, "{{tools}}", toolsBlock)
	}
	if h.Mode == harness.ModePlan {
		if plan := strings.TrimSpace(readPrompt(filepath.Join(dir, "plan.md"))); plan != "" {
			body = strings.TrimRight(body, "\n") + "\n\n" + plan + "\n"
		}
	}
	if extra != "" {
		body = strings.TrimRight(body, "\n") + "\n\n" + extra + "\n"
	}
	body = appendCapped(body, "User instructions", userInstructions(h.Home), 16*1024)
	body = appendCapped(body, "Project instructions", projectInstructions(h.Workspace.Root), 16*1024)
	body = appendCapped(body, "Local instructions", localInstructions(h.Workspace.Root), 16*1024)
	if h.Workspace != nil {
		mem := strings.TrimSpace(readPrompt(filepath.Join(harness.MemoryDir(h.Home, harness.HashWorkspace(h.Workspace.Root)), "MEMORY.md")))
		if mem != "" {
			body = appendCapped(body, "Memory index (untrusted notes; read_file to load a topic)", mem, 8*1024)
		}
	}
	if len(body) > 32*1024 {
		body = body[:32*1024] + "\n…[truncated]…"
	}
	return body
}

func appendCapped(body, title, text string, capn int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return body
	}
	if len(text) > capn {
		text = text[:capn] + "\n…[truncated]…"
	}
	return strings.TrimRight(body, "\n") + "\n\n" + title + ":\n" + text + "\n"
}

func userInstructions(home string) string {
	return strings.TrimSpace(readPrompt(filepath.Join(home, "instructions.md")))
}

func localInstructions(root string) string {
	return strings.TrimSpace(readPrompt(filepath.Join(root, ".siggy", "instructions.local.md")))
}

func Compact(home string) string {
	return strings.TrimSpace(readPrompt(filepath.Join(DestDir(home), "compact.md")))
}

func Agent(home, name string) string {
	return strings.TrimSpace(readPrompt(filepath.Join(DestDir(home), "agents", name+".md")))
}

func formatTools(reg *tools.Registry) string {
	if reg == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("Tools:\n")
	for _, t := range reg.List() {
		fmt.Fprintf(&b, "- %s [%s]: %s\n", t.Name(), t.Risk(), t.Description())
	}
	return strings.TrimRight(b.String(), "\n")
}

func readPrompt(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
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
