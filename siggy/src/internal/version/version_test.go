package version

import "testing"

func TestValueNonEmpty(t *testing.T) {
	if Value == "" {
		t.Fatal("version.Value must be non-empty")
	}
}
