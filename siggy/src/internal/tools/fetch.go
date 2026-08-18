package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"siggy/src/internal/harness"
)

type fetchTool struct {
	client *http.Client
}

func NewFetch() Tool {
	return &fetchTool{client: &http.Client{Timeout: 20 * time.Second}}
}

func (t *fetchTool) Name() string { return "web_fetch" }
func (t *fetchTool) Description() string {
	return "Fetch an http(s) URL and return a truncated text body."
}
func (t *fetchTool) Risk() harness.Risk { return harness.RiskNetwork }
func (t *fetchTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"url": map[string]any{"type": "string"},
	}, []string{"url"})
}

func (t *fetchTool) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		URL string `json:"url"`
	}](raw)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(args.URL)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("only http(s) URLs are allowed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "siggy/0.1")
	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", err
	}
	text := string(body)
	if !strings.Contains(resp.Header.Get("Content-Type"), "text") &&
		!strings.Contains(resp.Header.Get("Content-Type"), "json") &&
		!strings.Contains(resp.Header.Get("Content-Type"), "xml") {
		text = fmt.Sprintf("status %d content-type %s (%d bytes)", resp.StatusCode, resp.Header.Get("Content-Type"), len(body))
	}
	return fmt.Sprintf("status %d\n%s", resp.StatusCode, text), nil
}
