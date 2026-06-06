package gui

import (
	"strings"

	"silk/core"
	"silk/paint"
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
}

// NewDiffView creates an empty diff viewer. Callers populate it with
// SetTexts (or SetOldText/SetNewText) once the two sides are known.
func NewDiffView() *DiffView {
	p := new(DiffView)
	p.Init(p)
	return p
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

	// Per-row tints. Light red/green washes — alpha kept moderate so the
	// foreground text stays readable against the form background.
	colRemoved := paint.Color{R: 255, G: 220, B: 220, A: 180}
	colAdded := paint.Color{R: 220, G: 245, B: 220, A: 180}
	colStripe := paint.Color{R: 0, G: 0, B: 0, A: 8} // subtle alternate stripe

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

		// Foreground colour matches the theme text colour; tints supply
		// the per-row status colour.
		g.SetBrush1(t.TextColor)
		if dr.OldLine != "" {
			g.DrawText1(leftX+diffLinePad, ty, dr.OldLine)
		}
		if dr.NewLine != "" {
			g.DrawText1(rightX+diffLinePad, ty, dr.NewLine)
		}
	}

	this.drawDivider(g, w, h, t)
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

// OnLeftDown grabs focus so subsequent wheel events route here. The diff
// view is read-only beyond that — there's no caret to place.
func (this *DiffView) OnLeftDown(x, y float64) {
	this.SetFocus()
}

// EnumProperties exposes the two texts to the property sheet so the
// designer can preview the widget with sample content.
func (this *DiffView) EnumProperties(list core.IPropertyList) {
	list.AddProperty("旧文本", this.OldText, this.SetOldText)
	list.AddProperty("新文本", this.NewText, this.SetNewText)
}
