//go:build !windows

package gui

import (
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
	"image"
	"math"
	"testing"
)

func TestSceneCompositionRejectsInexactTransforms(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    geom.Mat3x2
		ok   bool
		want image.Rectangle
	}{
		{"scaled", geom.Mat3x2{Xx: 1.5, Yy: 2, X0: 4, Y0: 8}, true, image.Rect(4, 8, 34, 28)},
		{"fractional", geom.Mat3x2{Xx: 1.5, Yy: 2, X0: 4.25}, false, image.Rectangle{}},
		{"rotate", geom.Mat3x2{Xy: 1, Yx: -1}, false, image.Rectangle{}},
		{"reflect", geom.Mat3x2{Xx: -1, Yy: 1}, false, image.Rectangle{}},
		{"nan", geom.Mat3x2{Xx: math.NaN(), Yy: 1}, false, image.Rectangle{}},
		{"infinite", geom.Mat3x2{Xx: 1, Yy: math.Inf(1)}, false, image.Rectangle{}},
		{"overflow", geom.Mat3x2{Xx: 1, Yy: 1, X0: 1 << 30}, false, image.Rectangle{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, ok := sceneDeviceDestination(tc.m, 20, 10)
			if ok != tc.ok || r != tc.want {
				t.Fatal(r, ok)
			}
		})
	}
}
func TestSceneCompositionConservativePainterGuards(t *testing.T) {
	for _, kind := range []string{"source", "restore", "restore_to", "target", "icon", "icon1"} {
		t.Run(kind, func(t *testing.T) {
			pm := paint.NewPixmap(40, 40)
			defer paint.CloseSurface(pm)
			base := pm.NewPainter()
			defer paint.ClosePainter(base)
			state := sceneCompositionWindow{}
			p := &sceneCompositionPainter{Painter: base, state: &state}
			p.SetOperator(paint.OpSource) // safe before any cutout
			if state.failed != "" {
				t.Fatal("premature rejection")
			}
			p.Save()
			p.SetOperator(paint.OpOver)
			state.layers = []sceneCompositionLayer{{}}
			switch kind {
			case "source":
				p.SetOperator(paint.OpSource)
			case "restore":
				p.Restore()
			case "restore_to":
				p.RestoreTo(0)
			case "target":
				p.Target()
			case "icon":
				p.DrawIcon(paint.LoadIcon("edit-copy"), 12, false)
			case "icon1":
				p.DrawIcon1(paint.LoadIcon("edit-copy"), 2, 2, 12, false)
			}
			if state.failed == "" {
				t.Fatal("unsafe painter operation accepted")
			}
			base.RestoreTo(0)
		})
	}
}
