package gui

import (
	"github.com/uk0/silk/paint"
	"image"
	"testing"
)

func TestTableLaterHeaderBackgroundDoesNotEraseFirstCaption(t *testing.T) {
	render := func(title string) image.Image {
		table := NewTable()
		m := NewSimpleTableModel([]string{title, "Type", "Unit"})
		table.SetModel(m)
		table.SetSize(420, 100)
		pm := paint.NewPixmap(420, 100)
		g := pm.NewPainter()
		table.Draw(g)
		paint.ClosePainter(g)
		im, err := pm.Image()
		paint.CloseSurface(pm)
		if err != nil {
			t.Fatal(err)
		}
		return im
	}
	a, b := render("POINT_NAME"), render("")
	different := 0
	for y := 3; y < 20; y++ {
		for x := 4; x < 100; x++ {
			if a.At(x, y) != b.At(x, y) {
				different++
			}
		}
	}
	if different == 0 {
		t.Fatal("later column backgrounds erased first header")
	}
}

func TestTablePointerRoutingReachesCellsEditorAndScrollBars(t *testing.T) {
	table, m := geometryTable()
	table.SetCellsEditable(true)
	root := NewForm()
	root.SetSize(500, 300)
	table.SetParent(root)
	table.SetBounds(10, 20, 400, 200)
	for i := 0; i < 2; i++ {
		hit := root.FindWidgetAt(160, 70)
		if hit != table {
			t.Fatalf("cell intercepted by %T", hit)
		}
		hit.(interface{ OnLeftDown(float64, float64) }).OnLeftDown(150, 50)
	}
	if !table.editing || table.editRow != 1 || table.editCol != 1 {
		t.Fatal("pointer double-click did not edit cell")
	}
	if hit := table.FindWidgetAt(150, 50); hit != table.editor {
		t.Fatalf("inline editor lost hit: %T", hit)
	}
	table.editor.SetText("Edited")
	table.commitEdit()
	if m.CellText(1, 1) != "Edited" {
		t.Fatal("pointer edit not committed")
	}
	table.SetSize(200, 80)
	bar := table.scrollArea.HorzScrollBar()
	x, y, w, h := bar.Bounds()
	if !bar.IsVisible() || table.FindWidgetAt(x+w/2, y+h/2) != bar {
		t.Fatal("horizontal scroll bar lost input")
	}
	if table.FindWidgetAt(-1, 10) != nil {
		t.Fatal("out of bounds hit")
	}
}
