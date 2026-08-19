package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const ProtocolOpenAI = "openai"

type MCPServer struct {
	Name    string   `toml:"name"`
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
	Env     []string `toml:"env"`
}

type Provider struct {
	Name      string   `toml:"name"`
	URL       string   `toml:"url"`
	APIKey    string   `toml:"api_key"`
	Models    []string `toml:"models"`
	Protocols []string `toml:"protocols"`
}

type File struct {
	ActiveProvider string      `toml:"active_provider"`
	Model          string      `toml:"model"`
	BaseURL        string      `toml:"base_url"`
	APIKey         string      `toml:"api_key"`
	Workspace      string      `toml:"workspace"`
	ContextWindow  int         `toml:"context_window"`
	Providers      []Provider  `toml:"providers"`
	MCP            []MCPServer `toml:"mcp"`
}

type Config struct {
	Home           string
	Workspace      string
	AutoApprove    bool
	Mode           string
	ActiveProvider string
	Model          string
	ContextWindow  int
	Providers      []Provider
	MCP            []MCPServer
}

func DefaultHome() string {
	if h := os.Getenv("SIGGY_HOME"); h != "" {
		return h
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".siggy")
	}
	return filepath.Join(os.TempDir(), "siggy-home")
}

func ConfigPath(home string) string {
	return filepath.Join(home, "config.toml")
}

func Load() (*Config, error) {
	home := DefaultHome()
	if err := os.MkdirAll(home, 0o755); err != nil {
		return nil, fmt.Errorf("create siggy home: %w", err)
	}

	cwd := mustAbs(".")
	cfg := &Config{
		Home:      home,
		Workspace: cwd,
		Mode:      "act",
	}

	if env := strings.TrimSpace(os.Getenv("SIGGY_WORKSPACE")); env != "" {
		ws, err := ResolveWorkspace(env)
		if err != nil {
			return nil, err
		}
		cfg.Workspace = ws
	}

	path := ConfigPath(home)
	var file File
	if raw, err := os.ReadFile(path); err == nil {
		if err := toml.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		cfg.MCP = file.MCP
		if file.Workspace != "" && os.Getenv("SIGGY_WORKSPACE") == "" {
			if st, err := os.Stat(file.Workspace); err == nil && st.IsDir() {
				cfg.Workspace = file.Workspace
			}
		}
		cfg.Providers = file.Providers
		cfg.ActiveProvider = file.ActiveProvider
		cfg.Model = file.Model
		if file.ContextWindow > 0 {
			cfg.ContextWindow = file.ContextWindow
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if len(cfg.Providers) == 0 {
		cfg.Providers = []Provider{envProvider(file)}
		cfg.ActiveProvider = cfg.Providers[0].Name
		if cfg.Model == "" {
			cfg.Model = firstNonEmpty(os.Getenv("OPENAI_MODEL"), firstModel(cfg.Providers[0]), "gpt-4.1")
		}
	}

	if os.Getenv("OPENAI_MODEL") != "" {
		cfg.Model = os.Getenv("OPENAI_MODEL")
	}

	if p := cfg.Provider(cfg.ActiveProvider); p == nil && len(cfg.Providers) > 0 {
		cfg.ActiveProvider = cfg.Providers[0].Name
	}
	if cfg.Model == "" {
		if p := cfg.Active(); p.Name != "" {
			cfg.Model = firstModel(p)
		}
	}

	abs, err := filepath.Abs(cfg.Workspace)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	cfg.Workspace = abs
	if cfg.ContextWindow <= 0 {
		cfg.ContextWindow = 128000
	}
	return cfg, nil
}

func ResolveWorkspace(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("workspace path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("workspace: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("workspace: %w", err)
	}
	if st.IsDir() {
		return abs, nil
	}
	return filepath.Dir(abs), nil
}

func OverrideWorkspace(cfg *Config, cli string) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if os.Getenv("SIGGY_WORKSPACE") != "" {
		return nil
	}
	cli = strings.TrimSpace(cli)
	if cli == "" {
		return nil
	}
	ws, err := ResolveWorkspace(cli)
	if err != nil {
		return err
	}
	cfg.Workspace = ws
	return nil
}

func envProvider(file File) Provider {
	url := firstNonEmpty(os.Getenv("OPENAI_BASE_URL"), file.BaseURL, "https://api.openai.com/v1")
	key := firstNonEmpty(os.Getenv("OPENAI_API_KEY"), file.APIKey)
	model := firstNonEmpty(os.Getenv("OPENAI_MODEL"), file.Model, "gpt-4.1")
	return Provider{
		Name:      "env",
		URL:       url,
		APIKey:    key,
		Models:    []string{model},
		Protocols: []string{ProtocolOpenAI},
	}
}

func (c *Config) Provider(name string) *Provider {
	for i := range c.Providers {
		if c.Providers[i].Name == name {
			return &c.Providers[i]
		}
	}
	return nil
}

func (c *Config) Active() Provider {
	if p := c.Provider(c.ActiveProvider); p != nil {
		return *p
	}
	if len(c.Providers) > 0 {
		return c.Providers[0]
	}
	return Provider{Name: "env", URL: "https://api.openai.com/v1", Models: []string{"gpt-4.1"}, Protocols: []string{ProtocolOpenAI}}
}

func (c *Config) SetActive(name string) error {
	p := c.Provider(name)
	if p == nil {
		return fmt.Errorf("unknown provider %q", name)
	}
	c.ActiveProvider = name
	if c.Model == "" || !contains(p.Models, c.Model) {
		c.Model = firstModel(*p)
	}
	return nil
}

func (c *Config) Upsert(p Provider) error {
	if err := ValidateProvider(p); err != nil {
		return err
	}
	for i := range c.Providers {
		if c.Providers[i].Name == p.Name {
			c.Providers[i] = p
			return nil
		}
	}
	c.Providers = append(c.Providers, p)
	return nil
}

func ValidateProvider(p Provider) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(p.URL) == "" {
		return fmt.Errorf("url is required")
	}
	var models []string
	for _, m := range p.Models {
		if strings.TrimSpace(m) != "" {
			models = append(models, strings.TrimSpace(m))
		}
	}
	if len(models) == 0 {
		return fmt.Errorf("at least one model is required")
	}
	if !contains(p.Protocols, ProtocolOpenAI) {
		return fmt.Errorf("protocol %q is required", ProtocolOpenAI)
	}
	for _, proto := range p.Protocols {
		if proto != ProtocolOpenAI {
			return fmt.Errorf("unsupported protocol %q", proto)
		}
	}
	return nil
}

func (c *Config) Save() error {
	if err := os.MkdirAll(c.Home, 0o755); err != nil {
		return err
	}
	file := File{
		ActiveProvider: c.ActiveProvider,
		Model:          c.Model,
		Workspace:      c.Workspace,
		ContextWindow:  c.ContextWindow,
		Providers:      c.Providers,
		MCP:            c.MCP,
	}
	f, err := os.Create(ConfigPath(c.Home))
	if err != nil {
		return err
	}
	defer f.Close()
	enc := toml.NewEncoder(f)
	return enc.Encode(file)
}

func firstModel(p Provider) string {
	for _, m := range p.Models {
		if strings.TrimSpace(m) != "" {
			return m
		}
	}
	return "gpt-4.1"
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func mustAbs(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func MaskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return "••••"
	}
	return "••••" + key[len(key)-4:]
}
