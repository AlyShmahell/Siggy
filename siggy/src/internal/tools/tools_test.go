package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"siggy/src/internal/harness"
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
	if err != nil || got != "hello world" {
		t.Fatalf("read = %q %v", got, err)
	}
	e := NewEdit(h)
	if _, err := e.Run(ctx, json.RawMessage(`{"path":"a.txt","old_string":"world","new_string":"siggy"}`)); err != nil {
		t.Fatal(err)
	}
	got, err = r.Run(ctx, json.RawMessage(`{"path":"a.txt"}`))
	if err != nil || got != "hello siggy" {
		t.Fatalf("edited = %q %v", got, err)
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
	if _, ok := r.Get("read_file"); !ok {
		t.Fatal("missing read_file")
	}
	if _, ok := r.Get("read_pdf"); !ok {
		t.Fatal("missing read_pdf")
	}
	if _, ok := r.Get("delegate"); ok {
		t.Fatal("delegate should be absent without delegator")
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
	if err == nil || !strings.Contains(err.Error(), "read_pdf") {
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
