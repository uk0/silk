package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"testing"
)

func TestProcessEquipmentClippedVectorQuality(t *testing.T) {
	w := NewProcessEquipment()
	for _, kind := range []string{"reactor", "exchanger", "separator"} {
		w.SetKind(kind)
		for _, size := range [][2]float64{{1, 1}, {16, 12}, {100, 180}} {
			w.SetSize(size[0], size[1])
			pm := paint.NewPixmap(120, 200)
			g := pm.NewPainter()
			w.Draw(g)
			paint.ClosePainter(g)
			im, e := pm.Image()
			if e != nil {
				t.Fatal(e)
			}
			for y := 0; y < 200; y++ {
				for x := 0; x < 120; x++ {
					if float64(x) >= size[0] || float64(y) >= size[1] {
						_, _, _, a := im.At(x, y).RGBA()
						if a != 0 {
							t.Fatal("equipment escaped clip", kind, x, y)
						}
					}
				}
			}
			paint.CloseSurface(pm)
		}
	}
	w.SetSample(core.Value{Raw: true, Quality: core.QualityGood})
	if !w.IsRunning() {
		t.Fatal("DI not bound")
	}
	w.SetSample(core.Value{Raw: true, Quality: core.QualityBad})
	if w.IsRunning() {
		t.Fatal("bad data looks running")
	}
	w.SetSample(core.Value{Raw: "invalid", Quality: core.QualityGood})
	if w.IsRunning() {
		t.Fatal("invalid boolean looks running")
	}
}
