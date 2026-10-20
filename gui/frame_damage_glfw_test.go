//go:build !windows

package gui

import (
	"bytes"
	"image"
	"math"
	"testing"
	"unsafe"

	"github.com/uk0/silk/geom"
)

func TestFramebufferDamage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		r      geom.Rect
		full   bool
		sx, sy float64
		want   image.Rectangle
	}{
		{"retina", geom.Rect{X: 10, Y: 20, Width: 30, Height: 40}, false, 2, 2, image.Rect(18, 38, 82, 122)},
		{"fractional", geom.Rect{X: 10.2, Y: 20.3, Width: 30.4, Height: 40.2}, false, 1.25, 1.5, image.Rect(11, 28, 52, 93)},
		{"edge", geom.Rect{X: -3, Y: -1, Width: 10, Height: 10}, false, 2, 2, image.Rect(0, 0, 16, 20)},
		{"full", geom.Rect{Width: 1, Height: 1}, true, 2, 2, image.Rect(0, 0, 800, 600)},
		{"large", geom.Rect{Width: 400, Height: 300}, false, 2, 2, image.Rect(0, 0, 800, 600)},
		{"offscreen", geom.Rect{X: 10000, Width: 1, Height: 1}, false, 2, 2, image.Rect(0, 0, 800, 600)},
		{"nan", geom.Rect{X: math.NaN(), Width: 1, Height: 1}, false, 2, 2, image.Rect(0, 0, 800, 600)},
		{"infinite-scale", geom.Rect{Width: 1, Height: 1}, false, math.Inf(1), 2, image.Rect(0, 0, 800, 600)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := framebufferDamage(tc.r, tc.full, 800, 600, tc.sx, tc.sy); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestFrameDamagePreservesDrawTimeInvalidations(t *testing.T) {
	w := &Window{}
	w.MarkDirtyRect(10, 20, 30, 40)
	w.MarkDirtyRect(20, 30, 30, 40)
	full, r := w.takeFrameDamage()
	if full || r != (geom.Rect{X: 10, Y: 20, Width: 40, Height: 50}) || w.dirty {
		t.Fatalf("incorrect snapshot: %v %v", full, r)
	}
	// Represents a callback issuing Update while DrawWidgetAll is executing.
	w.Update()
	if !w.dirty || !w.fullDirty {
		t.Fatal("draw-time update lost")
	}
	full, _ = w.takeFrameDamage()
	if !full || w.dirty {
		t.Fatal("next frame failed to consume update")
	}
	w.MarkDirtyRect(math.NaN(), 0, 1, 1)
	if w.dirty {
		t.Fatal("invalid damage accepted")
	}
}

func TestCopyFramebufferRegion(t *testing.T) {
	for _, stride := range []int{24, 32} {
		src := make([]byte, stride*5)
		for i := range src {
			src[i] = byte(i)
		}
		for _, r := range []image.Rectangle{image.Rect(0, 0, 6, 5), image.Rect(2, 1, 5, 4), image.Rect(0, 3, 6, 5)} {
			dst := make([]byte, r.Dx()*r.Dy()*4)
			copyFramebufferRegion(unsafe.Pointer(&dst[0]), unsafe.Pointer(&src[0]), stride, r)
			var want []byte
			for y := r.Min.Y; y < r.Max.Y; y++ {
				want = append(want, src[y*stride+r.Min.X*4:y*stride+r.Max.X*4]...)
			}
			if !bytes.Equal(dst, want) {
				t.Fatalf("stride=%d rect=%v corrupt copy", stride, r)
			}
		}
	}
}
