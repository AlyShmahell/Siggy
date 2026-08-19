package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"siggy/src/internal/harness"
	"siggy/src/internal/llm"
)

const (
	defaultPDFPages = 4
	maxPDFPages     = 8
	pdfRenderDPI    = "110"
)

type pdfTool struct {
	h *harness.Harness
}

func NewReadPDF(h *harness.Harness) Tool { return &pdfTool{h: h} }

func (t *pdfTool) Name() string { return "pdf_read" }
func (t *pdfTool) Description() string {
	return "Rasterize PDF pages to images for a vision-capable model. Use for papers, figures, tables, and equations. pages is like \"1-4\" or \"3,7\" (default first 4, max 8)."
}
func (t *pdfTool) Risk() harness.Risk { return harness.RiskRead }
func (t *pdfTool) Schema() json.RawMessage {
	return objectSchema(map[string]any{
		"path":  map[string]any{"type": "string", "description": "Workspace-relative PDF path"},
		"pages": map[string]any{"type": "string", "description": "Page range, e.g. 1-4 or 3,7"},
	}, []string{"path"})
}

func (t *pdfTool) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	text, _, err := t.RunVisual(ctx, raw)
	return text, err
}

func (t *pdfTool) RunVisual(ctx context.Context, raw json.RawMessage) (string, []llm.Part, error) {
	args, err := decode[struct {
		Path  string `json:"path"`
		Pages string `json:"pages"`
	}](raw)
	if err != nil {
		return "", nil, err
	}
	path, err := t.h.Workspace.Resolve(args.Path)
	if err != nil {
		return "", nil, err
	}
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return "", nil, fmt.Errorf("pdftoppm not found; rebuild the Siggy image")
	}
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		return "", nil, fmt.Errorf("pdfinfo not found; rebuild the Siggy image")
	}
	count, err := pdfPageCount(ctx, path)
	if err != nil {
		return "", nil, err
	}
	pages, err := parsePages(args.Pages, count)
	if err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp("", "siggy-pdf-*")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(dir)
	prefix := filepath.Join(dir, "page")
	first, last := pages[0], pages[len(pages)-1]
	contiguous := last-first+1 == len(pages)
	if !contiguous {
		var images []llm.Part
		for _, p := range pages {
			part, err := renderPDFPages(ctx, path, p, p, prefix+"-"+strconv.Itoa(p))
			if err != nil {
				return "", nil, err
			}
			images = append(images, part...)
		}
		return pdfCaption(args.Path, pages, count), images, nil
	}
	images, err := renderPDFPages(ctx, path, first, last, prefix)
	if err != nil {
		return "", nil, err
	}
	return pdfCaption(args.Path, pages, count), images, nil
}

func pdfCaption(path string, pages []int, count int) string {
	return fmt.Sprintf("Rendered pages %s of %s (%d pages). Images attached for a vision model.", formatPages(pages), path, count)
}

func formatPages(pages []int) string {
	if len(pages) == 0 {
		return ""
	}
	var parts []string
	start := pages[0]
	prev := pages[0]
	flush := func() {
		if start == prev {
			parts = append(parts, strconv.Itoa(start))
			return
		}
		parts = append(parts, fmt.Sprintf("%d-%d", start, prev))
	}
	for _, p := range pages[1:] {
		if p == prev+1 {
			prev = p
			continue
		}
		flush()
		start, prev = p, p
	}
	flush()
	return strings.Join(parts, ", ")
}

func parsePages(spec string, pageCount int) ([]int, error) {
	if pageCount < 1 {
		return nil, fmt.Errorf("pdf has no pages")
	}
	spec = strings.TrimSpace(spec)
	if spec == "" {
		n := min(defaultPDFPages, pageCount)
		out := make([]int, n)
		for i := 0; i < n; i++ {
			out[i] = i + 1
		}
		return out, nil
	}
	seen := map[int]bool{}
	var pages []int
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if lo, hi, ok := strings.Cut(part, "-"); ok {
			a, err1 := strconv.Atoi(strings.TrimSpace(lo))
			b, err2 := strconv.Atoi(strings.TrimSpace(hi))
			if err1 != nil || err2 != nil || a < 1 || b < a {
				return nil, fmt.Errorf("invalid pages %q", spec)
			}
			for i := a; i <= b && i <= pageCount; i++ {
				if !seen[i] {
					seen[i] = true
					pages = append(pages, i)
				}
			}
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid pages %q", spec)
		}
		if n > pageCount {
			continue
		}
		if !seen[n] {
			seen[n] = true
			pages = append(pages, n)
		}
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages in range (pdf has %d)", pageCount)
	}
	if len(pages) > maxPDFPages {
		pages = pages[:maxPDFPages]
	}
	return pages, nil
}

var pagesRe = regexp.MustCompile(`(?m)^Pages:\s+(\d+)`)

func pdfPageCount(ctx context.Context, path string) (int, error) {
	cmd := exec.CommandContext(ctx, "pdfinfo", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("pdfinfo: %s", bytes.TrimSpace(out))
	}
	m := pagesRe.FindSubmatch(out)
	if m == nil {
		return 0, fmt.Errorf("pdfinfo: missing page count")
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil || n < 1 {
		return 0, fmt.Errorf("pdfinfo: bad page count")
	}
	return n, nil
}

func renderPDFPages(ctx context.Context, pdf string, first, last int, prefix string) ([]llm.Part, error) {
	cmd := exec.CommandContext(ctx, "pdftoppm", "-jpeg", "-r", pdfRenderDPI, "-f", strconv.Itoa(first), "-l", strconv.Itoa(last), pdf, prefix)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("pdftoppm: %s", msg)
	}
	matches, err := filepath.Glob(prefix + "*.jpg")
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("pdftoppm produced no images")
	}
	sort.Strings(matches)
	var images []llm.Part
	for _, p := range matches {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			continue
		}
		images = append(images, llm.Part{Type: "image", MIME: "image/jpeg", Data: data})
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("pdftoppm produced empty images")
	}
	return images, nil
}
