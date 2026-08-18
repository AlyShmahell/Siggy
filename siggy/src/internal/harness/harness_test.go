package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceRejectsEscape(t *testing.T) {
	root := t.TempDir()
	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Resolve("../outside"); err == nil {
		t.Fatal("expected escape to fail")
	}
	inside := filepath.Join(root, "a.txt")
	if err := os.WriteFile(inside, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ws.Resolve("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != inside {
		t.Fatalf("got %s want %s", got, inside)
	}
}

func TestPlanModeBlocksWrites(t *testing.T) {
	if err := ModePlan.Allows(RiskRead); err != nil {
		t.Fatal(err)
	}
	if err := ModePlan.Allows(RiskWrite); err == nil {
		t.Fatal("expected write block")
	}
	if err := ModeAct.Allows(RiskWrite); err != nil {
		t.Fatal(err)
	}
}

func TestApprovalAuto(t *testing.T) {
	bus := NewApprovalBus(true)
	d, err := bus.Ask(context.Background(), ApprovalRequest{Tool: "write_file"})
	if err != nil || !d.Allowed() {
		t.Fatalf("auto approve failed: %v %v", d, err)
	}
}

func TestLoopDetect(t *testing.T) {
	d := NewLoopDetect(3)
	if err := d.Observe("grep", `{"q":"x"}`); err != nil {
		t.Fatal(err)
	}
	if err := d.Observe("grep", `{"q":"x"}`); err != nil {
		t.Fatal(err)
	}
	if err := d.Observe("grep", `{"q":"x"}`); err == nil {
		t.Fatal("expected halt")
	}
}

func TestSessionRoundTrip(t *testing.T) {
	home := t.TempDir()
	s, err := NewSession(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(Record{Type: "user", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	opened, err := OpenSession(home, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	recs := opened.Records()
	if len(recs) != 1 || recs[0].Text != "hello" {
		t.Fatalf("records = %#v", recs)
	}
	opened.Close()
}

func TestDeleteSession(t *testing.T) {
	home := t.TempDir()
	a, err := NewSession(home)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSession(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Append(Record{Type: "user", Text: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := b.Append(Record{Type: "user", Text: "b"}); err != nil {
		t.Fatal(err)
	}
	a.Close()
	b.Close()
	if err := DeleteSession(home, a.ID); err != nil {
		t.Fatal(err)
	}
	ids, err := ListSessions(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != b.ID {
		t.Fatalf("ids = %#v", ids)
	}
	if err := DeleteAllSessions(home); err != nil {
		t.Fatal(err)
	}
	ids, err = ListSessions(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected empty, got %#v", ids)
	}
}

func TestSessionSeqAndImport(t *testing.T) {
	home := t.TempDir()
	s, err := NewSession(home)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Append(Record{Type: "system", Text: "sys"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := s.Append(Record{Type: "user", Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	recs := s.Records()
	if recs[0].Seq != 1 {
		t.Fatalf("seq = %d", recs[0].Seq)
	}
	if recs[0].Type != "system" {
		t.Fatalf("system prefix lost: %#v", recs[0])
	}
}

func TestJSONLImport(t *testing.T) {
	home := t.TempDir()
	dir := SessionsDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "imported1"
	body := `{"type":"user","text":"from jsonl"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := OpenSession(home, id)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	recs := s.Records()
	if len(recs) != 1 || recs[0].Text != "from jsonl" {
		t.Fatalf("imported = %#v", recs)
	}
}

func TestBranchCopiesParent(t *testing.T) {
	home := t.TempDir()
	s, err := NewSession(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(Record{Type: "user", Text: "seed"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	child, err := BranchSession(home, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	if child.Meta.ParentID != s.ID {
		t.Fatalf("parent_id = %q", child.Meta.ParentID)
	}
	recs := child.Records()
	if len(recs) != 1 || recs[0].Text != "seed" {
		t.Fatalf("branch records = %#v", recs)
	}
}

func TestExportRoundTrip(t *testing.T) {
	recs := []Record{
		{Seq: 1, Type: "user", Text: "hello"},
		{Seq: 2, Type: "assistant", Text: "world"},
	}
	raw, err := ExportJSONL(recs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"type":"user"`) || !strings.Contains(string(raw), "hello") {
		t.Fatalf("jsonl = %s", raw)
	}
	md := ExportMarkdown(recs)
	if !strings.Contains(md, "## User") || !strings.Contains(md, "hello") || !strings.Contains(md, "world") {
		t.Fatalf("md = %s", md)
	}
}

func TestCheckpointRestore(t *testing.T) {
	root := t.TempDir()
	ws, err := NewWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RestoreCheckpoint(ws, Record{Type: "checkpoint", Path: "a.txt", Text: "old"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "old" {
		t.Fatalf("restored = %q %v", got, err)
	}
}

func TestListSessionsHidesSubagents(t *testing.T) {
	home := t.TempDir()
	parent, err := NewSessionMeta(home, SessionMeta{WorkspaceHash: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Append(Record{Type: "user", Text: "parent"}); err != nil {
		t.Fatal(err)
	}
	parent.Close()
	child, err := NewSessionMeta(home, SessionMeta{WorkspaceHash: "abc", Origin: "subagent", ParentID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Append(Record{Type: "user", Text: "child"}); err != nil {
		t.Fatal(err)
	}
	child.Close()
	ids, err := ListSessionsFor(home, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != parent.ID {
		t.Fatalf("ids = %#v", ids)
	}
}

func TestListSessionsSkipsEmptyAndNewestFirst(t *testing.T) {
	home := t.TempDir()
	empty, err := NewSession(home)
	if err != nil {
		t.Fatal(err)
	}
	empty.Close()
	ids, err := ListSessions(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("empty session listed: %#v", ids)
	}
	a, err := NewSession(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Append(Record{Type: "user", Text: "first"}); err != nil {
		t.Fatal(err)
	}
	a.Close()
	b, err := NewSession(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Append(Record{Type: "user", Text: "second"}); err != nil {
		t.Fatal(err)
	}
	b.Close()
	ids, err = ListSessions(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != b.ID || ids[1] != a.ID {
		t.Fatalf("newest first = %#v want %#v", ids, []string{b.ID, a.ID})
	}
}
