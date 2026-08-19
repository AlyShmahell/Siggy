package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

func WritePageCache(home, kind, url string, raw []byte, md string) {
	if home == "" || kind == "" || url == "" {
		return
	}
	dir := filepath.Join(home, "cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	sum := sha256.Sum256([]byte(url))
	base := kind + "-" + hex.EncodeToString(sum[:])
	_ = os.WriteFile(filepath.Join(dir, base+".html"), raw, 0o644)
	_ = os.WriteFile(filepath.Join(dir, base+".md"), []byte(md), 0o644)
}
