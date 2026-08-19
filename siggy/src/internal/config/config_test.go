package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvSeedsProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIGGY_HOME", home)
	t.Setenv("OPENAI_MODEL", "from-env")
	t.Setenv("OPENAI_BASE_URL", "http://env.example/v1")
	t.Setenv("OPENAI_API_KEY", "sk-env")
	t.Setenv("SIGGY_WORKSPACE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveProvider != "env" {
		t.Fatalf("active = %q", cfg.ActiveProvider)
	}
	p := cfg.Active()
	if p.URL != "http://env.example/v1" || p.APIKey != "sk-env" {
		t.Fatalf("provider = %#v", p)
	}
	if cfg.Model != "from-env" {
		t.Fatalf("model = %q", cfg.Model)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIGGY_HOME", home)
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("SIGGY_WORKSPACE", home)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	p := Provider{
		Name:      "work",
		URL:       "https://api.example/v1",
		APIKey:    "sk-secret",
		Models:    []string{"gpt-4.1", "mini"},
		Protocols: []string{ProtocolOpenAI},
	}
	if err := cfg.Upsert(p); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetActive("work"); err != nil {
		t.Fatal(err)
	}
	cfg.Model = "mini"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	again, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	got := again.Provider("work")
	if got == nil || got.URL != p.URL || got.APIKey != p.APIKey || len(got.Models) != 2 {
		t.Fatalf("round trip = %#v", got)
	}
	if again.ActiveProvider != "work" || again.Model != "mini" {
		t.Fatalf("active=%q model=%q", again.ActiveProvider, again.Model)
	}
	if again.ContextWindow != 128000 {
		t.Fatalf("context window default = %d", again.ContextWindow)
	}
	if _, err := os.Stat(filepath.Join(home, "config.toml")); err != nil {
		t.Fatal(err)
	}
}

func TestValidateProvider(t *testing.T) {
	err := ValidateProvider(Provider{Name: "x", URL: "http://x", Models: []string{"m"}, Protocols: []string{ProtocolOpenAI}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProvider(Provider{Name: "", URL: "http://x", Models: []string{"m"}, Protocols: []string{ProtocolOpenAI}}); err == nil {
		t.Fatal("expected name error")
	}
	if err := ValidateProvider(Provider{Name: "x", URL: "http://x", Models: []string{"m"}, Protocols: []string{"anthropic"}}); err == nil {
		t.Fatal("expected protocol error")
	}
}

func TestMaskKey(t *testing.T) {
	if MaskKey("sk-abcdefgh") != "••••efgh" {
		t.Fatalf("mask = %q", MaskKey("sk-abcdefgh"))
	}
}

func TestLoadEnvOverridesModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIGGY_HOME", home)
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "")
	body := []byte("active_provider = \"work\"\nmodel = \"from-file\"\n\n[[providers]]\nname = \"work\"\nurl = \"http://file.example/v1\"\nmodels = [\"from-file\"]\nprotocols = [\"openai\"]\n")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "from-file" {
		t.Fatalf("model = %q", cfg.Model)
	}
	t.Setenv("OPENAI_MODEL", "from-env")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "from-env" {
		t.Fatalf("env should win, got %q", cfg.Model)
	}
}

func TestResolveWorkspaceFileUsesParent(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "note.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveWorkspace(file)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("file workspace = %q want %q", got, root)
	}
	got, err = ResolveWorkspace(root)
	if err != nil || got != root {
		t.Fatalf("dir workspace = %q %v", got, err)
	}
	if _, err := ResolveWorkspace(filepath.Join(root, "missing")); err == nil {
		t.Fatal("expected missing path error")
	}
}

func TestOverrideWorkspaceArgBeatsToml(t *testing.T) {
	home := t.TempDir()
	tomlWS := t.TempDir()
	cliWS := t.TempDir()
	t.Setenv("SIGGY_HOME", home)
	t.Setenv("SIGGY_WORKSPACE", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "")
	body := []byte(fmt.Sprintf("workspace = %q\n", tomlWS))
	if err := os.WriteFile(filepath.Join(home, "config.toml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Workspace != tomlWS {
		t.Fatalf("toml workspace = %q", cfg.Workspace)
	}
	if err := OverrideWorkspace(cfg, cliWS); err != nil {
		t.Fatal(err)
	}
	if cfg.Workspace != cliWS {
		t.Fatalf("cli should win, got %q", cfg.Workspace)
	}
}

func TestOverrideWorkspaceEnvBeatsArg(t *testing.T) {
	home := t.TempDir()
	envWS := t.TempDir()
	cliWS := t.TempDir()
	t.Setenv("SIGGY_HOME", home)
	t.Setenv("SIGGY_WORKSPACE", envWS)
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Workspace != envWS {
		t.Fatalf("env workspace = %q", cfg.Workspace)
	}
	if err := OverrideWorkspace(cfg, cliWS); err != nil {
		t.Fatal(err)
	}
	if cfg.Workspace != envWS {
		t.Fatalf("env should keep, got %q", cfg.Workspace)
	}
}

func TestLoadContextWindow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIGGY_HOME", home)
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "")
	body := []byte("context_window = 64000\n")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ContextWindow != 64000 {
		t.Fatalf("context_window = %d", cfg.ContextWindow)
	}
}
