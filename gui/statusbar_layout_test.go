package gui

import (
	"strings"
	"testing"
)

func TestStatusBarContainsOverflowAndEmptyCells(t *testing.T) {
	sb := NewStatusBar()
	left := sb.AddIconLabel("", strings.Repeat("长项目名称", 30))
	empty := NewLabel("")
	sb.AddPermanentWidget(empty)
	right := sb.AddIconLabel("", "v1.0")
	sb.SetMessage("message must not paint underneath permanent cells")
	for _, size := range [][2]float64{{1100, 30}, {120, 24}, {20, 4}, {1, 1}, {0, 0}, {700, 30}} {
		sb.SetSize(size[0], size[1])
		sb.Layout()
		lastRight := 0.0
		for _, cell := range sb.PermanentWidgets() {
			x, y, w, h := cell.Bounds()
			if x < 0 || y < 0 || w < 0 || h < 0 || x+w > size[0]+0.01 || y+h > size[1]+0.01 {
				t.Fatalf("%v: cell outside bar: %v %v %v %v", size, x, y, w, h)
			}
			if w > 0 {
				if x < lastRight {
					t.Fatalf("%v: status cells overlap", size)
				}
				lastRight = x + w
			}
			if !cell.IsVisible() {
				t.Fatal("layout changed explicit visibility")
			}
		}
		if ew, _ := empty.Size(); ew != 0 {
			t.Fatal("empty label reserves width")
		}
		lx, _, lw, _ := left.Bounds()
		rx, _, rw, _ := right.Bounds()
		if lw > 0 && rw > 0 && rx-lx-lw < 7.99 {
			t.Fatal("missing cell gap or empty cell retained spacing")
		}
		rec := newElideRecorder()
		sb.Draw(rec)
		if sb.hasPermanent && sb.permanentLeft <= Theme().ButtonMargin.L && rec.texts[0] != "" {
			t.Fatal("overfull bar painted message under permanent cells")
		}
	}
}

func TestStatusIconLabelElidesToAllocatedWidth(t *testing.T) {
	cell := NewStatusIconLabel("", strings.Repeat("Unicode 项目😀", 20))
	cell.SetSize(95, 24)
	rec := newElideRecorder()
	cell.Draw(rec)
	if len(rec.texts) != 1 || !strings.HasSuffix(rec.texts[0], elideMark) || Theme().Font.TextExtents(rec.texts[0]).Width > 95 {
		t.Fatalf("cell not elided to width: %q", rec.texts)
	}
	cell.SetSize(0, 0)
	rec = newElideRecorder()
	cell.Draw(rec)
	if len(rec.texts) != 0 {
		t.Fatal("zero-size cell still paints")
	}
}
