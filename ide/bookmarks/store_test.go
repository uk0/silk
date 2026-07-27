package bookmarks

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestAddDedupes: a second Add on the same (path, line) refreshes the
// existing mark instead of stacking a duplicate, and says so by returning
// false. Marks with no path or a line below 1 are rejected outright — a
// gutter click on nothing must not create a bookmark that cannot be
// jumped to.
func TestAddDedupes(t *testing.T) {
	s := NewStore()
	if !s.Add(Bookmark{Path: "a.go", Line: 3, Context: "old()", Note: "first"}) {
		t.Fatal("Add of a new mark returned false")
	}
	if s.Add(Bookmark{Path: "a.go", Line: 3, Context: "new()", Note: "second"}) {
		t.Error("Add of an existing (path, line) returned true")
	}
	got := s.List()
	want := []Bookmark{{Path: "a.go", Line: 3, Context: "new()", Note: "second"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %+v, want %+v", got, want)
	}
	for _, bad := range []Bookmark{
		{Line: 3, Note: "no path"},
		{Path: "a.go", Line: 0, Note: "no line"},
		{Path: "a.go", Line: -2, Note: "negative line"},
	} {
		if s.Add(bad) {
			t.Errorf("Add(%+v) returned true, want rejected", bad)
		}
	}
	if len(s.List()) != 1 {
		t.Errorf("invalid marks were stored: %+v", s.List())
	}
}

// TestToggle: toggling an unmarked line sets it, toggling a marked line
// clears it, and the return value reports whether the mark is set
// afterwards so a gutter can redraw from it.
func TestToggle(t *testing.T) {
	s := NewStore()
	b := Bookmark{Path: "a.go", Line: 7, Context: "x := 1", Note: "why"}
	if !s.Toggle(b) {
		t.Fatal("Toggle of an unmarked line returned false")
	}
	if got := s.List(); !reflect.DeepEqual(got, []Bookmark{b}) {
		t.Fatalf("after the first Toggle, List() = %+v, want %+v", got, []Bookmark{b})
	}
	// A second toggle clears it even when the note differs — the mark is
	// identified by (path, line) alone.
	if s.Toggle(Bookmark{Path: "a.go", Line: 7, Note: "other"}) {
		t.Error("Toggle of a marked line returned true")
	}
	if got := s.List(); len(got) != 0 {
		t.Errorf("after the second Toggle, List() = %+v, want empty", got)
	}
	if s.Remove("a.go", 7) {
		t.Error("Remove of an absent mark returned true")
	}
}

// TestListAndByFileOrder: List is ordered by path then line regardless of
// insertion order, ByFile narrows it to one file, and both hand back
// copies the caller can mutate freely.
func TestListAndByFileOrder(t *testing.T) {
	s := NewStore()
	for _, b := range []Bookmark{
		{Path: "z.go", Line: 5},
		{Path: "a.go", Line: 40},
		{Path: "a.go", Line: 2},
		{Path: "z.go", Line: 1},
	} {
		s.Add(b)
	}
	want := []Bookmark{
		{Path: "a.go", Line: 2},
		{Path: "a.go", Line: 40},
		{Path: "z.go", Line: 1},
		{Path: "z.go", Line: 5},
	}
	got := s.List()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %+v, want %+v", got, want)
	}
	got[0].Line = 999 // mutating the copy must not reach the store
	if again := s.List(); !reflect.DeepEqual(again, want) {
		t.Errorf("List() returned the store's own slice: %+v", again)
	}

	byFile := s.ByFile("a.go")
	wantFile := []Bookmark{{Path: "a.go", Line: 2}, {Path: "a.go", Line: 40}}
	if !reflect.DeepEqual(byFile, wantFile) {
		t.Errorf("ByFile(a.go) = %+v, want %+v", byFile, wantFile)
	}
	if other := s.ByFile("missing.go"); len(other) != 0 {
		t.Errorf("ByFile of an unknown file = %+v, want empty", other)
	}
}

// TestSaveLoadRoundTrip: the marks survive Save -> Load in List order,
// Save creates the parent directory, and the same store always writes the
// same bytes so the file only changes when the bookmarks do.
func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "bookmarks.json")
	s := NewStore()
	s.Add(Bookmark{Path: "z.go", Line: 9, Context: "return nil", Note: "check this"})
	s.Add(Bookmark{Path: "a.go", Line: 12, Context: "func main() {"})
	s.Add(Bookmark{Path: "a.go", Line: 3})

	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := loaded.List(), s.List(); !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}

	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	second := filepath.Join(dir, "again.json")
	if err := loaded.Save(second); err != nil {
		t.Fatalf("Save (reloaded): %v", err)
	}
	data, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(first, data) {
		t.Errorf("re-saving a loaded store changed the bytes:\n%s\n---\n%s", first, data)
	}
}

// TestLoadCleansFile: a hand-edited file with a duplicate (path, line) and
// unusable entries loads clean — the later duplicate updates the first,
// the junk is dropped — because Load funnels everything through Add.
func TestLoadCleansFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bookmarks.json")
	raw := []byte(`[
	  {"path":"a.go","line":3,"context":"one","note":"n1"},
	  {"path":"a.go","line":3,"context":"two","note":"n2"},
	  {"path":"","line":9,"note":"no path"},
	  {"path":"b.go","line":0,"note":"no line"}
	]`)
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []Bookmark{{Path: "a.go", Line: 3, Context: "two", Note: "n2"}}
	if got := s.List(); !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %+v, want %+v", got, want)
	}
}

// TestLoadEdgeFiles: a missing bookmark file is reported as such so "no
// bookmarks yet" is distinguishable, an empty file is an empty store, and
// corrupt JSON errors.
func TestLoadEdgeFiles(t *testing.T) {
	dir := t.TempDir()

	if _, err := Load(filepath.Join(dir, "absent.json")); !os.IsNotExist(err) {
		t.Errorf("Load of a missing file: err = %v, want IsNotExist", err)
	}

	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte("\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := Load(empty)
	if err != nil {
		t.Fatalf("Load of an empty file: %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Errorf("Load of an empty file = %+v, want an empty store", got)
	}

	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte(`[{"path":`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Load(broken); err == nil {
		t.Error("Load of truncated JSON returned no error")
	}
}

// TestReanchorFindsMovedLine: three lines were inserted above the
// bookmark while the IDE was closed, so its Context now sits three lines
// lower. Reanchor follows it and reports the move; marks in other files
// and marks with no Context to match on stay put.
func TestReanchorFindsMovedLine(t *testing.T) {
	s := NewStore()
	s.Add(Bookmark{Path: "a.go", Line: 4, Context: "hello()", Note: "entry"})
	s.Add(Bookmark{Path: "a.go", Line: 2, Note: "no context"})
	s.Add(Bookmark{Path: "other.go", Line: 4, Context: "hello()"})

	lines := []string{
		"// header",     // 1
		"// header two", // 2
		"// header tre", // 3
		"package main",  // 4
		"",              // 5
		"func main() {", // 6
		"\thello()",     // 7 — moved here, and re-indented
		"}",             // 8
	}
	if moved := s.Reanchor("a.go", lines); moved != 1 {
		t.Fatalf("Reanchor moved %d marks, want 1", moved)
	}
	want := []Bookmark{
		{Path: "a.go", Line: 2, Note: "no context"},
		{Path: "a.go", Line: 7, Context: "hello()", Note: "entry"},
		{Path: "other.go", Line: 4, Context: "hello()"},
	}
	if got := s.List(); !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %+v\nwant %+v", got, want)
	}
}

// TestReanchorKeepsMatchingLine: when the line still carries the mark's
// Context nothing moves, even though the same text also appears elsewhere
// in the file and even though the line was re-indented (matching ignores
// surrounding whitespace).
func TestReanchorKeepsMatchingLine(t *testing.T) {
	s := NewStore()
	s.Add(Bookmark{Path: "a.go", Line: 3, Context: "total++", Note: "hot loop"})
	lines := []string{
		"total++",       // 1 — decoy
		"x := 0",        // 2
		"      total++", // 3 — the anchor, re-indented
		"total++",       // 4 — decoy
	}
	if moved := s.Reanchor("a.go", lines); moved != 0 {
		t.Fatalf("Reanchor moved %d marks, want 0", moved)
	}
	if got := s.List()[0].Line; got != 3 {
		t.Errorf("Line = %d, want 3 (unchanged)", got)
	}
}

// TestReanchorKeepsLineWhenContextGone: the anchored text was deleted, so
// there is nothing to relocate to. The mark keeps its line — a stale line
// number is more useful to the user than a bookmark that vanished.
func TestReanchorKeepsLineWhenContextGone(t *testing.T) {
	s := NewStore()
	s.Add(Bookmark{Path: "a.go", Line: 2, Context: "removed()", Note: "gone"})
	lines := []string{"package main", "", "func main() {}"}
	if moved := s.Reanchor("a.go", lines); moved != 0 {
		t.Fatalf("Reanchor moved %d marks, want 0", moved)
	}
	if got := s.List()[0].Line; got != 2 {
		t.Errorf("Line = %d, want 2 (unchanged)", got)
	}
}

// TestReanchorPrefersNearest: of several lines carrying the Context the
// closest one wins, and a tie between one above and one below goes to the
// lower line number.
func TestReanchorPrefersNearest(t *testing.T) {
	lines := []string{
		"alpha",   // 1
		"target",  // 2
		"beta",    // 3
		"gamma",   // 4
		"delta",   // 5
		"target",  // 6
		"epsilon", // 7
	}
	// Line 4 no longer matches; line 6 is two away, line 2 is two away —
	// the tie resolves upward.
	tie := NewStore()
	tie.Add(Bookmark{Path: "a.go", Line: 4, Context: "target"})
	if moved := tie.Reanchor("a.go", lines); moved != 1 {
		t.Fatalf("tie: Reanchor moved %d marks, want 1", moved)
	}
	if got := tie.List()[0].Line; got != 2 {
		t.Errorf("tie: Line = %d, want 2 (lower line on a tie)", got)
	}
	// Line 5 is one away from 6 and three away from 2 — closest wins.
	near := NewStore()
	near.Add(Bookmark{Path: "a.go", Line: 5, Context: "target"})
	if moved := near.Reanchor("a.go", lines); moved != 1 {
		t.Fatalf("near: Reanchor moved %d marks, want 1", moved)
	}
	if got := near.List()[0].Line; got != 6 {
		t.Errorf("near: Line = %d, want 6 (nearest match)", got)
	}
}

// TestReanchorMergesCollision: a mark that relocates onto a line another
// mark already holds is merged away, keeping the older of the two, so the
// store's (path, line) uniqueness survives a reanchor.
func TestReanchorMergesCollision(t *testing.T) {
	s := NewStore()
	s.Add(Bookmark{Path: "a.go", Line: 1, Context: "beta", Note: "older"})
	s.Add(Bookmark{Path: "a.go", Line: 2, Context: "beta", Note: "newer"})
	if moved := s.Reanchor("a.go", []string{"alpha", "beta"}); moved != 1 {
		t.Fatalf("Reanchor moved %d marks, want 1", moved)
	}
	want := []Bookmark{{Path: "a.go", Line: 2, Context: "beta", Note: "older"}}
	if got := s.List(); !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %+v, want %+v", got, want)
	}
}

// TestReanchorBeyondDrift: a match further than maxDrift away is out of
// reach, because at that distance identical text says more about the
// language than about the bookmark.
func TestReanchorBeyondDrift(t *testing.T) {
	lines := make([]string, maxDrift+40)
	for i := range lines {
		lines[i] = "filler"
	}
	lines[len(lines)-1] = "anchor()"
	s := NewStore()
	s.Add(Bookmark{Path: "a.go", Line: 1, Context: "anchor()"})
	if moved := s.Reanchor("a.go", lines); moved != 0 {
		t.Fatalf("Reanchor moved %d marks, want 0", moved)
	}
	if got := s.List()[0].Line; got != 1 {
		t.Errorf("Line = %d, want 1 (unchanged)", got)
	}
}
