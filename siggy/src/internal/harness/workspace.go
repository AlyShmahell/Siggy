package harness

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Workspace struct {
	Root string
}

func NewWorkspace(root string) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("workspace %s: %w", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace %s is not a directory", abs)
	}
	return &Workspace{Root: abs}, nil
}

func (w *Workspace) Resolve(rel string) (string, error) {
	if rel == "" {
		return w.Root, nil
	}
	clean := filepath.Clean(rel)
	var candidate string
	if filepath.IsAbs(clean) {
		candidate = clean
	} else {
		candidate = filepath.Join(w.Root, clean)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			parent, perr := filepath.EvalSymlinks(filepath.Dir(candidate))
			if perr != nil {
				if os.IsNotExist(perr) {
					parent = filepath.Dir(candidate)
				} else {
					return "", perr
				}
			}
			resolved = filepath.Join(parent, filepath.Base(candidate))
		} else {
			return "", err
		}
	}
	root, err := filepath.EvalSymlinks(w.Root)
	if err != nil {
		root = w.Root
	}
	relToRoot, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", err
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes workspace", rel)
	}
	return resolved, nil
}

func (w *Workspace) Rel(abs string) string {
	rel, err := filepath.Rel(w.Root, abs)
	if err != nil {
		return abs
	}
	return rel
}

func (w *Workspace) Walk(fn fs.WalkDirFunc) error {
	return filepath.WalkDir(w.Root, fn)
}
