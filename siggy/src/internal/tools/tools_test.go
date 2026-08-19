package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"siggy/src/internal/harness"
	"siggy/src/internal/tools/utils"
)

func testHarness(t *testing.T) *harness.Harness {
	t.Helper()
	root := t.TempDir()
	home := t.TempDir()
	h, err := harness.New(root, home, true)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestReadWriteEdit(t *testing.T) {
	h := testHarness(t)
	ctx := context.Background()
	w := NewWrite(h)
	if _, err := w.Run(ctx, json.RawMessage(`{"path":"a.txt","content":"hello world"}`)); err != nil {
		t.Fatal(err)
	}
	r := NewRead(h)
	got, err := r.Run(ctx, json.RawMessage(`{"path":"a.txt"}`))
	if err != nil || !strings.Contains(got, "hello world") || !strings.Contains(got, "1|") {
		t.Fatalf("read = %q %v", got, err)
	}
	e := NewEdit(h)
	if _, err := e.Run(ctx, json.RawMessage(`{"path":"a.txt","old_string":"world","new_string":"siggy"}`)); err != nil {
		t.Fatal(err)
	}
	got, err = r.Run(ctx, json.RawMessage(`{"path":"a.txt"}`))
	if err != nil || !strings.Contains(got, "hello siggy") || !strings.Contains(got, "1|") {
		t.Fatalf("edited = %q %v", got, err)
	}
}

func TestReadDefaultLineCap(t *testing.T) {
	h := testHarness(t)
	lines := make([]string, 2001)
	for i := range lines {
		lines[i] = fmt.Sprintf("L%d", i+1)
	}
	if err := os.WriteFile(filepath.Join(h.Workspace.Root, "big.txt"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := NewRead(h).Run(context.Background(), json.RawMessage(`{"path":"big.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "  2000|L2000") {
		t.Fatalf("missing numbered cap line: %q", got[len(got)-80:])
	}
	if strings.Contains(got, "2001|L2001") {
		t.Fatalf("uncapped read included line 2001: %q", got[len(got)-80:])
	}
	if !strings.Contains(got, "2001 lines") || !strings.Contains(got, "use offset/limit") {
		t.Fatalf("missing remainder note: %q", got[len(got)-120:])
	}
	sliced, err := NewRead(h).Run(context.Background(), json.RawMessage(`{"path":"big.txt","offset":2001,"limit":1}`))
	if err != nil || !strings.Contains(sliced, "2001|L2001") {
		t.Fatalf("offset/limit = %q %v", sliced, err)
	}
}

func TestEditWriteDiffHunks(t *testing.T) {
	h := testHarness(t)
	ctx := context.Background()
	wrote, err := NewWrite(h).Run(ctx, json.RawMessage(`{"path":"a.txt","content":"hello world"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(wrote, "+") || !strings.Contains(wrote, "1 |") {
		t.Fatalf("write hunk = %q", wrote)
	}
	edited, err := NewEdit(h).Run(ctx, json.RawMessage(`{"path":"a.txt","old_string":"world","new_string":"siggy"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(edited, "-") || !strings.Contains(edited, "+") || !strings.Contains(edited, "1 |") {
		t.Fatalf("edit hunk = %q", edited)
	}
}

func TestWriteRejectsEscape(t *testing.T) {
	h := testHarness(t)
	_, err := NewWrite(h).Run(context.Background(), json.RawMessage(`{"path":"../x","content":"no"}`))
	if err == nil {
		t.Fatal("expected escape failure")
	}
}

func TestGlobAndGrep(t *testing.T) {
	h := testHarness(t)
	if err := os.WriteFile(filepath.Join(h.Workspace.Root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := NewGlob(h).Run(context.Background(), json.RawMessage(`{"pattern":"**/*.go"}`))
	if err != nil || !strings.Contains(got, "main.go") {
		t.Fatalf("glob = %q %v", got, err)
	}
	got, err = NewGlob(h).Run(context.Background(), json.RawMessage(`{"path":"."}{"pattern":"**/*.go"}{"pattern":"*.go"}`))
	if err != nil || !strings.Contains(got, "main.go") {
		t.Fatalf("concat glob = %q %v", got, err)
	}
	got, err = NewGrep(h).Run(context.Background(), json.RawMessage(`{"pattern":"package main"}`))
	if err != nil || !strings.Contains(got, "main.go") {
		t.Fatalf("grep = %q %v", got, err)
	}
}

func TestShell(t *testing.T) {
	h := testHarness(t)
	got, err := NewShell(h).Run(context.Background(), json.RawMessage(`{"command":"echo hi"}`))
	if err != nil || !strings.Contains(got, "hi") {
		t.Fatalf("shell = %q %v", got, err)
	}
}

func TestBuiltinsRegister(t *testing.T) {
	h := testHarness(t)
	r := Builtins(h, nil)
	if _, ok := r.Get("file_read"); !ok {
		t.Fatal("missing file_read")
	}
	if _, ok := r.Get("pdf_read"); !ok {
		t.Fatal("missing pdf_read")
	}
	if _, ok := r.Get("web_search"); !ok {
		t.Fatal("missing web_search")
	}
	if _, ok := r.Get("delegate"); ok {
		t.Fatal("delegate should be absent without delegator")
	}
}

func TestToolAliases(t *testing.T) {
	h := testHarness(t)
	r := Builtins(h, nil)
	got, ok := r.Get("read_pdf")
	if !ok || got.Name() != "pdf_read" {
		t.Fatalf("read_pdf alias = %v %v", got, ok)
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatal("expected missing nope")
	}
	for _, s := range r.Specs() {
		if s.Name == "read_pdf" {
			t.Fatal("alias leaked into Specs")
		}
	}
	f := r.Filter([]string{"read_file"})
	if _, ok := f.Get("file_read"); !ok {
		t.Fatal("filter missed file_read via alias")
	}
	if _, ok := f.Get("pdf_read"); ok {
		t.Fatal("filter kept unrelated tool")
	}
}

func TestWebSearch(t *testing.T) {
	var gotQ string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQ = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><script>secret()</script><a href="https://example.com/page">Example &amp; Co</a></html>`)
	}))
	defer srv.Close()
	tool := &searchTool{client: srv.Client(), endpoint: srv.URL}
	got, err := tool.Run(context.Background(), json.RawMessage(`{"query":"golang html"}`))
	if err != nil {
		t.Fatal(err)
	}
	if gotQ != "golang html" {
		t.Fatalf("q = %q", gotQ)
	}
	if !strings.Contains(got, "status 200") {
		t.Fatalf("missing status: %q", got)
	}
	if !strings.Contains(got, "https://example.com/page") || !strings.Contains(got, "Example & Co") {
		t.Fatalf("missing visible text/url: %q", got)
	}
	if strings.Contains(got, "<a") || strings.Contains(got, "<script") {
		t.Fatalf("tags leaked: %q", got)
	}
	if _, err := tool.Run(context.Background(), json.RawMessage(`{"query":"  "}`)); err == nil {
		t.Fatal("expected empty query error")
	}
}

func TestWebFetchStripsHTMLKeepsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><script>secret()</script><a href="https://example.com/x">Hi &amp; Co</a></html>`)
		case "/img":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><body><img src="/x.png" alt="duck"></body></html>`)
		case "/huge":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<html><body><p>%s</p></body></html>`, strings.Repeat("word ", 8000))
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"ok":true}`)
		case "/plainhtml":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "<html><body>visible</body></html>")
		case "/bin":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47})
		case "/big":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(strings.Repeat("a", utils.TextBodyCap+50)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	h := testHarness(t)
	tool := NewFetch(h)
	ctx := context.Background()

	html, err := tool.Run(ctx, json.RawMessage(fmt.Sprintf(`{"url":%q}`, srv.URL+"/page")))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script") || strings.Contains(html, "<a") {
		t.Fatalf("html tags leaked: %q", html)
	}
	if !strings.Contains(html, "https://example.com/x") || !strings.Contains(html, "Hi & Co") {
		t.Fatalf("stripped html missing text: %q", html)
	}

	img, err := tool.Run(ctx, json.RawMessage(fmt.Sprintf(`{"url":%q}`, srv.URL+"/img")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(img, srv.URL+"/x.png") {
		t.Fatalf("relative img src: %q", img)
	}

	huge, err := tool.Run(ctx, json.RawMessage(fmt.Sprintf(`{"url":%q}`, srv.URL+"/huge")))
	if err != nil {
		t.Fatal(err)
	}
	hugeBody := strings.TrimPrefix(huge, "status 200\n")
	if !strings.Contains(hugeBody, fmt.Sprintf("[kept %d of ", utils.TextBodyCap)) {
		t.Fatalf("missing kept marker: %q", hugeBody)
	}
	if !strings.Contains(hugeBody, " chars]") {
		t.Fatalf("missing chars marker: %q", hugeBody)
	}

	js, err := tool.Run(ctx, json.RawMessage(fmt.Sprintf(`{"url":%q}`, srv.URL+"/json")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, `{"ok":true}`) {
		t.Fatalf("json not raw: %q", js)
	}

	plain, err := tool.Run(ctx, json.RawMessage(fmt.Sprintf(`{"url":%q}`, srv.URL+"/plainhtml")))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "<html") || !strings.Contains(plain, "visible") {
		t.Fatalf("plain html sniff: %q", plain)
	}

	bin, err := tool.Run(ctx, json.RawMessage(fmt.Sprintf(`{"url":%q}`, srv.URL+"/bin")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bin, "image/png") || strings.Contains(bin, "\n\x89") {
		t.Fatalf("binary: %q", bin)
	}

	big, err := tool.Run(ctx, json.RawMessage(fmt.Sprintf(`{"url":%q}`, srv.URL+"/big")))
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimPrefix(big, "status 200\n")
	wantMark := fmt.Sprintf("[kept %d of %d chars]", utils.TextBodyCap, utils.TextBodyCap+50)
	if !strings.HasPrefix(body, strings.Repeat("a", utils.TextBodyCap)) || !strings.Contains(body, wantMark) {
		t.Fatalf("cap marked = %q want %q", body, wantMark)
	}
}

func TestWebFetchCacheNoCheckpoint(t *testing.T) {
	h := testHarness(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><p>cached page</p></body></html>`)
	}))
	defer srv.Close()
	got, err := NewFetch(h).Run(context.Background(), json.RawMessage(fmt.Sprintf(`{"url":%q}`, srv.URL)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "<html") {
		t.Fatalf("html dumped: %q", got)
	}
	if !strings.Contains(got, "cached page") {
		t.Fatalf("missing markdown: %q", got)
	}
	ents, err := os.ReadDir(filepath.Join(h.Home, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	var html, md bool
	for _, e := range ents {
		switch {
		case strings.HasPrefix(e.Name(), "fetch-") && strings.HasSuffix(e.Name(), ".html"):
			html = true
		case strings.HasPrefix(e.Name(), "fetch-") && strings.HasSuffix(e.Name(), ".md"):
			md = true
		}
	}
	if !html || !md {
		t.Fatalf("cache files = %v", ents)
	}
	for _, r := range h.Session.Records() {
		if r.Type == "checkpoint" {
			t.Fatalf("cache wrote checkpoint: %#v", r)
		}
	}
}

func TestWebFetchHTMLOption(t *testing.T) {
	h := testHarness(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>src</body></html>`)
	}))
	defer srv.Close()
	got, err := NewFetch(h).Run(context.Background(), json.RawMessage(fmt.Sprintf(`{"url":%q,"html":true}`, srv.URL)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "<html") {
		t.Fatalf("expected capped html: %q", got)
	}
}

func TestWebFetchPathPNGCheckpoint(t *testing.T) {
	h := testHarness(t)
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer srv.Close()
	got, err := NewFetch(h).Run(context.Background(), json.RawMessage(fmt.Sprintf(`{"url":%q,"path":"out.png"}`, srv.URL)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "saved out.png") {
		t.Fatalf("missing saved: %q", got)
	}
	data, err := os.ReadFile(filepath.Join(h.Workspace.Root, "out.png"))
	if err != nil || string(data[:4]) != "\x89PNG" {
		t.Fatalf("png = %q %v", data, err)
	}
	var cps int
	for _, r := range h.Session.Records() {
		if r.Type == "checkpoint" && r.Path == "out.png" {
			cps++
		}
	}
	if cps < 1 {
		t.Fatalf("missing checkpoint: %#v", h.Session.Records())
	}
}

func TestWebFetchRiskForPath(t *testing.T) {
	h := testHarness(t)
	tool := NewFetch(h)
	if got := EffectiveRisk(tool, json.RawMessage(`{"url":"http://example.com"}`)); got != harness.RiskNetwork {
		t.Fatalf("no path risk = %s", got)
	}
	if got := EffectiveRisk(tool, json.RawMessage(`{"url":"http://example.com","path":"a.png"}`)); got != harness.RiskWrite {
		t.Fatalf("path risk = %s", got)
	}
}

func TestWebSearchCacheAndHTML(t *testing.T) {
	h := testHarness(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>results</body></html>`)
	}))
	defer srv.Close()
	tool := &searchTool{h: h, client: srv.Client(), endpoint: srv.URL}
	got, err := tool.Run(context.Background(), json.RawMessage(`{"query":"q"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "<html") {
		t.Fatalf("html dumped: %q", got)
	}
	ents, err := os.ReadDir(filepath.Join(h.Home, "cache"))
	if err != nil || len(ents) < 2 {
		t.Fatalf("cache = %v %v", ents, err)
	}
	html, err := tool.Run(context.Background(), json.RawMessage(`{"query":"q","html":true}`))
	if err != nil || !strings.Contains(html, "<html") {
		t.Fatalf("html option = %q %v", html, err)
	}
}

func TestRememberForgetSearchMemory(t *testing.T) {
	h := testHarness(t)
	ctx := context.Background()
	got, err := NewRemember(h).Run(ctx, json.RawMessage(`{"fact":"the mascot is a seal"}`))
	if err != nil || !strings.Contains(got, "saved") {
		t.Fatalf("remember = %q %v", got, err)
	}
	idx := filepath.Join(harness.MemoryDir(h.Home, harness.HashWorkspace(h.Workspace.Root)), "MEMORY.md")
	body, err := os.ReadFile(idx)
	if err != nil || !strings.Contains(string(body), "seal") {
		t.Fatalf("index = %q %v", body, err)
	}
	found, err := NewSearchMemory(h).Run(ctx, json.RawMessage(`{"query":"seal"}`))
	if err != nil || !strings.Contains(found, "seal") || !strings.Contains(found, "untrusted") {
		t.Fatalf("search = %q %v", found, err)
	}
	if _, err := NewForget(h).Run(ctx, json.RawMessage(`{"query":"seal"}`)); err != nil {
		t.Fatal(err)
	}
	found, err = NewSearchMemory(h).Run(ctx, json.RawMessage(`{"query":"seal"}`))
	if err != nil || !strings.Contains(found, "no memory") {
		t.Fatalf("after forget = %q %v", found, err)
	}
}

func TestWriteCheckpointsFile(t *testing.T) {
	h := testHarness(t)
	ctx := context.Background()
	if _, err := NewWrite(h).Run(ctx, json.RawMessage(`{"path":"a.txt","content":"one"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWrite(h).Run(ctx, json.RawMessage(`{"path":"a.txt","content":"two"}`)); err != nil {
		t.Fatal(err)
	}
	var cps []harness.Record
	for _, r := range h.Session.Records() {
		if r.Type == "checkpoint" && r.Path == "a.txt" {
			cps = append(cps, r)
		}
	}
	if len(cps) < 2 {
		t.Fatalf("checkpoints = %#v", h.Session.Records())
	}
	if err := harness.RestoreCheckpoint(h.Workspace, cps[len(cps)-1]); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(h.Workspace.Root, "a.txt"))
	if err != nil || string(got) != "one" {
		t.Fatalf("restored = %q %v", got, err)
	}
}

func TestParsePages(t *testing.T) {
	cases := []struct {
		spec string
		n    int
		want []int
	}{
		{"1-3", 10, []int{1, 2, 3}},
		{"5", 10, []int{5}},
		{"2,4", 10, []int{2, 4}},
		{"", 10, []int{1, 2, 3, 4}},
		{"1-20", 5, []int{1, 2, 3, 4, 5}},
	}
	for _, tc := range cases {
		got, err := parsePages(tc.spec, tc.n)
		if err != nil {
			t.Fatalf("%q: %v", tc.spec, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%q: got %v want %v", tc.spec, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%q: got %v want %v", tc.spec, got, tc.want)
			}
		}
	}
	got, err := parsePages("1-20", 20)
	if err != nil || len(got) != maxPDFPages {
		t.Fatalf("cap = %v %v", got, err)
	}
}

func TestReadFilePDFHint(t *testing.T) {
	h := testHarness(t)
	if err := os.WriteFile(filepath.Join(h.Workspace.Root, "a.pdf"), []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewRead(h).Run(context.Background(), json.RawMessage(`{"path":"a.pdf"}`))
	if err == nil || !strings.Contains(err.Error(), "pdf_read") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadPDFRenders(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		t.Skip("pdfinfo not installed")
	}
	h := testHarness(t)
	path := filepath.Join(h.Workspace.Root, "tiny.pdf")
	if err := os.WriteFile(path, []byte(tinyPDF), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := NewReadPDF(h).(*pdfTool)
	text, images, err := tool.RunVisual(context.Background(), json.RawMessage(`{"path":"tiny.pdf"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "tiny.pdf") || !strings.Contains(text, "Rendered pages") {
		t.Fatalf("caption = %q", text)
	}
	if len(images) < 1 || images[0].Type != "image" || len(images[0].Data) == 0 {
		t.Fatalf("images = %#v", images)
	}
}

const tinyPDF = `%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj
2 0 obj<</Type/Pages/Count 1/Kids[3 0 R]>>endobj
3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>endobj
xref
0 4
0000000000 65535 f 
0000000009 00000 n 
0000000052 00000 n 
0000000101 00000 n 
trailer<</Size 4/Root 1 0 R>>
startxref
178
%%EOF
`
