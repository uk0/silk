package gui

import (
	"testing"

	"github.com/uk0/silk/geom"
)

func TestGridCellTracksAgreeWithLayoutAndAreDetached(t *testing.T) {
	g := NewGridLayout()
	g.SetSize(321, 246)
	g.SetPadding(Padding{L: 11, T: 13, R: 17, B: 19})
	g.SetSpacing(7)
	g.SetColumnWidth(0, 53)
	g.SetRowHeight(0, 39)
	a, b := NewButton(), NewButton()
	a.SetText("Span")
	b.SetText("Corner")
	g.AddLayoutWidget(a, 0, 0, 1, 2)
	g.AddLayoutWidget(b, 2, 2, 1, 1)
	g.Layout()
	s := g.CellTracks()
	if s.Content != (geom.Rect{X: 11, Y: 13, Width: 293, Height: 214}) || len(s.Rows) != 3 || len(s.Columns) != 3 {
		t.Fatalf("bad cell tracks: %+v", s)
	}
	x, y, w, h := a.Bounds()
	if x != s.Columns[0].Start || y != s.Rows[0].Start ||
		w != s.Columns[0].Size+s.Columns[1].Size+7 || h != s.Rows[0].Size {
		t.Fatalf("span does not equal solved tracks: %v %v %v %v / %+v", x, y, w, h, s)
	}
	s.Columns[0].Size = -99
	s.Rows[0].Start = -99
	second := g.CellTracks()
	if second.Columns[0].Size != 53 || second.Rows[0].Start != 13 {
		t.Fatal("snapshot alias escaped the pooled solver")
	}
}

func TestFormCellTracksKeepSparseIndicesAndEmptyFieldGeometry(t *testing.T) {
	f := NewFormLayout()
	f.SetSize(280, 144)
	f.SetPadding(Padding{L: 10, T: 9, R: 12, B: 5})
	f.SetSpacing(6)
	f.SetLabelWidth(80)
	f.SetRowHeight(24)
	first, sparse := NewLabel("label"), NewButton()
	sparse.SetText("field")
	f.AddLayoutWidget(first, 0, 0, 1, 1)
	f.AddLayoutWidget(sparse, 4, 1, 1, 1)
	f.Layout()
	s := f.CellTracks()
	if len(s.Rows) != 2 || s.Rows[0] != (LayoutTrack{0, 9, 24}) || s.Rows[1] != (LayoutTrack{4, 39, 24}) ||
		len(s.Columns) != 2 || s.Columns[1] != (LayoutTrack{1, 96, 172}) {
		t.Fatalf("sparse/empty form track differs: %+v", s)
	}
	x, y, w, h := sparse.Bounds()
	if x != s.Columns[1].Start || y != s.Rows[1].Start || w != s.Columns[1].Size || h != s.Rows[1].Size {
		t.Fatal("exposed form track differs from bounds")
	}
}
