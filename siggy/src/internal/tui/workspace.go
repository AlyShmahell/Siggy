package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"siggy/src/internal/harness"
)

type wsRow struct {
	kind  Kind
	index int
	label string
}

func (m *model) workspaceRows() []wsRow {
	rows := []wsRow{{kind: KindWorkspaceUse, label: "use this folder"}}
	if m.wsCanUp() {
		rows = append(rows, wsRow{kind: KindWorkspaceUp, label: ".."})
	}
	for i, d := range m.wsDirs {
		rows = append(rows, wsRow{kind: KindWorkspaceDir, index: i, label: d})
	}
	return rows
}

func (m *model) wsCanUp() bool {
	if m.wsRoot == "" || m.wsBrowse == "" {
		return false
	}
	cur := filepath.Clean(m.wsBrowse)
	root := filepath.Clean(m.wsRoot)
	if cur == root {
		return false
	}
	return underWorkspaceRoot(root, filepath.Dir(cur))
}

func (m *model) loadWorkspaceDirs() {
	m.wsDirs = nil
	if m.wsBrowse == "" {
		return
	}
	ents, err := os.ReadDir(m.wsBrowse)
	if err != nil {
		return
	}
	var dirs []string
	for _, e := range ents {
		if !e.IsDir() || skipWorkspaceDir(e.Name()) {
			continue
		}
		dirs = append(dirs, e.Name())
	}
	sort.Strings(dirs)
	m.wsDirs = dirs
}

func skipWorkspaceDir(name string) bool {
	return name == "" || strings.HasPrefix(name, ".") || name == "node_modules"
}

func underWorkspaceRoot(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func (m *model) applyWorkspace() {
	if m.running || m.wsBrowse == "" || m.h == nil {
		return
	}
	if !underWorkspaceRoot(m.wsRoot, m.wsBrowse) {
		return
	}
	ws, err := harness.NewWorkspace(m.wsBrowse)
	if err != nil {
		m.err = err.Error()
		return
	}
	m.h.Workspace = ws
	if m.g != nil {
		m.g.Reseed()
	}
	m.reloadSessions()
	m.startFreshSession()
	m.closeFloat()
}

func (m *model) workspaceUp() {
	if !m.wsCanUp() {
		return
	}
	m.wsBrowse = filepath.Dir(filepath.Clean(m.wsBrowse))
	m.palIdx = 0
	m.listOff = 0
	m.loadWorkspaceDirs()
}

func (m *model) enterWorkspaceDir(i int) {
	if i < 0 || i >= len(m.wsDirs) {
		return
	}
	next := filepath.Join(m.wsBrowse, m.wsDirs[i])
	if !underWorkspaceRoot(m.wsRoot, next) {
		return
	}
	m.wsBrowse = next
	m.palIdx = 0
	m.listOff = 0
	m.loadWorkspaceDirs()
}
