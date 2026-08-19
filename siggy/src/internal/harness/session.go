package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"siggy/src/internal/store"
)

type ToolCallRec struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

type TodoSnap struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

type Record struct {
	Seq              int           `json:"seq,omitempty"`
	Type             string        `json:"type"`
	Role             string        `json:"role,omitempty"`
	Text             string        `json:"text,omitempty"`
	Tool             string        `json:"tool,omitempty"`
	CallID           string        `json:"call_id,omitempty"`
	Args             string        `json:"args,omitempty"`
	Result           string        `json:"result,omitempty"`
	ToolCalls        []ToolCallRec `json:"tool_calls,omitempty"`
	From             int           `json:"from,omitempty"`
	To               int           `json:"to,omitempty"`
	ReplacesSeq      int           `json:"replaces_seq,omitempty"`
	Path             string        `json:"path,omitempty"`
	Mode             string        `json:"mode,omitempty"`
	Node             string        `json:"node,omitempty"`
	Todos            []TodoSnap    `json:"todos,omitempty"`
	PromptTokens     int           `json:"prompt_tokens,omitempty"`
	CompletionTokens int           `json:"completion_tokens,omitempty"`
	ReasoningTokens  int           `json:"reasoning_tokens,omitempty"`
	TotalTokens      int           `json:"total_tokens,omitempty"`
	Estimated        bool          `json:"estimated,omitempty"`
	At               string        `json:"at"`
}

type SessionMeta = store.Meta

type Session struct {
	ID   string
	Path string
	Meta SessionMeta

	mu        sync.Mutex
	records   []Record
	db        *store.DB
	persisted bool
}

func SessionsDir(home string) string {
	return filepath.Join(home, "sessions")
}

func HashWorkspace(root string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return hex.EncodeToString(sum[:])[:16]
}

func MemoryDir(home, workspaceHash string) string {
	if workspaceHash == "" {
		workspaceHash = "default"
	}
	return filepath.Join(home, "projects", workspaceHash, "memory")
}

func NewSession(home string) (*Session, error) {
	return NewSessionMeta(home, SessionMeta{})
}

func NewSessionMeta(home string, meta SessionMeta) (*Session, error) {
	db, err := store.Open(home)
	if err != nil {
		return nil, err
	}
	base := time.Now().UTC().Format("20060102T150405Z")
	var id string
	for n := 0; n < 50; n++ {
		id = base
		if n > 0 {
			id = fmt.Sprintf("%s-%02d", base, n)
		}
		if db.Claim(id) {
			break
		}
		id = ""
	}
	if id == "" {
		_ = db.Close()
		return nil, fmt.Errorf("session id collision")
	}
	meta.ID = id
	if meta.CreatedAt == "" {
		meta.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return &Session{ID: id, Path: db.Path, Meta: meta, db: db}, nil
}

func OpenSession(home, id string) (*Session, error) {
	db, err := store.Open(home)
	if err != nil {
		return nil, err
	}
	meta, err := db.Get(id)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	rows, err := db.Events(id)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	records := make([]Record, 0, len(rows))
	for _, row := range rows {
		rec, err := decodeRecord(row)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		records = append(records, rec)
	}
	for _, rec := range unpairedClosers(records) {
		raw, err := json.Marshal(rec)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		seq, err := db.Append(id, rec.Type, raw)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		rec.Seq = seq
		records = append(records, rec)
	}
	return &Session{ID: id, Path: db.Path, Meta: meta, records: records, db: db, persisted: true}, nil
}

func decodeRecord(row store.EventRow) (Record, error) {
	var rec Record
	if err := json.Unmarshal(row.Payload, &rec); err != nil {
		return Record{}, fmt.Errorf("session record: %w", err)
	}
	rec.Seq = row.Seq
	if rec.Type == "" {
		rec.Type = row.Type
	}
	return rec, nil
}

func unpairedClosers(records []Record) []Record {
	open := map[string]bool{}
	for _, r := range records {
		if r.Type == "assistant" {
			for _, tc := range r.ToolCalls {
				open[tc.ID] = true
			}
		}
		if r.Type == "tool" && r.CallID != "" {
			delete(open, r.CallID)
		}
	}
	var extra []Record
	for id := range open {
		extra = append(extra, Record{
			Type:   "tool",
			CallID: id,
			Result: "interrupted (session closed mid-turn)",
			At:     time.Now().UTC().Format(time.RFC3339),
		})
	}
	return extra
}

func DeleteSession(home, id string) error {
	if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return fmt.Errorf("invalid session id")
	}
	db, err := store.Open(home)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Delete(id)
}

func DeleteAllSessions(home string) error {
	db, err := store.Open(home)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.DeleteAll()
}

func ListSessions(home string) ([]string, error) {
	return ListSessionsFor(home, "")
}

func ListSessionsFor(home, workspaceHash string) ([]string, error) {
	list, err := ListSessionMetas(home, workspaceHash)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(list))
	for _, m := range list {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

func ListSessionMetas(home, workspaceHash string) ([]store.Meta, error) {
	db, err := store.Open(home)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	list, err := db.ListWithUser()
	if err != nil {
		return nil, err
	}
	var out []store.Meta
	for _, m := range list {
		if m.Origin == "subagent" {
			continue
		}
		if workspaceHash != "" && m.WorkspaceHash != "" && m.WorkspaceHash != workspaceHash {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func SessionExists(home, id string) bool {
	db, err := store.Open(home)
	if err != nil {
		return false
	}
	defer db.Close()
	return db.Exists(id)
}

func SearchCompact(home, query string) ([]Record, error) {
	db, err := store.Open(home)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.SearchCompact(query)
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(rows))
	for _, row := range rows {
		out = append(out, Record{Seq: row.Seq, Type: "compact", Text: string(row.Payload)})
	}
	return out, nil
}

func (s *Session) Append(rec Record) error {
	if rec.At == "" {
		rec.At = time.Now().UTC().Format(time.RFC3339)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.persisted {
		if rec.Type != "user" {
			s.records = append(s.records, rec)
			return nil
		}
		if err := s.persistLocked(); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	seq, err := s.db.Append(s.ID, rec.Type, raw)
	if err != nil {
		return err
	}
	rec.Seq = seq
	s.records = append(s.records, rec)
	return nil
}

func (s *Session) persistLocked() error {
	if s.persisted {
		return nil
	}
	if err := s.db.Create(s.Meta); err != nil {
		return err
	}
	s.db.Release(s.ID)
	pending := s.records
	s.records = s.records[:0]
	for _, rec := range pending {
		raw, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		seq, err := s.db.Append(s.ID, rec.Type, raw)
		if err != nil {
			return err
		}
		rec.Seq = seq
		s.records = append(s.records, rec)
	}
	s.persisted = true
	return nil
}

func (s *Session) SetTitle(title string) error {
	title = strings.TrimSpace(title)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Meta.Title = title
	if !s.persisted || s.db == nil {
		return nil
	}
	return s.db.SetTitle(s.ID, title)
}

func (s *Session) Records() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, len(s.records))
	copy(out, s.records)
	return out
}

func (s *Session) Close() error {
	if s.db == nil {
		return nil
	}
	if !s.persisted {
		s.db.Release(s.ID)
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func BranchSession(home, id string) (*Session, error) {
	src, err := OpenSession(home, id)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	meta := src.Meta
	meta.ParentID = src.ID
	meta.SeedSeq = 0
	if len(src.records) > 0 {
		meta.SeedSeq = src.records[len(src.records)-1].Seq
	}
	meta.ID = ""
	meta.CreatedAt = ""
	meta.Origin = ""
	child, err := NewSessionMeta(home, meta)
	if err != nil {
		return nil, err
	}
	for _, rec := range src.records {
		rec.Seq = 0
		if err := child.Append(rec); err != nil {
			_ = child.Close()
			return nil, err
		}
	}
	return child, nil
}

func ExportJSONL(records []Record) ([]byte, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	for _, rec := range records {
		if err := enc.Encode(rec); err != nil {
			return nil, err
		}
	}
	return []byte(b.String()), nil
}

func ExportMarkdown(records []Record) string {
	var b strings.Builder
	for _, rec := range records {
		switch rec.Type {
		case "user":
			fmt.Fprintf(&b, "## User\n\n%s\n\n", rec.Text)
		case "assistant":
			if rec.Text != "" {
				fmt.Fprintf(&b, "## Assistant\n\n%s\n\n", rec.Text)
			}
		case "tool":
			fmt.Fprintf(&b, "### Tool %s\n\n```\n%s\n```\n\n", rec.Tool, rec.Result)
		case "compact":
			fmt.Fprintf(&b, "## Compact\n\n%s\n\n", rec.Text)
		}
	}
	return b.String()
}

func RestoreCheckpoint(ws *Workspace, rec Record) error {
	if rec.Type != "checkpoint" || rec.Path == "" {
		return fmt.Errorf("not a checkpoint")
	}
	path, err := ws.Resolve(rec.Path)
	if err != nil {
		return err
	}
	if rec.Text == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(rec.Text), 0o644)
}
