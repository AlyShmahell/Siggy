package utils

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
)

const (
	FetchRawCap  = 1 * 1024 * 1024
	TextBodyCap  = 12 * 1024
	htmlSniffLen = 1024
)

var (
	reScriptStyle = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(script|style)>`)
	reHrefDouble  = regexp.MustCompile(`(?is)<a\b[^>]*href="([^"]+)"[^>]*>`)
	reHrefSingle  = regexp.MustCompile(`(?is)<a\b[^>]*href='([^']+)'[^>]*>`)
	reTags        = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpace       = regexp.MustCompile(`\s+`)
	reHTMLPrefix  = regexp.MustCompile(`(?i)^\s*(<!doctype\s+html|<html\b)`)
	reJSONPrefix  = regexp.MustCompile(`(?s)^\s*[{\[]`)
	reXMLPrefix   = regexp.MustCompile(`(?i)^\s*<\?xml`)
)

func HTMLToText(html, baseURL string) string {
	var opts []converter.ConvertOptionFunc
	if domain := markdownDomain(baseURL); domain != "" {
		opts = append(opts, converter.WithDomain(domain))
	}
	md, err := htmltomarkdown.ConvertString(html, opts...)
	if err != nil {
		return stripSearchHTML(html)
	}
	md = strings.TrimSpace(md)
	if md == "" {
		return stripSearchHTML(html)
	}
	return md
}

func markdownDomain(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	return u.Scheme + "://" + u.Host
}

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

func CapTextMarked(s string, n int) string {
	total := len(s)
	if n <= 0 || total <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n[kept %d of %d chars]", n, total)
}

func IsBinaryContentType(ct string) bool {
	ct = strings.ToLower(ct)
	if ct == "" {
		return false
	}
	if strings.Contains(ct, "json") || strings.Contains(ct, "xml") || strings.Contains(ct, "text") {
		return false
	}
	return true
}

func ShouldStripHTML(ct string, body []byte) bool {
	ct = strings.ToLower(ct)
	if strings.Contains(ct, "html") {
		return true
	}
	vague := ct == "" || strings.Contains(ct, "text/plain")
	if !vague {
		return false
	}
	prefix := body
	if len(prefix) > htmlSniffLen {
		prefix = prefix[:htmlSniffLen]
	}
	s := string(prefix)
	if reJSONPrefix.MatchString(s) || reXMLPrefix.MatchString(s) {
		return false
	}
	return reHTMLPrefix.MatchString(s)
}
