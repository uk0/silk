package gui

import (
	"strconv"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("gui.DiffView", core.TypeOf((*DiffView)(nil)))
}

// DiffRowStatus classifies a single row in the side-by-side diff view.
// The first four states cover every (left, right) line-pairing we emit:
// matched lines on both sides, a line that exists only on the left
// (removed), one that exists only on the right (added), or a row where both
// sides hold a line but they differ (modified). DiffHunkHeader is the fifth
// and only appears in patch mode (SetPatchFile): a full-width "@@ ... @@"
// separator that carries the per-hunk stage/revert actions instead of a
// left/right line pair.
type DiffRowStatus int

const (
	DiffSame       DiffRowStatus = iota // both sides hold the same line
	DiffRemoved                         // old has a line, new does not (left only)
	DiffAdded                           // new has a line, old does not (right only)
	DiffModified                        // both sides hold a line but they differ
	DiffHunkHeader                      // patch mode: "@@ ... @@" separator row
)

// DiffRow is one row in the rendered diff: oldLine renders on the left and
// newLine renders on the right. For DiffRemoved newLine is empty; for
// DiffAdded oldLine is empty; for DiffSame and DiffModified both fields are
// populated. The status drives the per-row background tint.
type DiffRow struct {
	OldLine string
	NewLine string
	Status  DiffRowStatus
}

// DiffView is a two-column line-by-line text diff viewer (Qt Creator's
// "Side-by-Side Diff", simplified). The left column shows the old text,
// the right column shows the new text, with matching lines neutral, lines
// only in the old tinted red on the left, lines only in the new tinted
// green on the right, and rows where both sides differ tinted on both
// sides. A vertical divider sits in the middle and a single shared scroll
// offset keeps the two columns aligned.
//
// Usage:
//
//	dv := gui.NewDiffView()
//	dv.SetTexts(oldSrc, newSrc)
//
// The diff is line-based and computed via a simple LCS pass — the helper
// lineDiff is exported package-locally so it can be unit-tested without
// any widget/GL state.
type DiffView struct {
	Widget

	oldText  string
	newText  string
	oldLines []string
	newLines []string
	diffRows []DiffRow

	scrollY float64

	// activeChangeRow is the index of the row currently highlighted by
	// the n/p navigation keys (and SetActiveChangeRow). -1 means "no
	// active row" — the initial state and what recompute resets to. The
	// row need not be a change row in practice (a click on a Same row
	// will set it), but JumpToNext/Prev only land on non-Same rows.
	activeChangeRow int

	// showGutter toggles the per-side line-number gutter. Default true —
	// hosts that want a bare two-column diff (e.g. an embedded preview in
	// a tooltip) can SetShowGutter(false) to suppress the numbers and
	// reclaim the gutter width for the diff text.
	showGutter bool

	// --- patch mode (SetPatchFile), all zero/nil in plain two-text mode ---

	// patchFile is the last patch handed to SetPatchFile, kept so hosts can
	// read the paths back for a header; patchMode says we are showing it.
	patchFile DiffPatchFile
	patchMode bool

	// rowHunk maps each diffRows index to the hunk it belongs to, or -1 for
	// an unchanged gap row reconstructed from the original file.
	rowHunk []int

	// hunkHeaderRows holds the diffRows index of each hunk's header row, in
	// hunk order — so hunkHeaderRows[i] is where hunk i starts on screen.
	hunkHeaderRows []int

	cbStageHunk  func(hunkIndex int)
	cbRevertHunk func(hunkIndex int)
}

// diffGutterWidth is the fixed pixel width reserved at the left edge of
// each column for the line-number gutter. Wide enough to hold a 4-digit
// number in the default monospace font without crowding the diff text.
const diffGutterWidth = 30.0

// NewDiffView creates an empty diff viewer. Callers populate it with
// SetTexts (or SetOldText/SetNewText) once the two sides are known.
func NewDiffView() *DiffView {
	p := new(DiffView)
	p.Init(p)
	p.activeChangeRow = -1
	p.showGutter = true
	return p
}

// ShowGutter reports whether the per-side line-number gutter is rendered.
func (this *DiffView) ShowGutter() bool { return this.showGutter }

// SetShowGutter toggles the per-side line-number gutter. With the gutter
// off the diff text expands into the reclaimed space; with it on each
// column reserves diffGutterWidth px on the left for the line numbers.
func (this *DiffView) SetShowGutter(b bool) {
	if this.showGutter == b {
		return
	}
	this.showGutter = b
	this.Self().Update()
}

// gutterLineNumbers returns parallel slices of left-side and right-side
// line numbers for each DiffRow. A 0 means "no number on this side":
// added rows have no left counterpart, removed rows have no right
// counterpart. The counters advance only when the row presents content
// on that side — DiffSame and DiffModified bump both, DiffAdded bumps
// only the right, DiffRemoved bumps only the left, and a DiffHunkHeader
// row bumps neither (it is a separator, not a line of either file).
func gutterLineNumbers(rows []DiffRow) (left, right []int) {
	left = make([]int, len(rows))
	right = make([]int, len(rows))
	var lc, rc int
	for i, r := range rows {
		switch r.Status {
		case DiffSame, DiffModified:
			lc++
			rc++
			left[i] = lc
			right[i] = rc
		case DiffAdded:
			rc++
			right[i] = rc
		case DiffRemoved:
			lc++
			left[i] = lc
		}
	}
	return left, right
}

// OldText returns the left-side text.
func (this *DiffView) OldText() string { return this.oldText }

// NewText returns the right-side text.
func (this *DiffView) NewText() string { return this.newText }

// SetTexts replaces both sides of the diff and recomputes the row list in
// one shot, then invalidates the widget. Use this when both sides change
// together to avoid an intermediate render with mismatched content.
func (this *DiffView) SetTexts(oldText, newText string) {
	this.oldText = oldText
	this.newText = newText
	this.recompute()
}

// SetOldText replaces only the left side. The diff is recomputed against
// the current right side so the user sees the new comparison immediately.
func (this *DiffView) SetOldText(s string) {
	this.oldText = s
	this.recompute()
}

// SetNewText replaces only the right side. Symmetric to SetOldText.
func (this *DiffView) SetNewText(s string) {
	this.newText = s
	this.recompute()
}

// DiffRows returns the computed row list. Exposed for tests and host code
// that wants to render its own summary on top of the same diff data.
func (this *DiffView) DiffRows() []DiffRow { return this.diffRows }

// ActiveChangeRow returns the index of the row the n/p navigation last
// landed on (or -1 if none / never used). Hosts that want to drive the
// view themselves can read this back after SetActiveChangeRow.
func (this *DiffView) ActiveChangeRow() int { return this.activeChangeRow }

// NextChangeRow returns the index of the next non-Same row strictly
// after `from`, or -1 if no such row exists. `from < 0` searches from
// row 0 inclusive (i.e. "find the first change from the top"). The
// search does NOT wrap around — past-the-last-change yields -1 so
// JumpToNextChange stops at the bottom rather than cycling.
func (this *DiffView) NextChangeRow(from int) int {
	start := from + 1
	if from < 0 {
		start = 0
	}
	for i := start; i < len(this.diffRows); i++ {
		if this.diffRows[i].Status != DiffSame {
			return i
		}
	}
	return -1
}

// PrevChangeRow returns the index of the previous non-Same row strictly
// before `from`, or -1 if none. `from > len(rows)` searches from the
// end (i.e. "find the last change from the bottom"). Like
// NextChangeRow, the search does NOT wrap around — past-the-first
// yields -1.
func (this *DiffView) PrevChangeRow(from int) int {
	start := from - 1
	if from > len(this.diffRows) {
		start = len(this.diffRows) - 1
	}
	if start >= len(this.diffRows) {
		start = len(this.diffRows) - 1
	}
	for i := start; i >= 0; i-- {
		if this.diffRows[i].Status != DiffSame {
			return i
		}
	}
	return -1
}

// SetActiveChangeRow marks `row` as the active change row and scrolls
// it into view. Passing -1 (or any out-of-range index) clears the
// active row without scrolling. The scroll machinery is the same
// scrollY/lh model OnMouseWheel uses, so the marker stays aligned
// with the per-row tints.
func (this *DiffView) SetActiveChangeRow(row int) {
	if row < 0 || row >= len(this.diffRows) {
		this.activeChangeRow = -1
		this.Self().Update()
		return
	}
	this.activeChangeRow = row
	this.scrollRowIntoView(row)
	this.Self().Update()
}

// scrollRowIntoView nudges scrollY so `row` sits inside the visible
// band. If the row is above the current viewport we top-align it; if
// below we bottom-align it; if already inside we leave scrollY alone.
// We clamp to the same [0, maxScroll] range OnMouseWheel uses so the
// two scroll paths stay consistent.
func (this *DiffView) scrollRowIntoView(row int) {
	fe := Theme().Font.FontExtents()
	lh := fe.Height + 2
	_, h := this.Size()

	rowTop := float64(row) * lh
	rowBot := rowTop + lh

	if rowTop < this.scrollY {
		this.scrollY = rowTop
	} else if rowBot > this.scrollY+h {
		this.scrollY = rowBot - h
	}

	if this.scrollY < 0 {
		this.scrollY = 0
	}
	maxScroll := float64(len(this.diffRows))*lh - h
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
}

// JumpToNextChange advances activeChangeRow to the next non-Same row,
// or no-ops if there isn't one. Wraps NextChangeRow(activeChangeRow)
// so a fresh view (activeChangeRow == -1) lands on the first change.
func (this *DiffView) JumpToNextChange() {
	idx := this.NextChangeRow(this.activeChangeRow)
	if idx >= 0 {
		this.SetActiveChangeRow(idx)
	}
}

// JumpToPrevChange is the symmetric helper for the previous change.
// From activeChangeRow == -1 the search starts past the end of the
// row list, so a fresh "press p" lands on the last change.
func (this *DiffView) JumpToPrevChange() {
	from := this.activeChangeRow
	if from < 0 {
		from = len(this.diffRows) + 1
	}
	idx := this.PrevChangeRow(from)
	if idx >= 0 {
		this.SetActiveChangeRow(idx)
	}
}

// recompute splits the two texts into lines and rebuilds the row list. We
// re-derive both line slices from the raw text on every change so the
// public setters can call us cheaply without juggling intermediate state.
// A nil/empty text yields a nil line slice (rather than [""]) so a "no
// content" side renders as zero rows instead of one phantom blank row.
//
// Setting either text explicitly also leaves patch mode: the rows now come
// from the LCS pass, so the hunk map that SetPatchFile built no longer
// describes them. The stage/revert callbacks stay registered.
func (this *DiffView) recompute() {
	this.oldLines = splitDiffLines(this.oldText)
	this.newLines = splitDiffLines(this.newText)
	this.diffRows = lineDiff(this.oldLines, this.newLines)
	this.patchMode = false
	this.patchFile = DiffPatchFile{}
	this.rowHunk = nil
	this.hunkHeaderRows = nil
	this.scrollY = 0
	this.activeChangeRow = -1
	this.Self().Update()
}

// splitDiffLines splits s on '\n' and treats the empty string as zero
// lines (not one blank line). A trailing newline still produces a final
// empty line — matching how editors render "file ending in newline".
func splitDiffLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// lineDiff computes a side-by-side row list from two line slices using a
// classic LCS table. Lines that appear in the LCS become DiffSame rows;
// runs of unmatched old/new lines between two matches (or at the file
// ends) are paired by position into DiffModified rows for as many rows as
// both sides have content, with the leftover tail emitted as DiffRemoved
// (old-only) or DiffAdded (new-only).
//
// The pairing rule is the load-bearing semantic choice for "two strings
// differ in the same row": old [A B C] vs new [A X C] yields rows
// {A=same, B/X=modified, C=same} rather than {A=same, B=removed, X=added,
// C=same}. This keeps the two columns visually aligned and matches how
// Qt Creator / VS Code render small edits.
//
// The helper is package-private (lower-case) but unit-testable through
// diffview_test.go in the same package.
func lineDiff(oldLines, newLines []string) []DiffRow {
	if len(oldLines) == 0 && len(newLines) == 0 {
		return nil
	}
	m := len(oldLines)
	n := len(newLines)

	// Standard LCS length table, (m+1) x (n+1) ints. For the sizes we
	// expect (a viewer comparing two file snapshots) the O(m*n) cost is
	// fine and the code is small enough to keep in one place.
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if oldLines[i-1] == newLines[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Walk the table back to front, classifying each step as "same",
	// "delete from old", or "insert into new". We collect operations in
	// reverse and reverse the slice at the end.
	type op struct {
		kind   int // 0=same, 1=removed, 2=added
		oldIdx int
		newIdx int
	}
	ops := make([]op, 0, m+n)
	i, j := m, n
	for i > 0 && j > 0 {
		if oldLines[i-1] == newLines[j-1] {
			ops = append(ops, op{kind: 0, oldIdx: i - 1, newIdx: j - 1})
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			ops = append(ops, op{kind: 1, oldIdx: i - 1})
			i--
		} else {
			ops = append(ops, op{kind: 2, newIdx: j - 1})
			j--
		}
	}
	for i > 0 {
		ops = append(ops, op{kind: 1, oldIdx: i - 1})
		i--
	}
	for j > 0 {
		ops = append(ops, op{kind: 2, newIdx: j - 1})
		j--
	}
	// Reverse to forward order.
	for a, b := 0, len(ops)-1; a < b; a, b = a+1, b-1 {
		ops[a], ops[b] = ops[b], ops[a]
	}

	// Emit rows. Buffer pending removed/added runs between matches; when
	// the run ends (next "same" or end of input), pair them position-by-
	// position into DiffModified for the overlap and emit the tail as
	// pure DiffRemoved or DiffAdded.
	var rows []DiffRow
	var pendingRemoved, pendingAdded []int // indices into oldLines / newLines

	flush := func() {
		k := len(pendingRemoved)
		if len(pendingAdded) < k {
			k = len(pendingAdded)
		}
		for x := 0; x < k; x++ {
			rows = append(rows, DiffRow{
				OldLine: oldLines[pendingRemoved[x]],
				NewLine: newLines[pendingAdded[x]],
				Status:  DiffModified,
			})
		}
		for x := k; x < len(pendingRemoved); x++ {
			rows = append(rows, DiffRow{
				OldLine: oldLines[pendingRemoved[x]],
				Status:  DiffRemoved,
			})
		}
		for x := k; x < len(pendingAdded); x++ {
			rows = append(rows, DiffRow{
				NewLine: newLines[pendingAdded[x]],
				Status:  DiffAdded,
			})
		}
		pendingRemoved = pendingRemoved[:0]
		pendingAdded = pendingAdded[:0]
	}

	for _, o := range ops {
		switch o.kind {
		case 0:
			flush()
			rows = append(rows, DiffRow{
				OldLine: oldLines[o.oldIdx],
				NewLine: newLines[o.newIdx],
				Status:  DiffSame,
			})
		case 1:
			pendingRemoved = append(pendingRemoved, o.oldIdx)
		case 2:
			pendingAdded = append(pendingAdded, o.newIdx)
		}
	}
	flush()
	return rows
}

// --- Drawing ---

// diffLinePad is the inner horizontal padding inside each column. Keeping
// it small leaves more room for the actual text in narrow widgets.
const diffLinePad = 6.0

// Draw paints the two columns, the centre divider, and per-row tints.
func (this *DiffView) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Size()

	// Background — neutral form colour so untinted rows blend with the
	// surrounding panel.
	g.SetBrush1(t.FormLightColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	if len(this.diffRows) == 0 {
		// Still draw the divider so the empty viewer reads as a diff
		// widget rather than a blank panel.
		this.drawDivider(g, w, h, t)
		return
	}

	g.SetFont(t.Font)
	f := t.Font
	fe := f.FontExtents()
	lh := fe.Height + 2

	colW := w / 2
	leftX := 0.0
	rightX := colW

	// Reserve a small gutter on the left edge of each column for the
	// per-side line numbers. The diff text starts after the gutter so it
	// never overlaps the numbers. Toggling SetShowGutter(false) zeros
	// the reservation and gives the text the whole column.
	var gw float64
	if this.showGutter {
		gw = diffGutterWidth
	}

	// Per-row tints. Light red/green washes — alpha kept moderate so the
	// foreground text stays readable against the form background.
	colRemoved := paint.Color{R: 255, G: 220, B: 220, A: 180}
	colAdded := paint.Color{R: 220, G: 245, B: 220, A: 180}
	colStripe := paint.Color{R: 0, G: 0, B: 0, A: 8} // subtle alternate stripe

	// Dimmed gutter foreground — theme text colour at ~45% alpha so the
	// numbers stay legible without competing with the diff text.
	gutterFg := t.TextColor
	gutterFg.A = 115

	// Precompute the per-side line numbers once per Draw; cheap and keeps
	// the inner loop free of branching state.
	var leftNums, rightNums []int
	if this.showGutter {
		leftNums, rightNums = gutterLineNumbers(this.diffRows)
	}

	startRow := int(this.scrollY / lh)
	if startRow < 0 {
		startRow = 0
	}
	visibleRows := int(h/lh) + 2

	for row := startRow; row < startRow+visibleRows && row < len(this.diffRows); row++ {
		dr := this.diffRows[row]
		y := float64(row)*lh - this.scrollY

		// Patch mode: a hunk header spans both columns and shows the
		// per-hunk actions rather than a left/right line pair.
		if dr.Status == DiffHunkHeader {
			this.drawHunkHeaderRow(g, dr.OldLine, y, w, lh, fe.Ascent, t)
			continue
		}

		// Alternating stripe for "same" rows so the eye can track lines
		// across the divider without losing its place.
		if dr.Status == DiffSame && row%2 == 1 {
			g.SetBrush1(colStripe)
			g.Rectangle(0, y, w, lh)
			g.Fill()
		}

		// Left-side tint: removed or modified.
		if dr.Status == DiffRemoved || dr.Status == DiffModified {
			g.SetBrush1(colRemoved)
			g.Rectangle(leftX, y, colW, lh)
			g.Fill()
		}
		// Right-side tint: added or modified.
		if dr.Status == DiffAdded || dr.Status == DiffModified {
			g.SetBrush1(colAdded)
			g.Rectangle(rightX, y, colW, lh)
			g.Fill()
		}

		// Text baseline within the row.
		ty := y + fe.Ascent + 1

		// Line-number gutter — right-aligned, dimmed, monospace. Drawn
		// before the diff text so the tint sits behind both. A zero
		// number renders nothing (added rows have no left counterpart,
		// removed rows have no right counterpart).
		if this.showGutter {
			g.SetBrush1(gutterFg)
			if leftNums[row] > 0 {
				s := strconv.Itoa(leftNums[row])
				tw := f.TextExtents(s).Width
				g.DrawText1(leftX+gw-diffLinePad-tw, ty, s)
			}
			if rightNums[row] > 0 {
				s := strconv.Itoa(rightNums[row])
				tw := f.TextExtents(s).Width
				g.DrawText1(rightX+gw-diffLinePad-tw, ty, s)
			}
		}

		// Foreground colour matches the theme text colour; tints supply
		// the per-row status colour. Text starts after the gutter so it
		// doesn't overlap the numbers.
		g.SetBrush1(t.TextColor)
		if dr.OldLine != "" {
			g.DrawText1(leftX+gw+diffLinePad, ty, dr.OldLine)
		}
		if dr.NewLine != "" {
			g.DrawText1(rightX+gw+diffLinePad, ty, dr.NewLine)
		}
	}

	this.drawDivider(g, w, h, t)
	this.drawActiveMarker(g, w, h, lh)
}

// drawActiveMarker paints a thin accent stripe on the left edge of the
// active change row so the user can see where the n/p cursor is. Only
// drawn when activeChangeRow is in range and the row is on-screen.
// Width is 3px — wide enough to read at a glance, narrow enough not to
// eat into the left column's text.
func (this *DiffView) drawActiveMarker(g paint.Painter, w, h, lh float64) {
	if this.activeChangeRow < 0 || this.activeChangeRow >= len(this.diffRows) {
		return
	}
	y := float64(this.activeChangeRow)*lh - this.scrollY
	if y+lh <= 0 || y >= h {
		return
	}
	const markerW = 3.0
	// Accent blue — distinct from the red/green row tints so the marker
	// stays legible against any row status.
	g.SetBrush1(paint.Color{R: 30, G: 110, B: 220, A: 255})
	g.Rectangle(0, y, markerW, lh)
	g.Fill()
}

// drawDivider paints the vertical separator between the two columns.
func (this *DiffView) drawDivider(g paint.Painter, w, h float64, t *defaultTheme) {
	mid := w / 2
	g.SetPen1(t.FormDarkColor, 1)
	g.MoveTo(mid, 0)
	g.LineTo(mid, h)
	g.Stroke()
}

// SizeHints returns the default footprint for a diff viewer: wide enough
// to hold two reasonable columns of monospaced text and tall enough for
// several lines without scrolling.
func (this *DiffView) SizeHints() SizeHints {
	return SizeHints{
		Width:  480,
		Height: 240,
		Policy: GrowHorizontal | GrowVertical,
	}
}

// OnMouseWheel scrolls both columns together. We measure the line height
// from the theme font so the step matches the rendered row size.
func (this *DiffView) OnMouseWheel(x, y, z float64) {
	fe := Theme().Font.FontExtents()
	lh := fe.Height + 2

	this.scrollY -= z * 3 * lh
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := float64(len(this.diffRows))*lh - h
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// OnLeftDown grabs focus so subsequent wheel and key events route here,
// and sets the active change row to whatever row the click landed on
// when that row is a change. Clicks on Same rows just take focus
// without disturbing the navigation cursor — landing the cursor on a
// non-change row would be surprising relative to the n/p behaviour.
// In patch mode a click inside a hunk header's action zone fires the
// stage/revert callback and consumes the click instead.
func (this *DiffView) OnLeftDown(x, y float64) {
	this.SetFocus()
	if len(this.diffRows) == 0 {
		return
	}
	fe := Theme().Font.FontExtents()
	lh := fe.Height + 2
	row := int((y + this.scrollY) / lh)
	if row < 0 || row >= len(this.diffRows) {
		return
	}
	if this.ActivateHunkAction(row, x) {
		return
	}
	if this.diffRows[row].Status == DiffSame {
		return
	}
	this.SetActiveChangeRow(row)
}

// OnKeyDown wires n/p to JumpToNextChange / JumpToPrevChange. Letter
// keys arrive as uppercase ASCII (see keyboard_glfw.go's A-Z mapping),
// which is the same convention ComboBox's type-ahead relies on.
func (this *DiffView) OnKeyDown(key int, repeat bool) {
	switch key {
	case 'N':
		this.JumpToNextChange()
	case 'P':
		this.JumpToPrevChange()
	}
}

// --- Patch mode: whole-file view of one file's patch, with hunk actions ---

// DiffPatchLineKind classifies one line of a hunk body handed to
// SetPatchFile. It mirrors core.PatchLineKind but is declared here so the
// widget stays independent of the patch parser: any host that can produce
// context/added/deleted lines can drive the view.
type DiffPatchLineKind int

const (
	DiffPatchContext DiffPatchLineKind = iota // unchanged, on both sides
	DiffPatchAdded                            // only in the new file
	DiffPatchDeleted                          // only in the old file
)

// DiffPatchLine is one line of a hunk body, without the +/-/space marker.
type DiffPatchLine struct {
	Kind DiffPatchLineKind
	Text string
}

// DiffPatchHunk is one hunk of a file patch. Header is the pre-rendered
// "@@ -a,b +c,d @@" text for the header row; when empty the view renders one
// from the four range fields. OldStart is 1-based and is what positions the
// hunk against OldText so the unchanged gaps land in the right place.
type DiffPatchHunk struct {
	Header   string
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Lines    []DiffPatchLine
}

// DiffPatchFile is one file's patch plus the original content it applies to.
// OldText is what lets the view show the WHOLE file: the unchanged gaps
// between hunks come from it. With OldText empty the view degrades to the
// changed neighbourhoods only (still with header rows and hunk actions),
// which is all a patch without its original file can show.
type DiffPatchFile struct {
	OldPath string
	NewPath string
	OldText string
	Hunks   []DiffPatchHunk
}

// DiffHunkAction names the clickable affordances on a hunk header row.
type DiffHunkAction int

const (
	DiffHunkActionNone   DiffHunkAction = iota // click landed outside both zones
	DiffHunkActionStage                        // "stage this hunk"
	DiffHunkActionRevert                       // "revert this hunk"
)

const (
	// diffHunkActionW is the width of each action hot zone at the right edge
	// of a hunk header row: [w-2*W, w-W) stages, [w-W, w) reverts. Fixed so
	// the hit test is pure geometry — no font measuring, no render state.
	diffHunkActionW = 60.0

	diffStageLabel  = "暂存"
	diffRevertLabel = "还原"
)

// NewDiffPatchFile converts a parsed core.FilePatch plus the original file
// content into the plain struct SetPatchFile takes. Hosts that already speak
// core.ParsePatchSet get the whole-file view in two calls.
func NewDiffPatchFile(f core.FilePatch, original string) DiffPatchFile {
	out := DiffPatchFile{OldPath: f.OldPath, NewPath: f.NewPath, OldText: original}
	for _, h := range f.Hunks {
		dh := DiffPatchHunk{
			Header:   h.Header(),
			OldStart: h.OldStart,
			OldLines: h.OldLines,
			NewStart: h.NewStart,
			NewLines: h.NewLines,
		}
		for _, ln := range h.Lines {
			k := DiffPatchContext
			switch ln.Kind {
			case core.PatchAdded:
				k = DiffPatchAdded
			case core.PatchDeleted:
				k = DiffPatchDeleted
			}
			dh.Lines = append(dh.Lines, DiffPatchLine{Kind: k, Text: ln.Text})
		}
		out.Hunks = append(out.Hunks, dh)
	}
	return out
}

// SetPatchFile shows one file's patch as a whole-file side-by-side diff: the
// unchanged gaps between hunks are reconstructed from f.OldText, each hunk is
// introduced by a header row carrying the stage/revert actions, and the two
// columns end up holding the complete old and new file text (readable back
// through OldText/NewText).
//
// This is the multi-hunk, gap-aware counterpart to SetTexts. Calling either
// SetTexts setter afterwards returns the view to plain two-text mode.
func (this *DiffView) SetPatchFile(f DiffPatchFile) {
	b := buildPatchRows(f)

	this.patchFile = f
	this.patchMode = true
	this.oldText = b.oldText
	this.newText = b.newText
	this.oldLines = splitDiffLines(b.oldText)
	this.newLines = splitDiffLines(b.newText)
	this.diffRows = b.rows
	this.rowHunk = b.rowHunk
	this.hunkHeaderRows = b.headerRows
	this.scrollY = 0
	this.activeChangeRow = -1
	this.Self().Update()
}

// PatchFile returns the patch last handed to SetPatchFile (zero value when
// the view is in plain two-text mode).
func (this *DiffView) PatchFile() DiffPatchFile { return this.patchFile }

// IsPatchMode reports whether the rows came from SetPatchFile.
func (this *DiffView) IsPatchMode() bool { return this.patchMode }

// HunkCount is the number of hunks currently on screen.
func (this *DiffView) HunkCount() int { return len(this.hunkHeaderRows) }

// HunkHeaderRows returns a copy of the row index of each hunk's header row,
// in hunk order. Hosts use it to scroll a hunk into view.
func (this *DiffView) HunkHeaderRows() []int {
	if len(this.hunkHeaderRows) == 0 {
		return nil
	}
	out := make([]int, len(this.hunkHeaderRows))
	copy(out, this.hunkHeaderRows)
	return out
}

// HunkIndexAtRow maps a row index to the hunk it belongs to — header row and
// body rows alike — or -1 for an unchanged gap row, an out-of-range row, or
// any row in plain two-text mode.
func (this *DiffView) HunkIndexAtRow(row int) int {
	if row < 0 || row >= len(this.rowHunk) {
		return -1
	}
	return this.rowHunk[row]
}

// SigStageHunk registers the callback fired when the user clicks a hunk
// header's stage affordance. The argument is the hunk index, which indexes
// straight into the hunk slice the host built the DiffPatchFile from (and
// therefore into core.FilePatch.Hunks / ApplySelected).
func (this *DiffView) SigStageHunk(fn func(hunkIndex int)) { this.cbStageHunk = fn }

// SigRevertHunk is the symmetric hook for the revert affordance.
func (this *DiffView) SigRevertHunk(fn func(hunkIndex int)) { this.cbRevertHunk = fn }

// HunkActionAt hit-tests a click at column x on `row`. It returns the hunk
// index and which action zone was hit; a row that is not a hunk header
// yields (-1, DiffHunkActionNone), and a header row clicked outside both
// zones yields (hunkIndex, DiffHunkActionNone). Pure geometry against the
// widget size, so it is testable without any render state.
func (this *DiffView) HunkActionAt(row int, x float64) (hunkIndex int, action DiffHunkAction) {
	if row < 0 || row >= len(this.diffRows) || this.diffRows[row].Status != DiffHunkHeader {
		return -1, DiffHunkActionNone
	}
	hunkIndex = this.HunkIndexAtRow(row)
	if hunkIndex < 0 {
		return -1, DiffHunkActionNone
	}
	w, _ := this.Size()
	stageX := w - 2*diffHunkActionW
	if stageX < 0 {
		// Too narrow to place the labels; the header is text-only.
		return hunkIndex, DiffHunkActionNone
	}
	switch {
	case x >= w-diffHunkActionW:
		return hunkIndex, DiffHunkActionRevert
	case x >= stageX:
		return hunkIndex, DiffHunkActionStage
	}
	return hunkIndex, DiffHunkActionNone
}

// ActivateHunkAction fires the stage/revert callback for a click at (row, x)
// and reports whether the click hit an action zone. A hit consumes the click
// even with no callback registered, so the header row never doubles as a
// change-row selection.
func (this *DiffView) ActivateHunkAction(row int, x float64) bool {
	hunkIndex, action := this.HunkActionAt(row, x)
	if hunkIndex < 0 {
		return false
	}
	switch action {
	case DiffHunkActionStage:
		if this.cbStageHunk != nil {
			this.cbStageHunk(hunkIndex)
		}
		return true
	case DiffHunkActionRevert:
		if this.cbRevertHunk != nil {
			this.cbRevertHunk(hunkIndex)
		}
		return true
	}
	return false
}

// patchRowBuild is the output of buildPatchRows: the row list plus the maps
// that tie rows back to hunks, and the two reconstructed full texts.
type patchRowBuild struct {
	rows       []DiffRow
	rowHunk    []int
	headerRows []int
	oldText    string
	newText    string
}

// buildPatchRows lays a file patch out as diff rows. For each hunk it emits
// the unchanged gap that precedes it (taken from f.OldText, which is what the
// old hunks-only rendering dropped), then a header row, then the body with
// deleted/added runs paired position-by-position into DiffModified rows so
// the two columns stay aligned — the same pairing rule lineDiff uses.
//
// Because the gaps are filled in, the emitted rows reconstruct the complete
// old and new files: walking the rows and collecting each side's content
// yields the two texts, and the gutter numbering therefore reads as real file
// line numbers. Pure function, no widget state.
func buildPatchRows(f DiffPatchFile) patchRowBuild {
	var b patchRowBuild
	oldLines := splitDiffLines(f.OldText)
	var oldRecon, newRecon []string

	emit := func(r DiffRow, hunk int) {
		b.rows = append(b.rows, r)
		b.rowHunk = append(b.rowHunk, hunk)
		switch r.Status {
		case DiffSame, DiffModified:
			oldRecon = append(oldRecon, r.OldLine)
			newRecon = append(newRecon, r.NewLine)
		case DiffRemoved:
			oldRecon = append(oldRecon, r.OldLine)
		case DiffAdded:
			newRecon = append(newRecon, r.NewLine)
		}
	}

	cursor := 0 // next original line (0-based) not yet emitted
	for hi, h := range f.Hunks {
		// Unchanged gap before this hunk. Clamped so a bogus/overlapping
		// OldStart can never index outside the original file.
		start := h.OldStart - 1
		if start < cursor {
			start = cursor
		}
		if start > len(oldLines) {
			start = len(oldLines)
		}
		for ; cursor < start; cursor++ {
			emit(DiffRow{OldLine: oldLines[cursor], NewLine: oldLines[cursor], Status: DiffSame}, -1)
		}

		emit(DiffRow{OldLine: patchHunkHeaderText(h), Status: DiffHunkHeader}, hi)
		b.headerRows = append(b.headerRows, len(b.rows)-1)

		var pendDel, pendAdd []string
		flush := func() {
			k := len(pendDel)
			if len(pendAdd) < k {
				k = len(pendAdd)
			}
			for x := 0; x < k; x++ {
				emit(DiffRow{OldLine: pendDel[x], NewLine: pendAdd[x], Status: DiffModified}, hi)
			}
			for x := k; x < len(pendDel); x++ {
				emit(DiffRow{OldLine: pendDel[x], Status: DiffRemoved}, hi)
			}
			for x := k; x < len(pendAdd); x++ {
				emit(DiffRow{NewLine: pendAdd[x], Status: DiffAdded}, hi)
			}
			pendDel = pendDel[:0]
			pendAdd = pendAdd[:0]
		}

		for _, ln := range h.Lines {
			switch ln.Kind {
			case DiffPatchContext:
				flush()
				emit(DiffRow{OldLine: ln.Text, NewLine: ln.Text, Status: DiffSame}, hi)
				cursor++
			case DiffPatchDeleted:
				pendDel = append(pendDel, ln.Text)
				cursor++
			case DiffPatchAdded:
				pendAdd = append(pendAdd, ln.Text)
			}
		}
		flush()

		if cursor > len(oldLines) {
			cursor = len(oldLines)
		}
	}

	// Unchanged tail after the last hunk.
	for ; cursor < len(oldLines); cursor++ {
		emit(DiffRow{OldLine: oldLines[cursor], NewLine: oldLines[cursor], Status: DiffSame}, -1)
	}

	b.oldText = strings.Join(oldRecon, "\n")
	b.newText = strings.Join(newRecon, "\n")
	return b
}

// patchHunkHeaderText is the text of a hunk's header row: the host-supplied
// Header when present, otherwise one rendered from the ranges (dropping the
// ",1" count git elides).
func patchHunkHeaderText(h DiffPatchHunk) string {
	if h.Header != "" {
		return h.Header
	}
	return "@@ -" + diffRangeText(h.OldStart, h.OldLines) +
		" +" + diffRangeText(h.NewStart, h.NewLines) + " @@"
}

// diffRangeText renders "start,count", dropping a count of 1.
func diffRangeText(start, count int) string {
	if count == 1 {
		return strconv.Itoa(start)
	}
	return strconv.Itoa(start) + "," + strconv.Itoa(count)
}

// drawHunkHeaderRow paints one hunk header: a muted band across both
// columns, the "@@ ... @@" text on the left, and the two action labels in
// the right-edge zones HunkActionAt hit-tests.
func (this *DiffView) drawHunkHeaderRow(g paint.Painter, text string, y, w, lh, ascent float64, t *defaultTheme) {
	g.SetBrush1(paint.Color{R: 225, G: 232, B: 245, A: 220})
	g.Rectangle(0, y, w, lh)
	g.Fill()

	ty := y + ascent + 1
	hdr := t.TextColor
	hdr.A = 200
	g.SetBrush1(hdr)
	g.DrawText1(diffLinePad, ty, text)

	if w-2*diffHunkActionW < 0 {
		return
	}
	g.SetBrush1(paint.Color{R: 30, G: 110, B: 220, A: 255})
	g.DrawText1(w-2*diffHunkActionW+diffLinePad, ty, diffStageLabel)
	g.DrawText1(w-diffHunkActionW+diffLinePad, ty, diffRevertLabel)
}

// EnumProperties exposes the two texts to the property sheet so the
// designer can preview the widget with sample content.
func (this *DiffView) EnumProperties(list core.IPropertyList) {
	list.AddProperty("旧文本", this.OldText, this.SetOldText)
	list.AddProperty("新文本", this.NewText, this.SetNewText)
}
