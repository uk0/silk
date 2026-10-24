package gui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/uk0/silk/paint"
)

func TestDockTabsReserveWindowControls(t *testing.T) {
	f := NewFrame()
	d := f.SuggestDocDock().(*Dock)
	for _, title := range []string{"Terminal", "Output", "Problems", "Tests", "Log", "Debugger", "References", "Git Changes", "Git History"} {
		v := NewLabel(title)
		v.SetDockTitle(title)
		d.AddView(v)
	}
	for _, width := range []float64{1000, 400, 180, 50, 0} {
		d.SetBounds(0, 0, width, 240)
		d.Layout()
		tx, _, tw, _ := d.tabbar.Bounds()
		mx, _, mw, _ := d.minBtn.Bounds()
		xx, _, xw, _ := d.maxBtn.Bounds()
		if tx < 0 || tw < 0 || tx+tw > mx || mx+mw > xx || xx+xw > width {
			t.Fatalf("width=%v tabs=%v..%v min=%v..%v max=%v..%v", width, tx, tx+tw, mx, mx+mw, xx, xx+xw)
		}
	}
}

type diffClipRecorder struct {
	diffContrastRecorder
	clip      diffFillRecord
	stack     []diffFillRecord
	textClips map[string]diffFillRecord
}

func (r *diffClipRecorder) Save() int { r.stack = append(r.stack, r.clip); return len(r.stack) }
func (r *diffClipRecorder) Restore() int {
	r.clip = r.stack[len(r.stack)-1]
	r.stack = r.stack[:len(r.stack)-1]
	return len(r.stack)
}
func (r *diffClipRecorder) Clip() {
	c := r.rect
	if r.clip.w > 0 {
		x, y := max(c.x, r.clip.x), max(c.y, r.clip.y)
		c.w = max(0, min(c.x+c.w, r.clip.x+r.clip.w)-x)
		c.h = max(0, min(c.y+c.h, r.clip.y+r.clip.h)-y)
		c.x, c.y = x, y
	}
	r.clip = c
}
func (r *diffClipRecorder) DrawText1(x, y float64, s string) {
	r.textClips[s] = r.clip
	r.diffContrastRecorder.DrawText1(x, y, s)
}

func TestDiffTextClipsRespectColumnsAndHeaderActions(t *testing.T) {
	d := NewDiffView()
	d.SetSize(600, 240)
	left, right, header := strings.Repeat("left", 100), strings.Repeat("right", 100), strings.Repeat("@@ header ", 100)
	d.diffRows = []DiffRow{{OldLine: left, NewLine: right, Status: DiffModified}, {OldLine: header, Status: DiffHunkHeader}}
	r := &diffClipRecorder{textClips: make(map[string]diffFillRecord)}
	d.Draw(r)
	lc, rc, hc := r.textClips[left], r.textClips[right], r.textClips[header]
	if lc.x < 0 || lc.x+lc.w > 300 || rc.x < 300 || rc.x+rc.w > 600 {
		t.Fatalf("text spills across columns: %+v %+v", lc, rc)
	}
	if hc.x+hc.w > 600-2*diffHunkActionW {
		t.Fatal("hunk header overlays actions")
	}
	if len(r.stack) != 0 {
		t.Fatal("clip stack leaked")
	}
}

func TestDiffGutterCacheReusedAndInvalidated(t *testing.T) {
	d := NewDiffView()
	d.SetSize(600, 200)
	d.diffRows = make([]DiffRow, 100000)
	for i := range d.diffRows {
		d.diffRows[i] = DiffRow{OldLine: "same", NewLine: "same", Status: DiffSame}
	}
	r := diffViewNopPainter{}
	d.Draw(r)
	left, right := &d.leftNumbers[0], &d.rightNumbers[0]
	for i := 0; i < 20; i++ {
		d.Draw(r)
	}
	if left != &d.leftNumbers[0] || right != &d.rightNumbers[0] || d.leftNumbers[99999] != 100000 {
		t.Fatal("repaint rebuilt or corrupted numbering")
	}
	d.SetTexts("a\nb", "a")
	if len(d.leftNumbers) != 2 || d.leftNumbers[1] != 2 || d.rightNumbers[1] != 0 {
		t.Fatal("text replacement retained numbering")
	}
	d.SetPatchFile(DiffPatchFile{OldText: "a\nb", Hunks: []DiffPatchHunk{{Header: "@@ -1,2 +1,2 @@", OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2, Lines: []DiffPatchLine{{Kind: DiffPatchContext, Text: "a"}, {Kind: DiffPatchDeleted, Text: "b"}, {Kind: DiffPatchAdded, Text: "c"}}}}})
	wantL, wantR := gutterLineNumbers(d.diffRows)
	if !equalIntSlice(d.leftNumbers, wantL) || !equalIntSlice(d.rightNumbers, wantR) {
		t.Fatal("patch replacement retained numbering")
	}
}

func BenchmarkDiffVisibleRows(b *testing.B) {
	for _, n := range []int{1000, 100000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			d := NewDiffView()
			d.SetSize(600, 200)
			d.diffRows = make([]DiffRow, n)
			for i := range d.diffRows {
				d.diffRows[i] = DiffRow{OldLine: "same", NewLine: "same", Status: DiffSame}
			}
			r := diffViewNopPainter{}
			d.Draw(r)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				d.Draw(r)
			}
		})
	}
}

type diffInkRecord struct {
	text   string
	fg, bg paint.Color
}
type diffFillRecord struct {
	x, y, w, h float64
	color      paint.Color
}
type diffContrastRecorder struct {
	diffViewNopPainter
	brush paint.Color
	rect  diffFillRecord
	fills []diffFillRecord
	inks  []diffInkRecord
}

func (r *diffContrastRecorder) SetBrush1(c paint.Color) { r.brush = c }
func (r *diffContrastRecorder) Rectangle(x, y, w, h float64) {
	r.rect = diffFillRecord{x, y, w, h, r.brush}
}
func (r *diffContrastRecorder) Fill() { r.rect.color = r.brush; r.fills = append(r.fills, r.rect) }
func diffCompositeTest(fg, bg paint.Color) paint.Color {
	a := uint32(fg.A)
	blend := func(f, b uint8) uint8 { return uint8((uint32(f)*a + uint32(b)*(255-a) + 127) / 255) }
	return paint.Color{R: blend(fg.R, bg.R), G: blend(fg.G, bg.G), B: blend(fg.B, bg.B), A: 255}
}
func (r *diffContrastRecorder) DrawText1(x, y float64, s string) {
	bg := paint.Color{A: 255}
	for _, f := range r.fills {
		if x >= f.x && x < f.x+f.w && y >= f.y && y < f.y+f.h {
			bg = diffCompositeTest(f.color, bg)
		}
	}
	r.inks = append(r.inks, diffInkRecord{s, diffCompositeTest(r.brush, bg), bg})
}
func TestDiffDrawTextAndGuttersReadableInBothThemes(t *testing.T) {
	original := CurrentThemeMode()
	defer SetThemeMode(original)
	for _, mode := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(mode)
		d := NewDiffView()
		d.SetBounds(0, 0, 600, 250)
		d.diffRows = []DiffRow{{OldLine: "same", NewLine: "same", Status: DiffSame}, {NewLine: "added", Status: DiffAdded}, {OldLine: "removed", Status: DiffRemoved}, {OldLine: "before", NewLine: "after", Status: DiffModified}, {OldLine: "@@ header @@", Status: DiffHunkHeader}}
		r := &diffContrastRecorder{}
		d.Draw(r)
		if len(r.inks) < 10 {
			t.Fatal("draw did not exercise rows and gutters")
		}
		for _, ink := range r.inks {
			if ratio := contrastRatio(ink.fg, ink.bg); ratio < 4.5 {
				t.Errorf("mode=%v text=%q contrast=%.2f fg=%v bg=%v", mode, ink.text, ratio, ink.fg, ink.bg)
			}
		}
	}
}

// A checked menu action must be visible even without pointer hover. Otherwise
// the all-tabs chooser records the active item but paints it like every other
// entry (also affecting checkable SDK popup actions).
func TestPopupCheckedActionHasVisibleReadableState(t *testing.T) {
	old := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(old) })
	for _, mode := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(mode)
		menu := NewPopupMenu()
		b := menu.AddButton1("Current tab", nil)
		b.SetSize(160, 28)
		unchecked := newThemeFillRecorder()
		unchecked.fill = Theme().MenuBGColor
		Theme().DrawButton(unchecked, b)
		b.Action().SetChecked(true)
		checked := newThemeFillRecorder()
		checked.fill = Theme().MenuBGColor
		Theme().DrawButton(checked, b)
		if checked.fill == unchecked.fill {
			t.Error("checked popup action has no visible state")
		}
		if checked.texts == 0 || contrastRatio(checked.ink, checked.fill) < 4.5 {
			t.Errorf("checked popup action label is unreadable: ink=%v fill=%v texts=%v ratio=%.2f", checked.ink, checked.fill, checked.texts, contrastRatio(checked.ink, checked.fill))
		}
	}
}
