package gui

import (
	"math"
	"strings"
)

// SetCompletionStatus supplies short, non-animated provider feedback on the UI
// thread. It does not open a popup dismissed by Escape or a pointer action.
// Hosts must reject stale responses before calling, just as for completions.
func (ed *CodeEditor) SetCompletionStatus(status string) {
	if ed.completion == nil {
		ed.completion = NewCompletionPopup(ed)
	}
	status = strings.Join(strings.Fields(status), " ")
	if r := []rune(status); len(r) > 256 {
		status = string(r[:256])
	}
	if ed.completion.status == status {
		return
	}
	ed.completion.status = status
	if ed.completion.visible {
		ed.Self().Update()
	}
}

func (p *CompletionPopup) footerText() string {
	if p.status != "" {
		return p.status
	}
	if p.localPending {
		return "Indexing local symbols..."
	}
	source := "Local"
	if p.selectedIdx >= 0 && p.selectedIdx < len(p.filtered) && p.filtered[p.selectedIdx].Source != "" {
		source = strings.Join(strings.Fields(p.filtered[p.selectedIdx].Source), " ")
	}
	return source + " | Enter/Tab or click to accept"
}

// One geometry for painting and all pointer input. Footer/borders are part of
// the overlay, not click-through areas. No independent native window is used.
type completionLayout struct {
	x, y, w, h, rowH float64
	rows             int
}

func (l completionLayout) hit(x, y float64, scroll int) (row int, inside bool) {
	if l.rows == 0 || x < l.x || x >= l.x+l.w || y < l.y || y >= l.y+l.h {
		return -1, false
	}
	r := int((y - l.y) / l.rowH)
	if r >= l.rows {
		return -1, true
	}
	return scroll + r, true
}

func (p *CompletionPopup) layout(ed *CodeEditor) completionLayout {
	if !p.visible || len(p.filtered) == 0 {
		return completionLayout{}
	}
	fe := ed.font.FontExtents()
	w, h := ed.Size()
	x, y := ed.caretLocalXY() // same folded-row coordinate system as the caret
	left, right := ed.gutterW+5, w-ed.minimapWidth()-5
	top, bottom := ed.topOffset(), h-ed.statusBarHeight
	l := fitCompletionLayout(x, y, fe.Height+2, fe.Height+6, math.Max(300, fe.Height*20), left, top, right, bottom, min(p.maxVisible, len(p.filtered)))
	p.pageRows = l.rows
	if l.rows > 0 {
		p.selectIndex(p.selectedIdx)
	} else {
		p.Dismiss() // invisible choices must not consume Enter/Tab
	}
	return l
}

// Fit whole rows plus a footer on the larger side of the caret if necessary.
// Tiny viewports suppress the overlay rather than paint over editor chrome.
func fitCompletionLayout(x, y, lineH, rowH, wantedW, left, top, right, bottom float64, wantedRows int) completionLayout {
	width := math.Min(math.Min(wantedW, 480), right-left)
	if width < 40 || rowH <= 0 || wantedRows <= 0 || bottom <= top {
		return completionLayout{}
	}
	y = math.Max(top, math.Min(bottom, y))
	below, above := bottom-y, math.Max(0, y-lineH-top)
	aboveCaret := below < float64(wantedRows+1)*rowH && above > below
	space := below
	if aboveCaret {
		space = above
	}
	rows := min(wantedRows, int(space/rowH)-1)
	if rows < 1 {
		return completionLayout{}
	}
	height := float64(rows+1) * rowH
	if aboveCaret {
		y -= lineH + height
	}
	return completionLayout{x: math.Max(left, math.Min(x, right-width)), y: math.Max(top, y), w: width, h: height, rowH: rowH, rows: rows}
}

func (p *CompletionPopup) pageSize() int {
	if p.pageRows > 0 {
		return p.pageRows
	}
	return max(1, p.maxVisible)
}

func (p *CompletionPopup) selectIndex(index int) {
	if len(p.filtered) == 0 {
		return
	}
	p.selectedIdx = max(0, min(index, len(p.filtered)-1))
	rows := p.pageSize()
	p.scrollY = max(0, min(p.scrollY, len(p.filtered)-rows))
	if p.selectedIdx < p.scrollY {
		p.scrollY = p.selectedIdx
	}
	if p.selectedIdx >= p.scrollY+rows {
		p.scrollY = p.selectedIdx - rows + 1
	}
}

func (p *CompletionPopup) scroll(z float64) bool {
	if z == 0 || math.IsNaN(z) || math.IsInf(z, 0) {
		return false
	}
	// Clamp before converting to int, including malicious/unusual device deltas.
	delta := int(math.Min(float64(len(p.filtered)), math.Ceil(math.Abs(z))*3))
	if z > 0 {
		delta = -delta
	}
	rows := p.pageSize()
	next := max(0, min(p.scrollY+delta, len(p.filtered)-rows))
	if next == p.scrollY {
		return false
	}
	p.scrollY = next
	p.selectedIdx = max(next, min(p.selectedIdx, min(len(p.filtered)-1, next+rows-1)))
	return true
}
