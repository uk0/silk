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
// The four states cover every (left, right) line-pairing we emit: matched
// lines on both sides, a line that exists only on the left (removed), one
// that exists only on the right (added), or a row where both sides hold a
// line but they differ (modified).
type DiffRowStatus int

const (
	DiffSame     DiffRowStatus = iota // both sides hold the same line
	DiffRemoved                       // old has a line, new does not (left only)
	DiffAdded                         // new has a line, old does not (right only)
	DiffModified                      // both sides hold a line but they differ
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
// only the right, DiffRemoved bumps only the left.
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
func (this *DiffView) recompute() {
	this.oldLines = splitDiffLines(this.oldText)
	this.newLines = splitDiffLines(this.newText)
	this.diffRows = lineDiff(this.oldLines, this.newLines)
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

// EnumProperties exposes the two texts to the property sheet so the
// designer can preview the widget with sample content.
func (this *DiffView) EnumProperties(list core.IPropertyList) {
	list.AddProperty("旧文本", this.OldText, this.SetOldText)
	list.AddProperty("新文本", this.NewText, this.SetNewText)
}
