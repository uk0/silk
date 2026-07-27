package gui

import (
	"strings"
	"testing"

	"github.com/uk0/silk/core"
)

// patchTenLineFile is the "original" the fixtures patch: ten lines, no
// trailing newline (so the row count is exactly ten content rows and the
// assertions stay readable).
const patchTenLineFile = "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10"

// patchFixture is a hand-built two-hunk patch over patchTenLineFile: l3
// becomes L3 and l9 becomes L9, leaving l5..l7 as an untouched gap between
// the hunks. Header is left empty on purpose so the fallback header text gets
// exercised.
func patchFixture() DiffPatchFile {
	return DiffPatchFile{
		OldPath: "f.txt",
		NewPath: "f.txt",
		OldText: patchTenLineFile,
		Hunks: []DiffPatchHunk{
			{
				OldStart: 2, OldLines: 3, NewStart: 2, NewLines: 3,
				Lines: []DiffPatchLine{
					{Kind: DiffPatchContext, Text: "l2"},
					{Kind: DiffPatchDeleted, Text: "l3"},
					{Kind: DiffPatchAdded, Text: "L3"},
					{Kind: DiffPatchContext, Text: "l4"},
				},
			},
			{
				OldStart: 8, OldLines: 3, NewStart: 8, NewLines: 3,
				Lines: []DiffPatchLine{
					{Kind: DiffPatchContext, Text: "l8"},
					{Kind: DiffPatchDeleted, Text: "l9"},
					{Kind: DiffPatchAdded, Text: "L9"},
					{Kind: DiffPatchContext, Text: "l10"},
				},
			},
		},
	}
}

// TestDiffViewSetPatchFileRebuildsGaps is the core of the whole-file view: the
// unchanged lines git elides between hunks (l1, l5..l7) come back from
// OldText, each hunk gets a header row, and the two columns end up holding the
// complete old and new file text.
func TestDiffViewSetPatchFileRebuildsGaps(t *testing.T) {
	dv := NewDiffView()
	dv.SetPatchFile(patchFixture())

	if !dv.IsPatchMode() {
		t.Fatal("IsPatchMode() = false after SetPatchFile")
	}
	rows := dv.DiffRows()
	// 10 file lines (l3/l9 paired into their replacements) + 2 header rows.
	if len(rows) != 12 {
		t.Fatalf("rows = %d, want 12: %+v", len(rows), rows)
	}

	type want struct {
		old, new string
		status   DiffRowStatus
	}
	expect := []want{
		{"l1", "l1", DiffSame},                  // gap before hunk 0
		{"@@ -2,3 +2,3 @@", "", DiffHunkHeader}, // hunk 0 header
		{"l2", "l2", DiffSame},
		{"l3", "L3", DiffModified},
		{"l4", "l4", DiffSame},
		{"l5", "l5", DiffSame}, // gap between the hunks
		{"l6", "l6", DiffSame},
		{"l7", "l7", DiffSame},
		{"@@ -8,3 +8,3 @@", "", DiffHunkHeader}, // hunk 1 header
		{"l8", "l8", DiffSame},
		{"l9", "L9", DiffModified},
		{"l10", "l10", DiffSame},
	}
	for i, w := range expect {
		got := rows[i]
		if got.OldLine != w.old || got.NewLine != w.new || got.Status != w.status {
			t.Errorf("row %d = {%q, %q, %v}, want {%q, %q, %v}",
				i, got.OldLine, got.NewLine, got.Status, w.old, w.new, w.status)
		}
	}

	if got := dv.OldText(); got != patchTenLineFile {
		t.Errorf("OldText() = %q, want the whole original %q", got, patchTenLineFile)
	}
	wantNew := "l1\nl2\nL3\nl4\nl5\nl6\nl7\nl8\nL9\nl10"
	if got := dv.NewText(); got != wantNew {
		t.Errorf("NewText() = %q, want %q", got, wantNew)
	}
	if got := dv.PatchFile().NewPath; got != "f.txt" {
		t.Errorf("PatchFile().NewPath = %q, want f.txt", got)
	}
}

// TestDiffViewPatchHunkMapping pins the row->hunk map the actions ride on:
// header and body rows report their hunk, reconstructed gap rows report -1.
func TestDiffViewPatchHunkMapping(t *testing.T) {
	dv := NewDiffView()
	dv.SetPatchFile(patchFixture())

	if got := dv.HunkCount(); got != 2 {
		t.Fatalf("HunkCount() = %d, want 2", got)
	}
	hdr := dv.HunkHeaderRows()
	if len(hdr) != 2 || hdr[0] != 1 || hdr[1] != 8 {
		t.Fatalf("HunkHeaderRows() = %v, want [1 8]", hdr)
	}
	// The returned slice is a copy — mutating it must not disturb the view.
	hdr[0] = 999
	if again := dv.HunkHeaderRows(); again[0] != 1 {
		t.Errorf("HunkHeaderRows() did not return a copy: %v", again)
	}

	cases := []struct {
		row  int
		want int
	}{
		{0, -1},  // gap before hunk 0
		{1, 0},   // hunk 0 header
		{2, 0},   // hunk 0 context
		{3, 0},   // hunk 0 change
		{4, 0},   // hunk 0 context
		{5, -1},  // gap
		{7, -1},  // gap
		{8, 1},   // hunk 1 header
		{10, 1},  // hunk 1 change
		{11, 1},  // hunk 1 context
		{-1, -1}, // out of range
		{99, -1}, // out of range
	}
	for _, c := range cases {
		if got := dv.HunkIndexAtRow(c.row); got != c.want {
			t.Errorf("HunkIndexAtRow(%d) = %d, want %d", c.row, got, c.want)
		}
	}
}

// TestDiffViewPatchGutterNumbers checks the property the gap reconstruction
// buys: because every file line is present, the per-side gutter numbers are
// real file line numbers, and header rows claim a number on neither side.
func TestDiffViewPatchGutterNumbers(t *testing.T) {
	dv := NewDiffView()
	dv.SetPatchFile(patchFixture())

	left, right := gutterLineNumbers(dv.DiffRows())
	for _, row := range []int{1, 8} {
		if left[row] != 0 || right[row] != 0 {
			t.Errorf("header row %d numbers = (%d, %d), want (0, 0)", row, left[row], right[row])
		}
	}
	// row -> (left, right) for a few anchors: l1 is line 1 on both sides,
	// the l3/L3 row is line 3, and l10 is line 10.
	anchors := []struct {
		row, l, r int
	}{
		{0, 1, 1},
		{3, 3, 3},
		{7, 7, 7},
		{11, 10, 10},
	}
	for _, a := range anchors {
		if left[a.row] != a.l || right[a.row] != a.r {
			t.Errorf("row %d numbers = (%d, %d), want (%d, %d)", a.row, left[a.row], right[a.row], a.l, a.r)
		}
	}
}

// TestDiffViewHunkActionHitTest exercises the pure geometry of the header-row
// affordances: with a 480px wide view, stage owns [360, 420) and revert
// [420, 480). Non-header rows never report an action.
func TestDiffViewHunkActionHitTest(t *testing.T) {
	dv := NewDiffView()
	dv.SetPatchFile(patchFixture())
	dv.SetSize(480, 240)

	cases := []struct {
		row      int
		x        float64
		wantHunk int
		wantAct  DiffHunkAction
	}{
		{1, 470, 0, DiffHunkActionRevert},
		{1, 425, 0, DiffHunkActionRevert},
		{1, 419, 0, DiffHunkActionStage},
		{1, 360, 0, DiffHunkActionStage},
		{1, 359, 0, DiffHunkActionNone}, // header text area
		{1, 10, 0, DiffHunkActionNone},
		{8, 470, 1, DiffHunkActionRevert},
		{8, 380, 1, DiffHunkActionStage},
		{0, 470, -1, DiffHunkActionNone}, // gap row
		{3, 470, -1, DiffHunkActionNone}, // hunk body row
		{-1, 470, -1, DiffHunkActionNone},
		{99, 470, -1, DiffHunkActionNone},
	}
	for _, c := range cases {
		gotHunk, gotAct := dv.HunkActionAt(c.row, c.x)
		if gotHunk != c.wantHunk || gotAct != c.wantAct {
			t.Errorf("HunkActionAt(%d, %g) = (%d, %v), want (%d, %v)",
				c.row, c.x, gotHunk, gotAct, c.wantHunk, c.wantAct)
		}
	}

	// Too narrow for the labels: the header is text-only, no actions.
	dv.SetSize(80, 240)
	if hunk, act := dv.HunkActionAt(1, 70); hunk != 0 || act != DiffHunkActionNone {
		t.Errorf("narrow view HunkActionAt(1, 70) = (%d, %v), want (0, None)", hunk, act)
	}
}

// TestDiffViewHunkActionCallbacks verifies SigStageHunk / SigRevertHunk fire
// with the clicked hunk's index, and that a click outside the zones neither
// fires a callback nor claims the click.
func TestDiffViewHunkActionCallbacks(t *testing.T) {
	dv := NewDiffView()
	dv.SetPatchFile(patchFixture())
	dv.SetSize(480, 240)

	var staged, reverted []int
	dv.SigStageHunk(func(i int) { staged = append(staged, i) })
	dv.SigRevertHunk(func(i int) { reverted = append(reverted, i) })

	if !dv.ActivateHunkAction(1, 380) {
		t.Error("ActivateHunkAction(1, 380) = false, want true (stage zone)")
	}
	if !dv.ActivateHunkAction(8, 470) {
		t.Error("ActivateHunkAction(8, 470) = false, want true (revert zone)")
	}
	if dv.ActivateHunkAction(1, 10) {
		t.Error("ActivateHunkAction(1, 10) = true, want false (header text area)")
	}
	if dv.ActivateHunkAction(3, 470) {
		t.Error("ActivateHunkAction(3, 470) = true, want false (body row)")
	}

	if len(staged) != 1 || staged[0] != 0 {
		t.Errorf("staged = %v, want [0]", staged)
	}
	if len(reverted) != 1 || reverted[0] != 1 {
		t.Errorf("reverted = %v, want [1]", reverted)
	}

	// With no callbacks registered a zone hit still consumes the click, so
	// the header row never doubles as a change-row selection.
	bare := NewDiffView()
	bare.SetPatchFile(patchFixture())
	bare.SetSize(480, 240)
	if !bare.ActivateHunkAction(1, 470) {
		t.Error("unwired view: ActivateHunkAction(1, 470) = false, want true")
	}
	if bare.ActiveChangeRow() != -1 {
		t.Errorf("unwired view: ActiveChangeRow() = %d, want -1", bare.ActiveChangeRow())
	}
}

// TestDiffViewSetTextsLeavesPatchMode checks the mode boundary: going back to
// the plain two-text API drops the hunk map so a stale hunk index can never
// reach a callback.
func TestDiffViewSetTextsLeavesPatchMode(t *testing.T) {
	dv := NewDiffView()
	dv.SetPatchFile(patchFixture())
	dv.SetSize(480, 240)

	dv.SetTexts("a\nb", "a\nc")
	if dv.IsPatchMode() {
		t.Error("IsPatchMode() = true after SetTexts")
	}
	if got := dv.HunkCount(); got != 0 {
		t.Errorf("HunkCount() = %d, want 0", got)
	}
	if got := dv.HunkHeaderRows(); got != nil {
		t.Errorf("HunkHeaderRows() = %v, want nil", got)
	}
	if got := dv.HunkIndexAtRow(0); got != -1 {
		t.Errorf("HunkIndexAtRow(0) = %d, want -1", got)
	}
	if got := dv.PatchFile(); got.NewPath != "" || len(got.Hunks) != 0 {
		t.Errorf("PatchFile() = %+v, want the zero value", got)
	}
	if hunk, act := dv.HunkActionAt(1, 470); hunk != -1 || act != DiffHunkActionNone {
		t.Errorf("HunkActionAt after SetTexts = (%d, %v), want (-1, None)", hunk, act)
	}
}

// TestDiffViewSetPatchFileWithoutOriginal covers the degraded case: with no
// OldText there are no gaps to rebuild, so the view shows the changed
// neighbourhoods only — still with header rows and working hunk actions.
func TestDiffViewSetPatchFileWithoutOriginal(t *testing.T) {
	f := patchFixture()
	f.OldText = ""

	dv := NewDiffView()
	dv.SetPatchFile(f)

	rows := dv.DiffRows()
	if len(rows) != 8 {
		t.Fatalf("rows = %d, want 8 (2 headers + 3 body rows each): %+v", len(rows), rows)
	}
	if rows[0].Status != DiffHunkHeader || rows[4].Status != DiffHunkHeader {
		t.Errorf("header rows = (%v, %v), want both DiffHunkHeader", rows[0].Status, rows[4].Status)
	}
	if got := dv.HunkHeaderRows(); len(got) != 2 || got[0] != 0 || got[1] != 4 {
		t.Errorf("HunkHeaderRows() = %v, want [0 4]", got)
	}
	if got := dv.OldText(); got != "l2\nl3\nl4\nl8\nl9\nl10" {
		t.Errorf("OldText() = %q, want the hunk bodies only", got)
	}
}

// TestNewDiffPatchFileFromCore bridges the two halves: a diff parsed by
// core.ParsePatchSet converts into the widget's plain struct, and the view's
// reconstructed new side matches what core.ApplyPatch produces for the same
// original — two independent paths that must agree.
func TestNewDiffPatchFileFromCore(t *testing.T) {
	src := `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -2,3 +2,3 @@ func f()
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
	ps, err := core.ParsePatchSet(src)
	if err != nil {
		t.Fatalf("core.ParsePatchSet: %v", err)
	}
	if len(ps.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(ps.Files))
	}
	cf := ps.Files[0]

	f := NewDiffPatchFile(cf, patchTenLineFile)
	if f.OldPath != "f.txt" || f.NewPath != "f.txt" || f.OldText != patchTenLineFile {
		t.Fatalf("converted file = %+v, want f.txt with the original text", f)
	}
	if len(f.Hunks) != 2 {
		t.Fatalf("converted hunks = %d, want 2", len(f.Hunks))
	}
	if got := f.Hunks[0].Header; got != "@@ -2,3 +2,3 @@ func f()" {
		t.Errorf("hunk 0 header = %q, want the core-rendered header with its section", got)
	}
	wantKinds := []DiffPatchLineKind{DiffPatchContext, DiffPatchDeleted, DiffPatchAdded, DiffPatchContext}
	for i, k := range wantKinds {
		if got := f.Hunks[0].Lines[i].Kind; got != k {
			t.Errorf("hunk 0 line %d kind = %v, want %v", i, got, k)
		}
	}

	dv := NewDiffView()
	dv.SetPatchFile(f)
	applied, err := cf.Apply(patchTenLineFile)
	if err != nil {
		t.Fatalf("core Apply: %v", err)
	}
	// The view is line-based and has no notion of a file terminator (a
	// trailing newline would render as one more empty row), so the
	// comparison is against core's content minus its terminator.
	if want := strings.TrimSuffix(applied, "\n"); dv.NewText() != want {
		t.Errorf("view new side = %q, core Apply = %q: the two must agree", dv.NewText(), want)
	}
	if dv.OldText() != patchTenLineFile {
		t.Errorf("view old side = %q, want the original", dv.OldText())
	}
	// The header row text carries core's section hint through to the view.
	if rows := dv.DiffRows(); rows[1].OldLine != "@@ -2,3 +2,3 @@ func f()" {
		t.Errorf("header row text = %q, want the core-rendered header", rows[1].OldLine)
	}
}
