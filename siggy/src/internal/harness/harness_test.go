package harness

import (
	"context"
	"os"
	"path/filepath"
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

func TestCompactRecords(t *testing.T) {
	var recs []Record
	recs = append(recs, Record{Type: "system", Text: "sys"})
	for i := 0; i < 20; i++ {
		recs = append(recs, Record{Type: "user", Text: "x"})
	}
	out := CompactRecords(recs, 4)
	if len(out) >= len(recs) {
		t.Fatalf("expected compaction, got %d", len(out))
	}
	if out[0].Type != "system" {
		t.Fatalf("system prefix lost: %#v", out[0])
	}
}
