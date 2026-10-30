//go:build cgo && !silk_pure_go

package paint

import (
	"github.com/uk0/silk/geom"
	"image"
	"math"
	"testing"
)

func TestDeviceClipExactAndCutoutPreservesState(t *testing.T) {
	pm := NewPixmap(80, 60)
	defer CloseSurface(pm)
	p := pm.NewPainter()
	defer ClosePainter(p)
	p.SetBrush1(Color{30, 60, 90, 255})
	p.Paint()
	p.Save()
	p.Translate(4, 6)
	p.Scale(2, 1.5)
	p.Rectangle(2, 4, 20, 12)
	p.Clip()
	p.MoveTo(3, 5)
	p.LineTo(12, 14)
	p.SetOperator(OpSource)
	var before geom.Mat3x2
	p.GetMatrix(&before)
	state := p.CurrentState()
	x, y := p.CurrentPoint()
	if r, ok := DeviceClipRectangle(p); !ok || r != image.Rect(8, 12, 48, 30) {
		t.Fatalf("exact clip=%v %v", r, ok)
	}
	if !ClearDeviceClip(p) {
		t.Fatal("clear failed")
	}
	var after geom.Mat3x2
	p.GetMatrix(&after)
	xx, yy := p.CurrentPoint()
	op, ok := PainterOperator(p)
	if before != after || state != p.CurrentState() || x != xx || y != yy || !ok || op != OpSource {
		t.Fatal("query/cutout changed painter state")
	}
	im, err := pm.Image()
	if err != nil {
		t.Fatal(err)
	}
	for yy := 0; yy < 60; yy++ {
		for xx := 0; xx < 80; xx++ {
			_, _, _, a := im.At(xx, yy).RGBA()
			want := uint32(65535)
			if image.Pt(xx, yy).In(image.Rect(8, 12, 48, 30)) {
				want = 0
			}
			if a != want {
				t.Fatalf("pixel %d,%d alpha %d want %d", xx, yy, a, want)
			}
		}
	}
	// Stroke the retained path: checking the point alone would miss path loss.
	p.SetOperator(OpOver)
	p.SetPen1(Color{255, 0, 0, 255}, 2)
	p.Stroke()
	im, _ = pm.Image()
	_, _, _, a := im.At(18, 21).RGBA()
	if a == 0 {
		t.Fatal("cutout discarded current path")
	}
	p.Restore()
}
func TestDeviceClipRejectsComplexFractionalAndEmptyMasks(t *testing.T) {
	for _, kind := range []string{"circle", "fractional", "disjoint", "empty"} {
		t.Run(kind, func(t *testing.T) {
			pm := NewPixmap(80, 60)
			defer CloseSurface(pm)
			p := pm.NewPainter()
			defer ClosePainter(p)
			switch kind {
			case "circle":
				p.Arc(30, 30, 20, 0, 2*math.Pi)
			case "fractional":
				p.Rectangle(1.25, 2.5, 20, 20)
			case "disjoint":
				p.Rectangle(0, 0, 10, 10)
				p.Rectangle(40, 40, 10, 10)
			case "empty":
				p.Rectangle(100, 100, 10, 10)
			}
			p.Clip()
			if r, ok := DeviceClipRectangle(p); ok {
				t.Fatal("approximated mask", r)
			}
			if ClearDeviceClip(p) {
				t.Fatal("cleared unsupported mask")
			}
		})
	}
	if _, ok := DeviceClipRectangle(nil); ok {
		t.Fatal("nil supported")
	}
	if _, ok := PainterOperator(nil); ok {
		t.Fatal("nil operator")
	}
}
