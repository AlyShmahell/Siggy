package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"siggy/src/internal/harness"
	"siggy/src/internal/tools"
)

func TestSeedCopiesMissingAndKeepsExisting(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "system.md"), []byte("hello {{workspace}}\n{{mode}}\n{{tools}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Seed(src, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "system.md"))
	if err != nil || !strings.Contains(string(got), "hello") {
		t.Fatalf("copied = %q %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(dest, "system.md"), []byte("kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "system.md"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Seed(src, dest); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(filepath.Join(dest, "system.md"))
	if err != nil || string(got) != "kept\n" {
		t.Fatalf("should keep dest: %q %v", got, err)
	}
}

func TestSystemFillsPlaceholders(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "system.md"), []byte("W={{workspace}} M={{mode}}\n{{tools}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Seed(src, DestDir(home)); err != nil {
		t.Fatal(err)
	}
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	got := System(h, tools.Builtins(h, nil), "")
	if !strings.Contains(got, "W="+root) {
		t.Fatalf("workspace: %q", got)
	}
	if !strings.Contains(got, "M=act") {
		t.Fatalf("mode: %q", got)
	}
	if !strings.Contains(got, "read_pdf") {
		t.Fatalf("tools: %q", got)
	}
	if strings.Contains(got, "{{") {
		t.Fatalf("placeholder left: %q", got)
	}
}

func TestInstructionStackAndMemoryIndex(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "instructions.md"), []byte("user-note"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".siggy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("project-note"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".siggy", "instructions.local.md"), []byte("local-note"), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	mem := harness.MemoryDir(home, harness.HashWorkspace(root))
	if err := os.MkdirAll(mem, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mem, "MEMORY.md"), []byte("- topic.md — remember this"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := System(h, tools.Builtins(h, nil), "")
	for _, want := range []string{"user-note", "project-note", "local-note", "remember this"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}
