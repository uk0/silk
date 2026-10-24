package ged

import (
	"path/filepath"
	"testing"
)

func TestGlobalSearchRejectsChangedPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.txt")
	writeFile(t, path, "user's newer content")
	p := NewGlobalSearchPanel()
	err := p.ApplyReplacements([]FileReplacement{{Path: path, Before: "old preview", After: "replacement"}})
	if err == nil {
		t.Fatal("stale replacement overwrote a concurrent file edit")
	}
	assertFileEquals(t, path, "user's newer content")
}
