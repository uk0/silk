//go:build !cgo || silk_pure_go

package glrender

import (
	"errors"
	"github.com/uk0/silk/scene3d"
	"image"
	"image/color"
	"testing"
)

func TestResidentUnsupportedBackendIsExplicit(t *testing.T) {
	for _, r := range []*Renderer{nil, {}} {
		f, a := r.RenderGPU(nil, scene3d.Camera{}, 1, 1, color.RGBA{})
		capture, b := r.Readback(f)
		id, c := r.PickGPU(f, 0, 0)
		tr, d := r.Transfers()
		e := r.BlitColor(f, 0, 1, 1, image.Rect(0, 0, 1, 1), image.Rect(0, 0, 1, 1))
		for _, err := range []error{a, b, c, d, e} {
			if !errors.Is(err, ErrUnavailable) {
				t.Fatal(err)
			}
		}
		if r.CanBlitColor() || capture != nil || id != "" || tr != (TransferStats{}) || f.RenderSequence() != 0 {
			t.Fatal("stub pretended to have a resident frame")
		}
		if w, h := f.Size(); w != 0 || h != 0 {
			t.Fatal(w, h)
		}
	}
}
