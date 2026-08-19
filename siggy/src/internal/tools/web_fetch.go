package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"siggy/src/internal/harness"
	"siggy/src/internal/tools/utils"
)

type fetchTool struct {
	h      *harness.Harness
	client *http.Client
}

func NewFetch(h *harness.Harness) Tool {
	return &fetchTool{h: h, client: &http.Client{Timeout: 20 * time.Second}}
}

func (t *fetchTool) Name() string { return "web_fetch" }
func (t *fetchTool) Description() string {
	return "Fetch an http(s) URL and return a truncated text body."
}
func (t *fetchTool) Risk() harness.Risk { return harness.RiskNetwork }
func (t *fetchTool) RiskFor(raw json.RawMessage) harness.Risk {
	args, err := decode[struct {
		Path string `json:"path"`
	}](raw)
	if err == nil && strings.TrimSpace(args.Path) != "" {
		return harness.RiskWrite
	}
	return harness.RiskNetwork
}
func (t *fetchTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"url":  map[string]any{"type": "string"},
		"path": map[string]any{"type": "string", "description": "Workspace path to save the raw response body"},
		"html": map[string]any{"type": "boolean", "description": "If true, also return capped raw HTML"},
	}, []string{"url"})
}

func (t *fetchTool) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		URL  string `json:"url"`
		Path string `json:"path"`
		HTML bool   `json:"html"`
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, utils.FetchRawCap))
	if err != nil {
		return "", err
	}
	ct := resp.Header.Get("Content-Type")
	base := u.String()
	if resp.Request != nil && resp.Request.URL != nil {
		base = resp.Request.URL.String()
	}

	var out strings.Builder
	fmt.Fprintf(&out, "status %d\n", resp.StatusCode)

	html := utils.ShouldStripHTML(ct, body)
	var md string
	switch {
	case utils.IsBinaryContentType(ct):
		fmt.Fprintf(&out, "content-type %s (%d bytes)", ct, len(body))
	case html:
		md = utils.CapTextMarked(utils.HTMLToText(string(body), base), utils.TextBodyCap)
		if t.h != nil {
			utils.WritePageCache(t.h.Home, "fetch", base, body, md)
		}
		out.WriteString(md)
	default:
		out.WriteString(utils.CapTextMarked(string(body), utils.TextBodyCap))
	}

	if rel := strings.TrimSpace(args.Path); rel != "" {
		if t.h == nil {
			return "", fmt.Errorf("path requires a workspace")
		}
		abs, err := t.h.Workspace.Resolve(rel)
		if err != nil {
			return "", err
		}
		utils.SnapshotFile(t.h, rel, abs)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(abs, body, 0o644); err != nil {
			return "", err
		}
		fmt.Fprintf(&out, "\nsaved %s (%s, %d bytes)", t.h.Workspace.Rel(abs), ct, len(body))
	}

	if args.HTML && html {
		out.WriteString("\n")
		out.WriteString(utils.CapTextMarked(string(body), utils.TextBodyCap))
	}
	return out.String(), nil
}
