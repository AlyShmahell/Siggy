package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"siggy/src/internal/harness"
)

const (
	ddgLite       = "https://lite.duckduckgo.com/lite/"
	searchRawCap  = 64 * 1024
	searchTextCap = 32 * 1024
)

type searchTool struct {
	client   *http.Client
	endpoint string
}

func NewSearch() Tool {
	return &searchTool{
		client:   &http.Client{Timeout: 20 * time.Second},
		endpoint: ddgLite,
	}
}

func (t *searchTool) Name() string { return "web_search" }
func (t *searchTool) Description() string {
	return "Search the public web. Returns stripped search-page text (untrusted). Use web_fetch to read a result URL."
}
func (t *searchTool) Risk() harness.Risk { return harness.RiskNetwork }
func (t *searchTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"query": map[string]any{"type": "string"},
	}, []string{"query"})
}

func (t *searchTool) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	args, err := decode[struct {
		Query string `json:"query"`
	}](raw)
	if err != nil {
		return "", err
	}
	q := strings.TrimSpace(args.Query)
	if q == "" {
		return "", fmt.Errorf("query is required")
	}
	endpoint := t.endpoint
	if endpoint == "" {
		endpoint = ddgLite
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	vals := u.Query()
	vals.Set("q", q)
	u.RawQuery = vals.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "siggy/0.1")
	client := t.client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, searchRawCap))
	if err != nil {
		return "", err
	}
	text := stripSearchHTML(string(body))
	if len(text) > searchTextCap {
		text = text[:searchTextCap]
	}
	return fmt.Sprintf("status %d\n%s", resp.StatusCode, text), nil
}

var (
	reScriptStyle = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(script|style)>`)
	reHrefDouble  = regexp.MustCompile(`(?is)<a\b[^>]*href="([^"]+)"[^>]*>`)
	reHrefSingle  = regexp.MustCompile(`(?is)<a\b[^>]*href='([^']+)'[^>]*>`)
	reTags        = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpace       = regexp.MustCompile(`\s+`)
)

func stripSearchHTML(s string) string {
	s = reScriptStyle.ReplaceAllString(s, " ")
	s = reHrefDouble.ReplaceAllString(s, " $1 ")
	s = reHrefSingle.ReplaceAllString(s, " $1 ")
	s = reTags.ReplaceAllString(s, " ")
	s = unescapeSearchEntities(s)
	s = reSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func unescapeSearchEntities(s string) string {
	return strings.NewReplacer(
		"&nbsp;", " ",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&apos;", "'",
	).Replace(s)
}
