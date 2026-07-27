package ged

import (
	"strconv"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.MergeEditor", gui.TypeOf(MergeEditor{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.MergeEditor",
		Name: "合并 / Merge",
		Icon: "merge",
		Desc: "三方合并冲突编辑器（ours / theirs / 两者 / 手工）",
	})
}

// MergeEditor is the IDE's conflict editor — Qt Creator's "merge tool" /
// VS Code's inline conflict view, driven entirely by core's three-way
// merge engine. It shows one row per chunk header plus the chunk's lines,
// and gives every conflict four answers: take ours, take theirs, keep
// both, or hand-written lines (SetManual, for a host that pops a text
// editor). A status band counts what is still unresolved and carries the
// Save chip.
//
// Like the sibling panes it owns no IO: the host feeds it either three
// sides (SetMerge → core.Merge3) or an already-conflicted file
// (SetConflictText → core.ParseConflictMarkers), and gets the merged text
// back through SigSave. Saving is gated on CanSave — SigSave never fires
// while a conflict is unanswered, so a half-merged file cannot reach disk
// through this widget.
//
// All the state lives in MergeModel (merge-editor-model.go); the widget
// only caches the model's flattened rows so Draw and the hit-test agree on
// the geometry.
type MergeEditor struct {
	gui.Widget

	model     *MergeModel
	rows      []MergeRow
	scrollY   float64
	hoverIdx  int
	rowHeight float64

	cbSave     func(text string)
	cbResolved func(index int, choice MergeChoice)
}

// NewMergeEditor creates an empty conflict editor.
func NewMergeEditor() *MergeEditor {
	e := new(MergeEditor)
	e.Init(e)
	return e
}

func (this *MergeEditor) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
	this.model = NewMergeModel(nil)
}

// Model exposes the data layer so a host can drive resolutions in bulk or
// inspect the merge without going through the widget.
func (this *MergeEditor) Model() *MergeModel {
	return this.model
}

// SetChunks installs a chunk list (from core.Merge3 or
// core.ParseConflictMarkers), dropping every previous resolution.
func (this *MergeEditor) SetChunks(chunks []core.MergeChunk) {
	this.model.SetChunks(chunks)
	this.scrollY = 0
	this.hoverIdx = -1
	this.refresh()
}

// SetMerge runs the three-way merge over base/ours/theirs and shows the
// result.
func (this *MergeEditor) SetMerge(base, ours, theirs []string) {
	this.SetChunks(core.Merge3(base, ours, theirs))
}

// SetConflictText loads a file that already carries git conflict markers.
// Whatever parsed is installed even when the markers were malformed; the
// parse error is returned so the host can report it.
func (this *MergeEditor) SetConflictText(text string) error {
	if text == "" {
		this.SetChunks(nil)
		return nil
	}
	chunks, err := core.ParseConflictMarkers(splitLines(text))
	this.SetChunks(chunks)
	return err
}

// Rows returns a copy of the flattened display rows — the same list Draw
// and the hit-test use.
func (this *MergeEditor) Rows() []MergeRow {
	out := make([]MergeRow, len(this.rows))
	copy(out, this.rows)
	return out
}

// Resolve answers conflict chunk index with ours / theirs / both (or
// clears it with MergeChoiceNone) and fires SigResolved on success. See
// MergeModel.Resolve for what gets rejected.
func (this *MergeEditor) Resolve(index int, choice MergeChoice) bool {
	if !this.model.Resolve(index, choice) {
		return false
	}
	this.refresh()
	if this.cbResolved != nil {
		this.cbResolved(index, choice)
	}
	return true
}

// SetManual answers conflict chunk index with hand-written lines and fires
// SigResolved on success.
func (this *MergeEditor) SetManual(index int, lines []string) bool {
	if !this.model.SetManual(index, lines) {
		return false
	}
	this.refresh()
	if this.cbResolved != nil {
		this.cbResolved(index, MergeChoiceManual)
	}
	return true
}

// ConflictCount is how many chunks need an answer.
func (this *MergeEditor) ConflictCount() int {
	return this.model.ConflictCount()
}

// UnresolvedCount is how many conflicts are still unanswered.
func (this *MergeEditor) UnresolvedCount() int {
	return this.model.UnresolvedCount()
}

// CanSave reports whether every conflict has an answer — the gate SigSave
// obeys.
func (this *MergeEditor) CanSave() bool {
	return this.model.CanSave()
}

// Result is the merged text as it stands (unresolved conflicts keep their
// markers).
func (this *MergeEditor) Result() string {
	return this.model.Result()
}

// SigSave registers the callback that receives the merged text. It fires
// only from Save(), and only once CanSave() is true.
func (this *MergeEditor) SigSave(fn func(text string)) {
	this.cbSave = fn
}

// SigResolved registers the callback fired whenever a conflict's answer
// changes — the host uses it to mark the document dirty.
func (this *MergeEditor) SigResolved(fn func(index int, choice MergeChoice)) {
	this.cbResolved = fn
}

// Save hands Result() to the SigSave callback and reports whether it
// fired. It refuses — returning false without calling anything — while a
// conflict is unanswered, and likewise when no callback is registered.
func (this *MergeEditor) Save() bool {
	if !this.CanSave() || this.cbSave == nil {
		return false
	}
	this.cbSave(this.Result())
	return true
}

// refresh re-flattens the model into rows and repaints. Every mutation
// goes through here so the cached rows can never lag the model.
func (this *MergeEditor) refresh() {
	this.rows = this.model.Rows()
	this.Self().Update()
}

// --- Pure helpers (GL-free, unit-testable) ---

// Row geometry. The status band sits on top; rows start below it.
const (
	mergeEditorHeaderH = 22.0 // status band height
	mergeHeaderPad     = 6.0  // right-edge inset for the band and row chips
	mergeSaveBtnW      = 74.0 // "保存 / Save" chip width
	mergeChoiceBtnW    = 46.0 // per-answer chip width on a conflict header row
	mergeChoiceBtnGap  = 4.0
	mergeRowTextX      = 8.0  // header text inset
	mergeRowIndentX    = 20.0 // content-line inset (indented under its header)
)

// mergeChoiceOrder is the left-to-right order of the answer chips; the
// manual answer has no chip (it needs text, so the host supplies it via
// SetManual).
var mergeChoiceOrder = [3]MergeChoice{MergeChoiceOurs, MergeChoiceTheirs, MergeChoiceBoth}

// mergeChoiceLabels are the chip captions, parallel to mergeChoiceOrder.
var mergeChoiceLabels = [3]string{"我们的", "他们的", "两者"}

// mergeRowAtY maps a y coordinate to a row index for a list whose rows
// start at topOffset with count rows of height rowH. The caller folds the
// scroll offset into y. Returns -1 above the rows, past the last row, or
// for a degenerate rowH. (Named mergeRowAtY because the package already
// owns rowAtY / refRowAtY / frameRowAtY.)
func mergeRowAtY(y, topOffset, rowH float64, count int) int {
	if rowH <= 0 || y < topOffset {
		return -1
	}
	idx := int((y - topOffset) / rowH)
	if idx < 0 || idx >= count {
		return -1
	}
	return idx
}

// mergeChoiceRect returns the [x0, x1) span of the i-th answer chip on a
// conflict header row of width w. The chips are right-aligned, ending
// mergeHeaderPad from the right edge, so the hit-test is a pure function
// of the widget width — no font measurement off the paint path.
func mergeChoiceRect(w float64, i int) (x0, x1 float64) {
	x1 = w - mergeHeaderPad - float64(len(mergeChoiceOrder)-1-i)*(mergeChoiceBtnW+mergeChoiceBtnGap)
	x0 = x1 - mergeChoiceBtnW
	return
}

// mergeChoiceAtX maps an x coordinate on a conflict header row to the
// answer its chip carries, or MergeChoiceNone when x hits no chip.
func mergeChoiceAtX(x, w float64) MergeChoice {
	for i := range mergeChoiceOrder {
		if x0, x1 := mergeChoiceRect(w, i); x >= x0 && x < x1 {
			return mergeChoiceOrder[i]
		}
	}
	return MergeChoiceNone
}

// mergeSaveHit reports whether (x, y) lands on the Save chip in the status
// band.
func mergeSaveHit(x, y, w float64) bool {
	if y < 0 || y >= mergeEditorHeaderH {
		return false
	}
	return x >= w-mergeHeaderPad-mergeSaveBtnW && x < w-mergeHeaderPad
}

// mergeStatusLabel renders the status band caption: the conflict tally and
// how many of them are still unanswered.
func mergeStatusLabel(conflicts, unresolved int) string {
	if conflicts == 0 {
		return "合并 / Merge (无冲突)"
	}
	if unresolved == 0 {
		return "合并 / Merge (冲突 " + strconv.Itoa(conflicts) + ", 已全部解决)"
	}
	return "合并 / Merge (冲突 " + strconv.Itoa(conflicts) +
		", 未解决 " + strconv.Itoa(unresolved) + ")"
}

// mergeKindColor is the accent colour of a chunk kind, used for the header
// row's left stripe and caption.
func mergeKindColor(k core.MergeKind) paint.Color {
	switch k {
	case core.MergeOurs:
		return paint.Color{R: 90, G: 150, B: 220, A: 255}
	case core.MergeTheirs:
		return paint.Color{R: 90, G: 190, B: 160, A: 255}
	case core.MergeConflict:
		return paint.Color{R: 220, G: 110, B: 90, A: 255}
	}
	return paint.Color{R: 130, G: 140, B: 155, A: 255}
}

// mergeSideColor is the text colour of a content row: dim grey for lines
// already in the merged text, and the ours / theirs accent for the two
// candidate sides of an unresolved conflict.
func mergeSideColor(side MergeSide) paint.Color {
	switch side {
	case MergeSideOurs:
		return paint.Color{R: 150, G: 190, B: 240, A: 255}
	case MergeSideTheirs:
		return paint.Color{R: 150, G: 225, B: 200, A: 255}
	}
	return paint.Color{R: 200, G: 200, B: 210, A: 255}
}

// --- Drawing ---

// Draw renders the status band (tally + Save chip) followed by the rows:
// each chunk header carries a kind stripe, its caption and — for conflicts
// — the three answer chips with the active one lit; content lines are
// indented under their header and tinted by side.
func (this *MergeEditor) Draw(g paint.Painter) {
	w, h := this.Size()

	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)
	fe := font.FontExtents()

	// Status band.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, 0, w, mergeEditorHeaderH)
	g.Fill()
	unresolved := this.model.UnresolvedCount()
	if unresolved > 0 {
		g.SetBrush1(paint.Color{R: 230, G: 150, B: 130, A: 255})
	} else {
		g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	}
	g.DrawText1(mergeRowTextX, fe.Ascent+4, mergeStatusLabel(this.model.ConflictCount(), unresolved))
	this.drawSaveChip(g, font, w)

	if len(this.rows) == 0 {
		return
	}

	rh := this.rowHeight
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-mergeEditorHeaderH)/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(this.rows); i++ {
		y := mergeEditorHeaderH + float64(i)*rh - this.scrollY
		row := this.rows[i]

		if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if row.Header {
			g.SetBrush1(paint.Color{R: 38, G: 38, B: 46, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		if !row.Header {
			g.SetBrush1(mergeSideColor(row.Side))
			g.DrawText1(mergeRowIndentX, y+fe.Ascent+2, row.Text)
			continue
		}

		// Kind stripe + caption.
		accent := mergeKindColor(row.Kind)
		g.SetBrush1(accent)
		g.Rectangle(0, y, 3, rh)
		g.Fill()
		g.DrawText1(mergeRowTextX, y+fe.Ascent+2, row.Text)

		if row.Kind == core.MergeConflict {
			this.drawChoiceChips(g, font, w, y, row.Choice)
		}
	}
}

// drawSaveChip paints the Save affordance in the status band: green while
// the merge is complete, muted grey while a conflict is unanswered (the
// state in which Save() refuses to fire).
func (this *MergeEditor) drawSaveChip(g paint.Painter, font paint.Font, w float64) {
	fe := font.FontExtents()
	const m = 3.0
	enabled := this.CanSave()
	if enabled {
		g.SetBrush1(paint.Color{R: 45, G: 110, B: 70, A: 255})
	} else {
		g.SetBrush1(paint.Color{R: 40, G: 44, B: 52, A: 255})
	}
	x0 := w - mergeHeaderPad - mergeSaveBtnW
	g.Rectangle(x0, m, mergeSaveBtnW, mergeEditorHeaderH-2*m)
	g.Fill()
	if enabled {
		g.SetBrush1(paint.Color{R: 220, G: 235, B: 225, A: 255})
	} else {
		g.SetBrush1(paint.Color{R: 120, G: 128, B: 138, A: 255})
	}
	label := "保存 / Save"
	lw := font.TextExtents(label).Width
	g.DrawText1(x0+(mergeSaveBtnW-lw)/2, fe.Ascent+4, label)
}

// drawChoiceChips paints the three answer chips on a conflict header row
// at y, lighting the one that is currently chosen.
func (this *MergeEditor) drawChoiceChips(g paint.Painter, font paint.Font, w, y float64, choice MergeChoice) {
	fe := font.FontExtents()
	const m = 3.0
	for i, ch := range mergeChoiceOrder {
		x0, x1 := mergeChoiceRect(w, i)
		if ch == choice {
			g.SetBrush1(paint.Color{R: 70, G: 100, B: 150, A: 255})
		} else {
			g.SetBrush1(paint.Color{R: 48, G: 52, B: 62, A: 255})
		}
		g.Rectangle(x0, y+m, x1-x0, this.rowHeight-2*m)
		g.Fill()
		if ch == choice {
			g.SetBrush1(paint.Color{R: 225, G: 232, B: 240, A: 255})
		} else {
			g.SetBrush1(paint.Color{R: 165, G: 172, B: 185, A: 255})
		}
		label := mergeChoiceLabels[i]
		lw := font.TextExtents(label).Width
		g.DrawText1(x0+(x1-x0-lw)/2, y+fe.Ascent+2, label)
	}
}

// --- Events ---

// OnLeftDown resolves a conflict when an answer chip on its header row is
// clicked, or saves when the Save chip in the status band is. Clicks
// elsewhere only take focus.
func (this *MergeEditor) OnLeftDown(x, y float64) {
	this.SetFocus()
	w, _ := this.Size()
	if y < mergeEditorHeaderH {
		if mergeSaveHit(x, y, w) {
			this.Save()
		}
		return
	}
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.rows) {
		return
	}
	row := this.rows[idx]
	if !row.Header || row.Kind != core.MergeConflict {
		return
	}
	if choice := mergeChoiceAtX(x, w); choice != MergeChoiceNone {
		this.Resolve(row.Chunk, choice)
	}
}

// OnMouseMove tracks the hover highlight.
func (this *MergeEditor) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(this.rows) {
		idx = -1
	}
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover highlight.
func (this *MergeEditor) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the row list, clamped to the content.
func (this *MergeEditor) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := float64(len(this.rows))*this.rowHeight - (h - mergeEditorHeaderH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// rowAt maps a y coordinate to a row index, folding in the scroll offset.
func (this *MergeEditor) rowAt(y float64) int {
	return mergeRowAtY(y+this.scrollY, mergeEditorHeaderH, this.rowHeight, len(this.rows))
}

func (this *MergeEditor) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 260, MinHeight: 100}
}
