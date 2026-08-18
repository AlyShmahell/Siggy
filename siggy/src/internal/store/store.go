package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
	_ "modernc.org/sqlite"
)

const (
	zstdMin   = 4096
	encJSON   = "json"
	encZstd   = "zstd+json"
	schemaVer = 1
)

type Meta struct {
	ID            string
	CreatedAt     string
	CWD           string
	ParentID      string
	SeedSeq       int
	Depth         int
	Model         string
	WorkspaceHash string
	Title         string
	Origin        string
}

type EventRow struct {
	Seq     int
	Type    string
	Payload []byte
}

type DB struct {
	sql    *sql.DB
	home   string
	Path   string
	refs   int
	heldMu sync.Mutex
	held   map[string]bool
}

var (
	dbsMu sync.Mutex
	dbs   = map[string]*DB{}
)

func Path(home string) string {
	return filepath.Join(home, "sessions.db")
}

func Open(home string) (*DB, error) {
	if home == "" {
		return nil, fmt.Errorf("empty siggy home")
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return nil, err
	}
	dbsMu.Lock()
	defer dbsMu.Unlock()
	if d, ok := dbs[home]; ok {
		d.refs++
		return d, nil
	}
	p := Path(home)
	dsn := "file:" + filepath.ToSlash(p) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	sq, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sq.SetMaxOpenConns(1)
	d := &DB{sql: sq, home: home, Path: p, refs: 1}
	if err := d.migrate(); err != nil {
		_ = sq.Close()
		return nil, err
	}
	if err := d.importJSONL(); err != nil {
		_ = sq.Close()
		return nil, err
	}
	dbs[home] = d
	return d, nil
}

func (d *DB) Close() error {
	dbsMu.Lock()
	defer dbsMu.Unlock()
	d.refs--
	if d.refs > 0 {
		return nil
	}
	delete(dbs, d.home)
	return d.sql.Close()
}

func (d *DB) migrate() error {
	if _, err := d.sql.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA synchronous=NORMAL;`); err != nil {
		return err
	}
	_, err := d.sql.Exec(`
CREATE TABLE IF NOT EXISTS meta (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  cwd TEXT,
  parent_id TEXT,
  seed_seq INTEGER NOT NULL DEFAULT 0,
  depth INTEGER NOT NULL DEFAULT 0,
  model TEXT,
  workspace_hash TEXT,
  title TEXT,
  origin TEXT
);
CREATE TABLE IF NOT EXISTS events (
  session_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  type TEXT NOT NULL,
  encoding TEXT NOT NULL,
  payload BLOB NOT NULL,
  PRIMARY KEY (session_id, seq)
);
CREATE VIRTUAL TABLE IF NOT EXISTS fts_events USING fts5(session_id, seq UNINDEXED, body);
`)
	if err != nil {
		return err
	}
	_, _ = d.sql.Exec(`INSERT OR IGNORE INTO meta(k,v) VALUES('schema', ?)`, fmt.Sprintf("%d", schemaVer))
	return nil
}

func (d *DB) importJSONL() error {
	dir := filepath.Join(d.home, "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	imported := filepath.Join(dir, "imported")
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		var n int
		if err := d.sql.QueryRow(`SELECT COUNT(1) FROM sessions WHERE id = ?`, id).Scan(&n); err != nil {
			return err
		}
		src := filepath.Join(dir, e.Name())
		if n > 0 {
			if err := os.MkdirAll(imported, 0o755); err != nil {
				return err
			}
			_ = os.Rename(src, filepath.Join(imported, e.Name()))
			continue
		}
		raw, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := d.Create(Meta{ID: id, CreatedAt: id, Title: "imported"}); err != nil {
			return err
		}
		lines := strings.Split(string(raw), "\n")
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var probe struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal([]byte(line), &probe); err != nil {
				if i == len(lines)-1 || (i == len(lines)-2 && strings.TrimSpace(lines[len(lines)-1]) == "") {
					continue
				}
				return fmt.Errorf("import %s: %w", e.Name(), err)
			}
			if _, err := d.Append(id, probe.Type, []byte(line)); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(imported, 0o755); err != nil {
			return err
		}
		if err := os.Rename(src, filepath.Join(imported, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) Create(m Meta) error {
	_, err := d.sql.Exec(`INSERT INTO sessions(id, created_at, cwd, parent_id, seed_seq, depth, model, workspace_hash, title, origin)
VALUES(?,?,?,?,?,?,?,?,?,?)`, m.ID, m.CreatedAt, m.CWD, m.ParentID, m.SeedSeq, m.Depth, m.Model, m.WorkspaceHash, m.Title, m.Origin)
	return err
}

func (d *DB) Get(id string) (Meta, error) {
	var m Meta
	err := d.sql.QueryRow(`SELECT id, created_at, cwd, parent_id, seed_seq, depth, model, workspace_hash, title, origin FROM sessions WHERE id = ?`, id).
		Scan(&m.ID, &m.CreatedAt, &m.CWD, &m.ParentID, &m.SeedSeq, &m.Depth, &m.Model, &m.WorkspaceHash, &m.Title, &m.Origin)
	if err == sql.ErrNoRows {
		return Meta{}, fmt.Errorf("session %s not found", id)
	}
	return m, err
}

func (d *DB) Exists(id string) bool {
	var n int
	_ = d.sql.QueryRow(`SELECT COUNT(1) FROM sessions WHERE id = ?`, id).Scan(&n)
	return n > 0
}

func (d *DB) List() ([]Meta, error) {
	return d.list(false)
}

func (d *DB) ListWithUser() ([]Meta, error) {
	return d.list(true)
}

func (d *DB) list(needUser bool) ([]Meta, error) {
	q := `SELECT id, created_at, cwd, parent_id, seed_seq, depth, model, workspace_hash, title, origin FROM sessions`
	if needUser {
		q += ` WHERE EXISTS (SELECT 1 FROM events e WHERE e.session_id = sessions.id AND e.type = 'user')`
	}
	q += ` ORDER BY created_at DESC, id DESC`
	rows, err := d.sql.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Meta
	for rows.Next() {
		var m Meta
		if err := rows.Scan(&m.ID, &m.CreatedAt, &m.CWD, &m.ParentID, &m.SeedSeq, &m.Depth, &m.Model, &m.WorkspaceHash, &m.Title, &m.Origin); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (d *DB) SetTitle(id, title string) error {
	_, err := d.sql.Exec(`UPDATE sessions SET title = ? WHERE id = ?`, title, id)
	return err
}

func (d *DB) Claim(id string) bool {
	d.heldMu.Lock()
	defer d.heldMu.Unlock()
	if d.held[id] {
		return false
	}
	var n int
	_ = d.sql.QueryRow(`SELECT COUNT(1) FROM sessions WHERE id = ?`, id).Scan(&n)
	if n > 0 {
		return false
	}
	if d.held == nil {
		d.held = map[string]bool{}
	}
	d.held[id] = true
	return true
}

func (d *DB) Release(id string) {
	d.heldMu.Lock()
	defer d.heldMu.Unlock()
	delete(d.held, id)
}

func (d *DB) Delete(id string) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM fts_events WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM events WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) DeleteAll() error {
	list, err := d.List()
	if err != nil {
		return err
	}
	for _, m := range list {
		if err := d.Delete(m.ID); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) Append(sessionID, recType string, payload []byte) (int, error) {
	var seq int
	err := d.sql.QueryRow(`SELECT COALESCE(MAX(seq), 0) FROM events WHERE session_id = ?`, sessionID).Scan(&seq)
	if err != nil {
		return 0, err
	}
	seq++
	enc := encJSON
	body := payload
	if len(payload) >= zstdMin {
		encEnc, err := zstd.NewWriter(nil)
		if err != nil {
			return 0, err
		}
		body = encEnc.EncodeAll(payload, nil)
		_ = encEnc.Close()
		enc = encZstd
	}
	tx, err := d.sql.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO events(session_id, seq, type, encoding, payload) VALUES(?,?,?,?,?)`, sessionID, seq, recType, enc, body); err != nil {
		return 0, err
	}
	if recType == "compact" {
		var rec struct {
			Text string `json:"text"`
		}
		bodyText := string(payload)
		if json.Unmarshal(payload, &rec) == nil && rec.Text != "" {
			bodyText = rec.Text
		}
		if _, err := tx.Exec(`INSERT INTO fts_events(session_id, seq, body) VALUES(?,?,?)`, sessionID, seq, bodyText); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	d.flush()
	return seq, nil
}

func (d *DB) flush() {
	_, _ = d.sql.Exec(`PRAGMA wal_checkpoint(PASSIVE)`)
}

func (d *DB) Events(sessionID string) ([]EventRow, error) {
	rows, err := d.sql.Query(`SELECT seq, type, encoding, payload FROM events WHERE session_id = ? ORDER BY seq`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EventRow
	for rows.Next() {
		var row EventRow
		var enc string
		var payload []byte
		if err := rows.Scan(&row.Seq, &row.Type, &enc, &payload); err != nil {
			return nil, err
		}
		if enc == encZstd {
			dec, err := zstd.NewReader(nil)
			if err != nil {
				return nil, err
			}
			payload, err = dec.DecodeAll(payload, nil)
			dec.Close()
			if err != nil {
				return nil, err
			}
		}
		row.Payload = payload
		out = append(out, row)
	}
	return out, rows.Err()
}

func (d *DB) SearchCompact(query string) ([]EventRow, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	quoted := `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
	rows, err := d.sql.Query(`SELECT session_id, seq, body FROM fts_events WHERE fts_events MATCH ? LIMIT 40`, quoted)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()
	var out []EventRow
	for rows.Next() {
		var sid string
		var seq int
		var body string
		if err := rows.Scan(&sid, &seq, &body); err != nil {
			return nil, err
		}
		out = append(out, EventRow{Seq: seq, Type: "compact", Payload: []byte(body)})
	}
	return out, rows.Err()
}
