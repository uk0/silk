package gui

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Coverage gutter state API (pure, no GL / no Draw)
//
// Coverage is keyed 0-based, matching cursorLine / breakpoints / bookmarks.
// The editor only renders what the host pushes via SetCoverage; there is no
// parser or `go tool cover` integration in here.
// ---------------------------------------------------------------------------

func TestCodeEditorCoverageInitiallyAbsent(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")
	if e.HasCoverage() {
		t.Errorf("new editor should not have coverage data, HasCoverage() = true")
	}
	if covered, has := e.LineCovered(0); covered || has {
		t.Errorf("LineCovered(0) on fresh editor = (%v,%v), want (false,false)", covered, has)
	}
}

func TestCodeEditorSetCoverage(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc\nd")

	e.SetCoverage(map[int]bool{0: true, 1: false, 3: true})
	if !e.HasCoverage() {
		t.Fatalf("HasCoverage() = false after SetCoverage")
	}

	cases := []struct {
		line          int
		wantCovered   bool
		wantHasEntry  bool
	}{
		{0, true, true},
		{1, false, true},
		{2, false, false}, // missing line — no entry, distinguish from "not covered"
		{3, true, true},
	}
	for _, c := range cases {
		covered, has := e.LineCovered(c.line)
		if covered != c.wantCovered || has != c.wantHasEntry {
			t.Errorf("LineCovered(%d) = (%v,%v), want (%v,%v)",
				c.line, covered, has, c.wantCovered, c.wantHasEntry)
		}
	}
}

func TestCodeEditorSetCoverageCopiesMap(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc")

	src := map[int]bool{0: true, 1: false}
	e.SetCoverage(src)

	// Mutating the source AFTER SetCoverage must not affect the editor's view.
	src[0] = false
	src[2] = true

	if covered, _ := e.LineCovered(0); !covered {
		t.Errorf("editor's view of line 0 changed after mutating source map")
	}
	if _, has := e.LineCovered(2); has {
		t.Errorf("editor picked up a new key added to source map after SetCoverage")
	}
}

func TestCodeEditorClearCoverage(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb")
	e.SetCoverage(map[int]bool{0: true, 1: true})
	if !e.HasCoverage() {
		t.Fatalf("setup: HasCoverage() = false")
	}

	e.ClearCoverage()
	if e.HasCoverage() {
		t.Errorf("HasCoverage() = true after ClearCoverage")
	}
	if covered, has := e.LineCovered(0); covered || has {
		t.Errorf("LineCovered(0) after ClearCoverage = (%v,%v), want (false,false)", covered, has)
	}
}

func TestCodeEditorSetCoverageNilClears(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb")
	e.SetCoverage(map[int]bool{0: true})
	if !e.HasCoverage() {
		t.Fatalf("setup: HasCoverage() = false")
	}

	e.SetCoverage(nil)
	if e.HasCoverage() {
		t.Errorf("SetCoverage(nil) should clear; HasCoverage() = true")
	}
}

// ---------------------------------------------------------------------------
// nextLineIndent pure helper
//
// Indent unit is whatever the caller passes. The CodeEditor uses "\t" because
// the editor's KeyTab handler inserts a literal tab; tests cover both tab and
// 4-space indent units to lock the helper's purity.
// ---------------------------------------------------------------------------

func TestNextLineIndentEmptyLine(t *testing.T) {
	got := nextLineIndent("", 0, "\t")
	if got != "" {
		t.Errorf("nextLineIndent(\"\", 0, \\t) = %q, want \"\"", got)
	}
}

func TestNextLineIndentPreservesLeadingTab(t *testing.T) {
	// Cursor at end of "\tfoo" (no opening brace) — just copy "\t".
	line := "\tfoo"
	got := nextLineIndent(line, len([]rune(line)), "\t")
	if got != "\t" {
		t.Errorf("nextLineIndent(%q, end, \\t) = %q, want \\t", line, got)
	}
}

func TestNextLineIndentTabBraceAtEnd(t *testing.T) {
	// Cursor at end of "\tfoo {" — copy "\t" and add one more tab.
	line := "\tfoo {"
	got := nextLineIndent(line, len([]rune(line)), "\t")
	if got != "\t\t" {
		t.Errorf("nextLineIndent(%q, end, \\t) = %q, want \\t\\t", line, got)
	}
}

func TestNextLineIndentSpacesBraceAtEnd(t *testing.T) {
	// Cursor at end of "    func f() {" with a 4-space indent unit:
	// copy 4 spaces leading, add 4 more → 8 spaces total.
	line := "    func f() {"
	got := nextLineIndent(line, len([]rune(line)), "    ")
	if got != "        " {
		t.Errorf("nextLineIndent(%q, end, \"    \") = %q, want 8 spaces", line, got)
	}
}

func TestNextLineIndentMidLineDoesNotExtend(t *testing.T) {
	// Cursor in the middle of "\tfoo {" — only the leading whitespace prefix.
	line := "\tfoo {"
	got := nextLineIndent(line, 3, "\t") // cursor sits after "\tfo"
	if got != "\t" {
		t.Errorf("nextLineIndent(%q, mid, \\t) = %q, want \\t (no extra indent)", line, got)
	}
}

func TestNextLineIndentTrailingWhitespaceCountsAsEnd(t *testing.T) {
	// "Cursor at end" tolerates trailing whitespace after the cursor — the
	// remaining run is whitespace only, so { still triggers the extra step.
	line := "if x {   "
	got := nextLineIndent(line, 6, "\t") // cursor sits after "if x {"
	if got != "\t" {
		t.Errorf("nextLineIndent(%q, after-brace, \\t) = %q, want \\t", line, got)
	}
}

func TestNextLineIndentNoIndentNoBrace(t *testing.T) {
	line := "hello world"
	got := nextLineIndent(line, len([]rune(line)), "\t")
	if got != "" {
		t.Errorf("nextLineIndent(%q, end, \\t) = %q, want \"\"", line, got)
	}
}

func TestNextLineIndentNegativeColClamped(t *testing.T) {
	got := nextLineIndent("\tfoo", -5, "\t")
	if got != "\t" {
		t.Errorf("nextLineIndent with negative col = %q, want \\t", got)
	}
}

func TestNextLineIndentOverlongColClamped(t *testing.T) {
	// Past-end cursor is clamped to end-of-line, so the brace rule still fires.
	line := "x {"
	got := nextLineIndent(line, 9999, "\t")
	if got != "\t" {
		t.Errorf("nextLineIndent(%q, 9999, \\t) = %q, want \\t", line, got)
	}
}

// ---------------------------------------------------------------------------
// End-to-end: pressing Enter at end of a line ending in '{' inserts a new
// line with one extra tab of indent. Drives the editor's own OnKeyDown so
// the test exercises the same path interactive users do.
// ---------------------------------------------------------------------------

func TestCodeEditorEnterIndentsAfterOpenBrace(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("if true {\n}")
	// Place the cursor at the end of line 0 ("if true {").
	e.cursorLine = 0
	e.cursorCol = len([]rune(e.lines[0]))

	e.OnKeyDown(KeyEnter, false)

	// After Enter we expect three lines: "if true {", "\t", "}".
	if len(e.lines) != 3 {
		t.Fatalf("line count = %d, want 3 (lines = %#v)", len(e.lines), e.lines)
	}
	if e.lines[0] != "if true {" {
		t.Errorf("line 0 = %q, want %q", e.lines[0], "if true {")
	}
	if e.lines[1] != "\t" {
		t.Errorf("line 1 = %q, want \\t (one tab of indent)", e.lines[1])
	}
	if e.lines[2] != "}" {
		t.Errorf("line 2 = %q, want %q", e.lines[2], "}")
	}
	// Cursor lands on the new line, just after the inserted tab.
	if e.cursorLine != 1 {
		t.Errorf("cursorLine = %d, want 1", e.cursorLine)
	}
	if e.cursorCol != 1 {
		t.Errorf("cursorCol = %d, want 1 (past the tab)", e.cursorCol)
	}
}

func TestCodeEditorEnterPreservesIndentNoBrace(t *testing.T) {
	// No '{' at end of line — just copy leading whitespace.
	e := NewCodeEditor()
	e.SetText("\tfoo")
	e.cursorLine = 0
	e.cursorCol = len([]rune(e.lines[0]))

	e.OnKeyDown(KeyEnter, false)

	if len(e.lines) != 2 {
		t.Fatalf("line count = %d, want 2", len(e.lines))
	}
	if e.lines[1] != "\t" {
		t.Errorf("line 1 = %q, want \\t (preserved indent only, no extra)", e.lines[1])
	}
}
