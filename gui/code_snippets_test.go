package gui

import (
	"sort"
	"strings"
	"testing"
)

// TestSnippetByTrigger checks ByTrigger resolution for known + unknown keys.
func TestSnippetByTrigger(t *testing.T) {
	s := NewGoSnippetSet()
	got := s.ByTrigger("iferr")
	if got == nil {
		t.Fatal("iferr trigger not found")
	}
	if got.Trigger != "iferr" {
		t.Errorf("Trigger = %q, want iferr", got.Trigger)
	}
	if !strings.Contains(got.Body, "if err != nil") {
		t.Errorf("body missing if err != nil: %q", got.Body)
	}
	if s.ByTrigger("nonexistent") != nil {
		t.Error("nonexistent should resolve nil")
	}
}

// TestSnippetTriggersHasDefaults verifies all required defaults are present.
func TestSnippetTriggersHasDefaults(t *testing.T) {
	s := NewGoSnippetSet()
	got := s.Triggers()
	want := []string{
		"iferr", "iferrln", "forrange", "forr", "func",
		"Test", "Benchmark", "sprintf", "errwrap",
	}
	idx := make(map[string]bool, len(got))
	for _, g := range got {
		idx[g] = true
	}
	for _, w := range want {
		if !idx[w] {
			t.Errorf("Triggers() missing %q (got %v)", w, sortedCopy(got))
		}
	}
}

func sortedCopy(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}

// TestSnippetExpandIferr expands "iferr" at end of buffer and checks the $0
// cursor mark ends up at the "return err" line.
func TestSnippetExpandIferr(t *testing.T) {
	s := NewGoSnippetSet()
	buf := "iferr"
	newBuf, newCur, ok := s.Expand(buf, len(buf), "iferr")
	if !ok {
		t.Fatal("expand failed")
	}
	want := "if err != nil {\n\treturn err\n}"
	if newBuf != want {
		t.Errorf("buffer = %q, want %q", newBuf, want)
	}
	if strings.Contains(newBuf, "$0") {
		t.Error("$0 marker not stripped")
	}
	// Cursor should land right after "return err".
	wantCur := strings.Index(want, "return err") + len("return err")
	if newCur != wantCur {
		t.Errorf("cursor = %d, want %d (buffer %q)", newCur, wantCur, newBuf)
	}
}

// TestSnippetExpandIndentation verifies leading whitespace of the trigger line
// is propagated to every body line after the first.
func TestSnippetExpandIndentation(t *testing.T) {
	s := NewGoSnippetSet()
	// "\t\tiferr" — two tabs of indent before the trigger.
	buf := "func f() {\n\t\tiferr"
	cursor := len(buf)
	newBuf, _, ok := s.Expand(buf, cursor, "iferr")
	if !ok {
		t.Fatal("expand failed")
	}
	want := "func f() {\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}"
	if newBuf != want {
		t.Errorf("buffer = %q\n  want %q", newBuf, want)
	}
}

// TestSnippetExpandNoZeroMark confirms a snippet without "$0" lands the
// cursor at the end of the inserted body.
func TestSnippetExpandNoZeroMark(t *testing.T) {
	s := &SnippetSet{byTrig: map[string]*Snippet{}}
	s.add(&Snippet{Trigger: "hi", Body: "hello"})
	buf := "hi"
	newBuf, newCur, ok := s.Expand(buf, len(buf), "hi")
	if !ok {
		t.Fatal("expand failed")
	}
	if newBuf != "hello" {
		t.Errorf("buffer = %q, want hello", newBuf)
	}
	if newCur != len("hello") {
		t.Errorf("cursor = %d, want %d", newCur, len("hello"))
	}
}

// TestSnippetExpandTriggerNotAtCursor: trailing chars after the trigger
// must inhibit expansion (the cursor is no longer at the trigger's end).
func TestSnippetExpandTriggerNotAtCursor(t *testing.T) {
	s := NewGoSnippetSet()
	buf := "iferr more text"
	newBuf, newCur, ok := s.Expand(buf, len(buf), "iferr")
	if ok {
		t.Error("expected ok=false when trigger is not flush with cursor")
	}
	if newBuf != buf || newCur != len(buf) {
		t.Errorf("buffer/cursor mutated on failure: %q / %d", newBuf, newCur)
	}
}

// TestSnippetExpandUnknownTrigger returns ok=false without mutation.
func TestSnippetExpandUnknownTrigger(t *testing.T) {
	s := NewGoSnippetSet()
	buf := "wat"
	newBuf, newCur, ok := s.Expand(buf, len(buf), "wat")
	if ok {
		t.Error("expected ok=false for unknown trigger")
	}
	if newBuf != buf || newCur != len(buf) {
		t.Errorf("buffer/cursor mutated on failure: %q / %d", newBuf, newCur)
	}
}

// TestSnippetExpandLeftBoundary ensures "xiferr" (preceded by an ident
// rune) does NOT match the "iferr" trigger.
func TestSnippetExpandLeftBoundary(t *testing.T) {
	s := NewGoSnippetSet()
	buf := "xiferr"
	if _, _, ok := s.Expand(buf, len(buf), "iferr"); ok {
		t.Error("trigger preceded by ident rune should not expand")
	}
	// But space before the trigger is fine.
	buf2 := "x iferr"
	if _, _, ok := s.Expand(buf2, len(buf2), "iferr"); !ok {
		t.Error("trigger preceded by space should expand")
	}
	// And start-of-line newline boundary works too.
	buf3 := "x\niferr"
	if _, _, ok := s.Expand(buf3, len(buf3), "iferr"); !ok {
		t.Error("trigger after newline should expand")
	}
}

// TestSnippetExpandForRange checks placeholder cursor lands at $0 mid-body.
func TestSnippetExpandForRange(t *testing.T) {
	s := NewGoSnippetSet()
	buf := "forrange"
	newBuf, newCur, ok := s.Expand(buf, len(buf), "forrange")
	if !ok {
		t.Fatal("expand failed")
	}
	if strings.Contains(newBuf, "$0") {
		t.Error("$0 marker not stripped")
	}
	// Cursor should sit between "range " and " {".
	want := "for k, v := range  {\n\t\n}"
	if newBuf != want {
		t.Errorf("buffer = %q, want %q", newBuf, want)
	}
	wantCur := strings.Index(want, "range ") + len("range ")
	if newCur != wantCur {
		t.Errorf("cursor = %d, want %d", newCur, wantCur)
	}
}

// TestSnippetExpandTestFunc covers the capital-letter Test trigger.
func TestSnippetExpandTestFunc(t *testing.T) {
	s := NewGoSnippetSet()
	buf := "Test"
	newBuf, newCur, ok := s.Expand(buf, len(buf), "Test")
	if !ok {
		t.Fatal("expand failed")
	}
	want := "func Test(t *testing.T) {\n\t\n}"
	if newBuf != want {
		t.Errorf("buffer = %q, want %q", newBuf, want)
	}
	wantCur := strings.Index(want, "Test(") + len("Test")
	if newCur != wantCur {
		t.Errorf("cursor = %d, want %d", newCur, wantCur)
	}
}
