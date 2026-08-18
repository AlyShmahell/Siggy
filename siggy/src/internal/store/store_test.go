package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONLImportAndZstdRoundTrip(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "20200101T000000Z"
	lines := []string{
		`{"type":"system","text":"sys"}`,
		`{"type":"user","text":"hello import"}`,
		`{"type":"assistant","text":"hi"}`,
		`{torn`,
	}
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if !db.Exists(id) {
		t.Fatal("imported session missing")
	}
	rows, err := db.Events(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("imported events = %d", len(rows))
	}
	if _, err := os.Stat(filepath.Join(dir, "imported", id+".jsonl")); err != nil {
		t.Fatalf("jsonl not moved: %v", err)
	}

	big := strings.Repeat("blob ", 2000)
	payload, _ := json.Marshal(map[string]string{"type": "assistant", "text": big})
	seq, err := db.Append(id, "assistant", payload)
	if err != nil {
		t.Fatal(err)
	}
	if seq != 4 {
		t.Fatalf("seq = %d", seq)
	}
	rows, err = db.Events(id)
	if err != nil {
		t.Fatal(err)
	}
	got := rows[len(rows)-1]
	if !strings.Contains(string(got.Payload), "blob ") {
		t.Fatalf("zstd round trip lost payload: %s", got.Payload[:min(80, len(got.Payload))])
	}
}
