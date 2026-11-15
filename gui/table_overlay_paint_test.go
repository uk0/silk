package gui

import (
	"testing"

	"github.com/uk0/silk/paint"
)

func TestTableScrollOverlayIsTransparent(t *testing.T) {
	// Also run with SILK_DEBUG=1: the inherited Widget.Draw placeholder used
	// to paint a translucent fill/red cross over the complete rendered table.
	table := NewTable()
	table.SetSize(320, 180)
	m := NewSimpleTableModel([]string{"Point", "Value"})
	m.AddRow("AI101", "72.5")
	table.SetModel(m)
	pm := paint.NewPixmap(320, 180)
	defer paint.CloseSurface(pm)
	g := pm.NewPainter()
	table.scrollArea.Self().Draw(g)
	paint.ClosePainter(g)
	im, err := pm.Image()
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 180; y++ {
		for x := 0; x < 320; x++ {
			_, _, _, alpha := im.At(x, y).RGBA()
			if alpha != 0 {
				t.Fatalf("scroll overlay covered cell pixel at %d,%d", x, y)
			}
		}
	}
}
