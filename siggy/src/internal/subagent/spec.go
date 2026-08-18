package subagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"siggy/src/internal/prompt"
)

type Spec struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Tools       []string `toml:"tools"`
	Prompt      string   `toml:"prompt"`
}

func Builtins() []Spec {
	return []Spec{
		{
			Name:        "explore",
			Description: "Read-only scout that maps a codebase and reports findings.",
			Tools:       []string{"read_file", "read_pdf", "list_dir", "glob", "grep"},
		},
		{
			Name:        "implement",
			Description: "Makes focused code changes for a single task.",
			Tools:       []string{"read_file", "read_pdf", "write_file", "edit_file", "list_dir", "glob", "grep", "shell", "todo_write"},
		},
		{
			Name:        "review",
			Description: "Reviews local files and reports risks.",
			Tools:       []string{"read_file", "read_pdf", "list_dir", "glob", "grep"},
		},
	}
}

func LoadProject(workspace string) ([]Spec, error) {
	dir := filepath.Join(workspace, ".siggy", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var specs []Spec
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var s Spec
		if err := toml.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if s.Name == "" {
			s.Name = strings.TrimSuffix(e.Name(), ".toml")
		}
		specs = append(specs, s)
	}
	return specs, nil
}

func Resolve(workspace, home, name string) (Spec, error) {
	for _, s := range Builtins() {
		if s.Name == name {
			return withAgentPrompt(home, s), nil
		}
	}
	proj, err := LoadProject(workspace)
	if err != nil {
		return Spec{}, err
	}
	for _, s := range proj {
		if s.Name == name {
			return withAgentPrompt(home, s), nil
		}
	}
	return Spec{}, fmt.Errorf("unknown subagent %q", name)
}

func withAgentPrompt(home string, s Spec) Spec {
	if strings.TrimSpace(s.Prompt) != "" {
		return s
	}
	s.Prompt = prompt.Agent(home, s.Name)
	return s
}
