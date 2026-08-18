package harness

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Record struct {
	Type    string `json:"type"`
	Role    string `json:"role,omitempty"`
	Text    string `json:"text,omitempty"`
	Tool    string `json:"tool,omitempty"`
	CallID  string `json:"call_id,omitempty"`
	Args    string `json:"args,omitempty"`
	Result  string `json:"result,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Node    string `json:"node,omitempty"`
	At      string `json:"at"`
}

type Session struct {
	ID   string
	Path string

	mu      sync.Mutex
	records []Record
	file    *os.File
}

func SessionsDir(home string) string {
	return filepath.Join(home, "sessions")
}

func NewSession(home string) (*Session, error) {
	dir := SessionsDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	base := time.Now().UTC().Format("20060102T150405Z")
	for n := 0; n < 50; n++ {
		id := base
		if n > 0 {
			id = fmt.Sprintf("%s-%02d", base, n)
		}
		path := filepath.Join(dir, id+".jsonl")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0o644)
		if err == nil {
			return &Session{ID: id, Path: path, file: f}, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("session id collision")
}

func OpenSession(home, id string) (*Session, error) {
	path := filepath.Join(SessionsDir(home), id+".jsonl")
	records, err := ReadRecords(path)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &Session{ID: id, Path: path, records: records, file: f}, nil
}

func DeleteSession(home, id string) error {
	if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return fmt.Errorf("invalid session id")
	}
	path := filepath.Join(SessionsDir(home), id+".jsonl")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func DeleteAllSessions(home string) error {
	ids, err := ListSessions(home)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := DeleteSession(home, id); err != nil {
			return err
		}
	}
	return nil
}

func ListSessions(home string) ([]string, error) {
	entries, err := os.ReadDir(SessionsDir(home))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(e.Name(), ".jsonl"))
	}
	return ids, nil
}

func ReadRecords(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var records []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("session record: %w", err)
		}
		records = append(records, rec)
	}
	return records, sc.Err()
}

func (s *Session) Append(rec Record) error {
	if rec.At == "" {
		rec.At = time.Now().UTC().Format(time.RFC3339)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, rec)
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := s.file.Write(append(raw, '\n')); err != nil {
		return err
	}
	return s.file.Sync()
}

func (s *Session) Records() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, len(s.records))
	copy(out, s.records)
	return out
}

func (s *Session) Close() error {
	if s.file == nil {
		return nil
	}
	return s.file.Close()
}

func CompactRecords(records []Record, keep int) []Record {
	if keep < 4 || len(records) <= keep {
		return records
	}
	var prefix []Record
	for _, r := range records {
		if r.Type == "system" {
			prefix = append(prefix, r)
		}
	}
	tail := records[len(records)-keep:]
	summary := Record{
		Type: "compact",
		Text: fmt.Sprintf("earlier conversation compacted (%d records omitted)", len(records)-keep-len(prefix)),
		At:   time.Now().UTC().Format(time.RFC3339),
	}
	out := append(append([]Record{}, prefix...), summary)
	return append(out, tail...)
}
