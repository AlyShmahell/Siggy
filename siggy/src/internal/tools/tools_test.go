package tools

import (
	"context"
	"encoding/json"
	"os"
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
	if _, ok := r.Get("delegate"); ok {
		t.Fatal("delegate should be absent without delegator")
	}
}
