package core

import (
	"strings"
	"testing"
)

// twoFilePatch modifies one file and creates another, the shape `git diff`
// emits for "edited alpha, added beta".
const twoFilePatch = `diff --git a/alpha.txt b/alpha.txt
index 1111111..2222222 100644
--- a/alpha.txt
+++ b/alpha.txt
@@ -1,4 +1,4 @@ func alpha()
 one
-two
+TWO
 three
 four
diff --git a/beta.txt b/beta.txt
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/beta.txt
@@ -0,0 +1,2 @@
+hello
+world
`

// TestParsePatchSetTwoFiles pins the multi-file contract: both files come
// back (the old silkide path rendered only the first), each with its own
// hunks, and the /dev/null old side marks the second one as a creation.
func TestParsePatchSetTwoFiles(t *testing.T) {
	ps, err := ParsePatchSet(twoFilePatch)
	if err != nil {
		t.Fatalf("ParsePatchSet: %v", err)
	}
	if len(ps.Files) != 2 {
		t.Fatalf("parsed %d files, want 2: %+v", len(ps.Files), ps.Files)
	}

	a := ps.Files[0]
	if a.OldPath != "alpha.txt" || a.NewPath != "alpha.txt" {
		t.Errorf("file 0 paths = (%q, %q), want (alpha.txt, alpha.txt)", a.OldPath, a.NewPath)
	}
	if a.IsAdd() || a.IsDelete() || a.IsRename() {
		t.Errorf("file 0 misclassified: add=%v delete=%v rename=%v", a.IsAdd(), a.IsDelete(), a.IsRename())
	}
	if len(a.Hunks) != 1 {
		t.Fatalf("file 0 hunks = %d, want 1", len(a.Hunks))
	}
	h := a.Hunks[0]
	if h.OldStart != 1 || h.OldLines != 4 || h.NewStart != 1 || h.NewLines != 4 {
		t.Errorf("hunk ranges = -%d,%d +%d,%d, want -1,4 +1,4", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
	}
	if h.Section != "func alpha()" {
		t.Errorf("hunk section = %q, want %q", h.Section, "func alpha()")
	}
	want := []PatchLine{
		{Kind: PatchContext, Text: "one"},
		{Kind: PatchDeleted, Text: "two"},
		{Kind: PatchAdded, Text: "TWO"},
		{Kind: PatchContext, Text: "three"},
		{Kind: PatchContext, Text: "four"},
	}
	if len(h.Lines) != len(want) {
		t.Fatalf("hunk lines = %d, want %d: %+v", len(h.Lines), len(want), h.Lines)
	}
	for i := range want {
		if h.Lines[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, h.Lines[i], want[i])
		}
	}
	if added, deleted := h.Stats(); added != 1 || deleted != 1 {
		t.Errorf("hunk stats = (+%d, -%d), want (+1, -1)", added, deleted)
	}

	b := ps.Files[1]
	if b.OldPath != "" || b.NewPath != "beta.txt" {
		t.Errorf("file 1 paths = (%q, %q), want (\"\", beta.txt)", b.OldPath, b.NewPath)
	}
	if !b.IsAdd() || b.IsDelete() || b.IsRename() {
		t.Errorf("file 1 should be an add: add=%v delete=%v rename=%v", b.IsAdd(), b.IsDelete(), b.IsRename())
	}
	if b.Path() != "beta.txt" {
		t.Errorf("file 1 Path() = %q, want beta.txt", b.Path())
	}
	if len(b.Hunks) != 1 {
		t.Fatalf("file 1 hunks = %d, want 1", len(b.Hunks))
	}
	cb := b.Hunks[0]
	if cb.OldStart != 0 || cb.OldLines != 0 || cb.NewStart != 1 || cb.NewLines != 2 {
		t.Errorf("creation hunk = -%d,%d +%d,%d, want -0,0 +1,2", cb.OldStart, cb.OldLines, cb.NewStart, cb.NewLines)
	}

	// A creation hunk applies to nothing and yields the whole new file.
	got, err := b.Apply("")
	if err != nil {
		t.Fatalf("Apply(create): %v", err)
	}
	if got != "hello\nworld\n" {
		t.Errorf("Apply(create) = %q, want %q", got, "hello\nworld\n")
	}
}

// renameDeletePatch renames one file (with an edit inside it) and deletes
// another — the two identity cases beyond plain modification.
const renameDeletePatch = `diff --git a/old/name.go b/new/name.go
similarity index 88%
rename from old/name.go
rename to new/name.go
--- a/old/name.go
+++ b/new/name.go
@@ -1,3 +1,3 @@
 package main
-// old comment
+// new comment
 func main() {}
diff --git a/gone.txt b/gone.txt
deleted file mode 100644
index 4444444..0000000
--- a/gone.txt
+++ /dev/null
@@ -1,2 +0,0 @@
-bye
-now
`

// TestParsePatchSetRenameAndDelete checks rename identity survives
// (rename from/to plus the differing ---/+++ paths) and that a /dev/null new
// side reads as a delete whose Apply empties the file.
func TestParsePatchSetRenameAndDelete(t *testing.T) {
	ps, err := ParsePatchSet(renameDeletePatch)
	if err != nil {
		t.Fatalf("ParsePatchSet: %v", err)
	}
	if len(ps.Files) != 2 {
		t.Fatalf("parsed %d files, want 2: %+v", len(ps.Files), ps.Files)
	}

	r := ps.Files[0]
	if r.OldPath != "old/name.go" || r.NewPath != "new/name.go" {
		t.Errorf("rename paths = (%q, %q), want (old/name.go, new/name.go)", r.OldPath, r.NewPath)
	}
	if !r.IsRename() || r.IsAdd() || r.IsDelete() {
		t.Errorf("file 0 should be a rename: rename=%v add=%v delete=%v", r.IsRename(), r.IsAdd(), r.IsDelete())
	}
	orig := "package main\n// old comment\nfunc main() {}\n"
	got, err := r.Apply(orig)
	if err != nil {
		t.Fatalf("Apply(rename edit): %v", err)
	}
	if want := "package main\n// new comment\nfunc main() {}\n"; got != want {
		t.Errorf("Apply(rename edit) = %q, want %q", got, want)
	}

	d := ps.Files[1]
	if d.OldPath != "gone.txt" || d.NewPath != "" {
		t.Errorf("delete paths = (%q, %q), want (gone.txt, \"\")", d.OldPath, d.NewPath)
	}
	if !d.IsDelete() || d.IsRename() {
		t.Errorf("file 1 should be a delete: delete=%v rename=%v", d.IsDelete(), d.IsRename())
	}
	if d.Path() != "gone.txt" {
		t.Errorf("delete Path() = %q, want gone.txt", d.Path())
	}
	got, err = d.Apply("bye\nnow\n")
	if err != nil {
		t.Fatalf("Apply(delete): %v", err)
	}
	if got != "" {
		t.Errorf("Apply(delete) = %q, want empty", got)
	}
}

// TestParsePatchSetBodyLinesLookingLikeHeaders is the reason the body is
// consumed by the @@ counters instead of by first-character sniffing: a
// deleted "-- x" reaches the diff as "--- x" and an added "++ y" as "+++ y",
// both of which a prefix-guessing parser mistakes for file headers and splits
// the file on.
func TestParsePatchSetBodyLinesLookingLikeHeaders(t *testing.T) {
	src := `--- a/tricky.sql
+++ b/tricky.sql
@@ -1,3 +1,4 @@
 keep
--- sql comment
+-- SQL comment
+++ marker
 tail
`
	ps, err := ParsePatchSet(src)
	if err != nil {
		t.Fatalf("ParsePatchSet: %v", err)
	}
	if len(ps.Files) != 1 {
		t.Fatalf("parsed %d files, want 1 (body lines split the file): %+v", len(ps.Files), ps.Files)
	}
	f := ps.Files[0]
	if f.OldPath != "tricky.sql" || f.NewPath != "tricky.sql" {
		t.Errorf("paths = (%q, %q), want (tricky.sql, tricky.sql)", f.OldPath, f.NewPath)
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(f.Hunks))
	}
	want := []PatchLine{
		{Kind: PatchContext, Text: "keep"},
		{Kind: PatchDeleted, Text: "-- sql comment"},
		{Kind: PatchAdded, Text: "-- SQL comment"},
		{Kind: PatchAdded, Text: "++ marker"},
		{Kind: PatchContext, Text: "tail"},
	}
	got := f.Hunks[0].Lines
	if len(got) != len(want) {
		t.Fatalf("lines = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	orig := "keep\n-- sql comment\ntail\n"
	applied, err := f.Apply(orig)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if wantText := "keep\n-- SQL comment\n++ marker\ntail\n"; applied != wantText {
		t.Errorf("Apply = %q, want %q", applied, wantText)
	}
}

// twoHunkPatch edits line 3 and line 9 of a ten-line file, leaving a five
// line unchanged gap between the two hunks.
const twoHunkPatch = `--- a/f.txt
+++ b/f.txt
@@ -2,3 +2,3 @@
 l2
-l3
+L3
 l4
@@ -8,3 +8,3 @@
 l8
-l9
+L9
 l10
`

// tenLineFile is the original twoHunkPatch applies to.
const tenLineFile = "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\n"

// TestApplyPatchAndReverseRoundTrip applies both hunks and then the reversed
// patch, which must land back on the byte-identical original.
func TestApplyPatchAndReverseRoundTrip(t *testing.T) {
	ps, err := ParsePatchSet(twoHunkPatch)
	if err != nil {
		t.Fatalf("ParsePatchSet: %v", err)
	}
	f := ps.Files[0]
	if len(f.Hunks) != 2 {
		t.Fatalf("hunks = %d, want 2", len(f.Hunks))
	}

	applied, err := f.Apply(tenLineFile)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := "l1\nl2\nL3\nl4\nl5\nl6\nl7\nl8\nL9\nl10\n"
	if applied != want {
		t.Fatalf("Apply = %q, want %q", applied, want)
	}

	rev := f.Reverse()
	if rev.OldPath != "f.txt" || rev.NewPath != "f.txt" {
		t.Errorf("reverse paths = (%q, %q), want (f.txt, f.txt)", rev.OldPath, rev.NewPath)
	}
	if rh := rev.Hunks[0]; rh.OldStart != 2 || rh.NewStart != 2 || rh.Lines[1].Kind != PatchAdded || rh.Lines[1].Text != "l3" {
		t.Errorf("reversed hunk 0 = %+v, want deletion flipped to addition of l3", rh)
	}

	back, err := rev.Apply(applied)
	if err != nil {
		t.Fatalf("Apply(reverse): %v", err)
	}
	if back != tenLineFile {
		t.Errorf("round trip = %q, want %q", back, tenLineFile)
	}
}

// TestApplyPatchSelectedStagesOneHunk is the partial-staging contract: hunk
// coordinates are all original-file relative, so applying a subset needs no
// offset fixups and leaves the skipped hunk's lines untouched.
func TestApplyPatchSelectedStagesOneHunk(t *testing.T) {
	ps, err := ParsePatchSet(twoHunkPatch)
	if err != nil {
		t.Fatalf("ParsePatchSet: %v", err)
	}
	f := ps.Files[0]

	firstOnly, err := f.ApplySelected(tenLineFile, []int{0})
	if err != nil {
		t.Fatalf("ApplySelected([0]): %v", err)
	}
	if want := "l1\nl2\nL3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\n"; firstOnly != want {
		t.Errorf("ApplySelected([0]) = %q, want %q", firstOnly, want)
	}

	secondOnly, err := f.ApplySelected(tenLineFile, []int{1})
	if err != nil {
		t.Fatalf("ApplySelected([1]): %v", err)
	}
	if want := "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nL9\nl10\n"; secondOnly != want {
		t.Errorf("ApplySelected([1]) = %q, want %q", secondOnly, want)
	}

	// Out of order and duplicated indices normalise to "apply both".
	both, err := f.ApplySelected(tenLineFile, []int{1, 0, 1})
	if err != nil {
		t.Fatalf("ApplySelected([1 0 1]): %v", err)
	}
	if want := "l1\nl2\nL3\nl4\nl5\nl6\nl7\nl8\nL9\nl10\n"; both != want {
		t.Errorf("ApplySelected([1 0 1]) = %q, want %q", both, want)
	}

	// Staging nothing changes nothing.
	none, err := f.ApplySelected(tenLineFile, nil)
	if err != nil {
		t.Fatalf("ApplySelected(nil): %v", err)
	}
	if none != tenLineFile {
		t.Errorf("ApplySelected(nil) = %q, want the original", none)
	}

	if _, err := f.ApplySelected(tenLineFile, []int{5}); err == nil {
		t.Error("ApplySelected([5]) on a 2-hunk patch: want an out-of-range error, got nil")
	}
}

// TestApplyPatchContextMismatch guards the validation: a patch whose context
// no longer matches the file must fail loudly instead of writing garbage.
func TestApplyPatchContextMismatch(t *testing.T) {
	ps, err := ParsePatchSet(twoHunkPatch)
	if err != nil {
		t.Fatalf("ParsePatchSet: %v", err)
	}
	stale := "l1\nl2\nCHANGED\nl4\nl5\nl6\nl7\nl8\nl9\nl10\n"
	if _, err := ps.Files[0].Apply(stale); err == nil {
		t.Fatal("Apply on a stale file: want a mismatch error, got nil")
	} else if !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("Apply error = %v, want it to mention a mismatch", err)
	}

	// A hunk anchored past the end of the file is rejected too.
	short := "l1\n"
	if _, err := ps.Files[0].Apply(short); err == nil {
		t.Fatal("Apply on a truncated file: want an error, got nil")
	}
}

// TestApplyPatchNoNewlineMarker covers both directions of the
// "\ No newline at end of file" marker: dropping the final newline and
// putting it back.
func TestApplyPatchNoNewlineMarker(t *testing.T) {
	// Patch strips the trailing newline.
	strip := `--- a/n.txt
+++ b/n.txt
@@ -1,2 +1,2 @@
 first
-second
+SECOND
\ No newline at end of file
`
	ps, err := ParsePatchSet(strip)
	if err != nil {
		t.Fatalf("ParsePatchSet(strip): %v", err)
	}
	f := ps.Files[0]
	last := f.Hunks[0].Lines[len(f.Hunks[0].Lines)-1]
	if last.Kind != PatchAdded || last.Text != "SECOND" || !last.NoNewline {
		t.Fatalf("last line = %+v, want added SECOND with NoNewline", last)
	}
	got, err := f.Apply("first\nsecond\n")
	if err != nil {
		t.Fatalf("Apply(strip): %v", err)
	}
	if got != "first\nSECOND" {
		t.Errorf("Apply(strip) = %q, want %q (no trailing newline)", got, "first\nSECOND")
	}
	back, err := f.Reverse().Apply(got)
	if err != nil {
		t.Fatalf("Apply(reverse strip): %v", err)
	}
	if back != "first\nsecond\n" {
		t.Errorf("reverse of a newline-stripping patch = %q, want %q", back, "first\nsecond\n")
	}

	// Patch adds the trailing newline back: the marker sits on the deleted
	// (old) line this time.
	add := `--- a/n.txt
+++ b/n.txt
@@ -1,2 +1,2 @@
 first
-second
\ No newline at end of file
+second2
`
	ps, err = ParsePatchSet(add)
	if err != nil {
		t.Fatalf("ParsePatchSet(add): %v", err)
	}
	lines := ps.Files[0].Hunks[0].Lines
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3 (the marker is not a line): %+v", len(lines), lines)
	}
	if lines[1].Kind != PatchDeleted || !lines[1].NoNewline {
		t.Errorf("line 1 = %+v, want deleted with NoNewline", lines[1])
	}
	if lines[2].NoNewline {
		t.Errorf("line 2 = %+v, want NoNewline false", lines[2])
	}
	got, err = ps.Files[0].Apply("first\nsecond")
	if err != nil {
		t.Fatalf("Apply(add): %v", err)
	}
	if got != "first\nsecond2\n" {
		t.Errorf("Apply(add) = %q, want %q", got, "first\nsecond2\n")
	}
}

// TestApplyPatchTerminatesUnmarkedFinalLine pins the other half of the
// terminator rule: git always marks a missing final newline, so a patch that
// replaces the last line WITHOUT a marker is asserting the result ends with
// one — even when the file it applied to did not.
func TestApplyPatchTerminatesUnmarkedFinalLine(t *testing.T) {
	src := `--- a/n.txt
+++ b/n.txt
@@ -1,2 +1,2 @@
 first
-second
+SECOND
`
	ps, err := ParsePatchSet(src)
	if err != nil {
		t.Fatalf("ParsePatchSet: %v", err)
	}
	got, err := ps.Files[0].Apply("first\nsecond")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != "first\nSECOND\n" {
		t.Errorf("Apply = %q, want %q", got, "first\nSECOND\n")
	}

	// A hunk that stops short of the end of the file leaves the terminator
	// alone — only the patch's own final line gets a say.
	mid := `--- a/n.txt
+++ b/n.txt
@@ -1,1 +1,1 @@
-first
+FIRST
`
	ps, err = ParsePatchSet(mid)
	if err != nil {
		t.Fatalf("ParsePatchSet(mid): %v", err)
	}
	got, err = ps.Files[0].Apply("first\nsecond")
	if err != nil {
		t.Fatalf("Apply(mid): %v", err)
	}
	if got != "FIRST\nsecond" {
		t.Errorf("Apply(mid) = %q, want %q (terminator untouched)", got, "FIRST\nsecond")
	}
}

// TestRebuildPatchSidesKeepsGaps is the gap-reconstruction contract that
// silkide's hunk-only rendering missed: the unchanged lines between the two
// hunks (l5..l7) must appear on both sides.
func TestRebuildPatchSidesKeepsGaps(t *testing.T) {
	ps, err := ParsePatchSet(twoHunkPatch)
	if err != nil {
		t.Fatalf("ParsePatchSet: %v", err)
	}
	oldText, newText, err := ps.Files[0].RebuildSides(tenLineFile)
	if err != nil {
		t.Fatalf("RebuildSides: %v", err)
	}
	if oldText != tenLineFile {
		t.Errorf("old side = %q, want the original verbatim", oldText)
	}
	oldLines := strings.Split(strings.TrimSuffix(oldText, "\n"), "\n")
	newLines := strings.Split(strings.TrimSuffix(newText, "\n"), "\n")
	if len(oldLines) != 10 || len(newLines) != 10 {
		t.Fatalf("side lengths = (%d, %d), want (10, 10): gaps dropped", len(oldLines), len(newLines))
	}
	for _, gap := range []string{"l1", "l5", "l6", "l7", "l10"} {
		if !strings.Contains(newText, gap+"\n") {
			t.Errorf("new side is missing unchanged gap line %q: %q", gap, newText)
		}
	}
	if newLines[2] != "L3" || newLines[8] != "L9" {
		t.Errorf("new side edits landed wrong: line3=%q line9=%q", newLines[2], newLines[8])
	}
}

// TestPatchHunkHeaderRendering pins the header text a viewer shows above each
// hunk, including git's habit of eliding a ",1" count and the section hint.
func TestPatchHunkHeaderRendering(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"@@ -1,4 +1,4 @@ func main()", "@@ -1,4 +1,4 @@ func main()"},
		{"@@ -3 +3 @@", "@@ -3 +3 @@"},
		{"@@ -0,0 +1,2 @@", "@@ -0,0 +1,2 @@"},
	}
	for _, c := range cases {
		h, err := parsePatchHunkHeader(c.src)
		if err != nil {
			t.Fatalf("parsePatchHunkHeader(%q): %v", c.src, err)
		}
		if got := h.Header(); got != c.want {
			t.Errorf("Header() for %q = %q, want %q", c.src, got, c.want)
		}
	}
}

// TestParsePatchSetMalformedHunkKeepsRest mirrors ParseUnifiedDiff's
// behaviour: a broken @@ header is reported but never aborts the parse, so a
// staging UI still sees the files it can act on.
func TestParsePatchSetMalformedHunkKeepsRest(t *testing.T) {
	src := `--- a/a.txt
+++ b/a.txt
@@ garbage @@
 x
diff --git a/b.txt b/b.txt
--- a/b.txt
+++ b/b.txt
@@ -1 +1 @@
-x
+y
`
	ps, err := ParsePatchSet(src)
	if err == nil {
		t.Fatal("ParsePatchSet with a malformed @@ header: want an error, got nil")
	}
	if len(ps.Files) != 2 {
		t.Fatalf("parsed %d files, want 2 (partial results survive): %+v", len(ps.Files), ps.Files)
	}
	if len(ps.Files[0].Hunks) != 0 {
		t.Errorf("file 0 hunks = %d, want 0 (the broken hunk is dropped)", len(ps.Files[0].Hunks))
	}
	if len(ps.Files[1].Hunks) != 1 {
		t.Fatalf("file 1 hunks = %d, want 1", len(ps.Files[1].Hunks))
	}
	got, err := ps.Files[1].Apply("x\n")
	if err != nil {
		t.Fatalf("Apply(b.txt): %v", err)
	}
	if got != "y\n" {
		t.Errorf("Apply(b.txt) = %q, want %q", got, "y\n")
	}
}

// TestParsePatchSetEmpty checks the empty-input shortcut: no files, no error.
func TestParsePatchSetEmpty(t *testing.T) {
	for _, src := range []string{"", "   \n\t\n"} {
		ps, err := ParsePatchSet(src)
		if err != nil {
			t.Errorf("ParsePatchSet(%q) error = %v, want nil", src, err)
		}
		if len(ps.Files) != 0 {
			t.Errorf("ParsePatchSet(%q) files = %d, want 0", src, len(ps.Files))
		}
	}
}

// TestSplitPatchLinesTrailingNewline pins the line splitter the apply path
// depends on: "" is zero lines, and the trailing-newline bit is reported
// separately so it can be preserved or dropped deliberately.
func TestSplitPatchLinesTrailingNewline(t *testing.T) {
	cases := []struct {
		in       string
		want     []string
		trailing bool
	}{
		{"", nil, false},
		{"a\n", []string{"a"}, true},
		{"a", []string{"a"}, false},
		{"a\nb\n", []string{"a", "b"}, true},
		{"a\nb", []string{"a", "b"}, false},
		{"\n", []string{""}, true},
	}
	for _, c := range cases {
		got, trailing := splitPatchLines(c.in)
		if trailing != c.trailing {
			t.Errorf("splitPatchLines(%q) trailing = %v, want %v", c.in, trailing, c.trailing)
		}
		if len(got) != len(c.want) {
			t.Fatalf("splitPatchLines(%q) = %#v, want %#v", c.in, got, c.want)
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("splitPatchLines(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}
