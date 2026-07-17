package ged

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunSearchMapsEngineMatches drives the non-UI search helper against a temp
// tree and checks it maps filesearch matches onto SearchMatch correctly. The
// key regression it guards is that Column is a 0-based BYTE offset, so a
// multibyte (CJK) prefix does not shift it — the old code stored a byte offset
// that Draw then indexed as a rune index, corrupting multibyte highlights. It
// also confirms the search is broadened past .go while binary blobs and .git
// content stay skipped by the engine.
func TestRunSearchMapsEngineMatches(t *testing.T) {
	dir := t.TempDir()

	// A .go file whose match sits after a multibyte prefix on line 2.
	goLine := "// 世界 hello world"
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n"+goLine+"\n")

	// A .txt file also containing the query proves the search is broadened
	// beyond .go.
	writeFile(t, filepath.Join(dir, "notes.txt"), "a world in text\n")

	// .git content and a binary blob must be skipped by the engine.
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".git", "config"), "world in git\n")
	writeFile(t, filepath.Join(dir, "blob.bin"), "world\x00binary\n")

	matches := runSearch(dir, "world")

	byFile := map[string][]SearchMatch{}
	for _, m := range matches {
		byFile[filepath.Base(m.FilePath)] = append(byFile[filepath.Base(m.FilePath)], m)
	}

	if n := len(byFile["config"]); n != 0 {
		t.Errorf(".git content was searched: %d matches", n)
	}
	if n := len(byFile["blob.bin"]); n != 0 {
		t.Errorf("binary file was searched: %d matches", n)
	}
	if n := len(byFile["notes.txt"]); n != 1 {
		t.Errorf("broadened search missed notes.txt: got %d matches, want 1", n)
	}

	got := byFile["main.go"]
	if len(got) != 1 {
		t.Fatalf("main.go matches = %d, want 1", len(got))
	}
	m := got[0]

	wantCol := strings.Index(goLine, "world") // 0-based BYTE offset
	runeCol := len([]rune(goLine[:wantCol]))   // where a rune index would land
	if wantCol == runeCol {
		t.Fatalf("test line lacks a multibyte prefix (byte %d == rune %d)", wantCol, runeCol)
	}

	if m.Line != 2 {
		t.Errorf("Line = %d, want 2", m.Line)
	}
	if m.Column != wantCol {
		t.Errorf("Column = %d, want byte offset %d (a rune offset would be %d)", m.Column, wantCol, runeCol)
	}
	if m.LineText != goLine {
		t.Errorf("LineText = %q, want %q", m.LineText, goLine)
	}
	if m.MatchLen != len("world") {
		t.Errorf("MatchLen = %d, want %d", m.MatchLen, len("world"))
	}
	if base := filepath.Base(m.FilePath); base != "main.go" {
		t.Errorf("FilePath base = %q, want main.go", base)
	}
}
