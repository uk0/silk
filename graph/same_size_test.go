package graph

import (
	"testing"

	"github.com/uk0/silk/geom"
)

// TestSameSizeRect pins the three modes and the min-size floor. The floor cases
// are the ones that regress: a same-size command names an absolute extent, so
// clamping the result straight up to minSize would turn "copy this width" into
// "grow to the floor" for every widget already sitting below it — the same
// failure resizeExtent was fixed for on the shrink key. Routing through
// resizeRectBy keeps the floor able to lower an extent and never raise one.
func TestSameSizeRect(t *testing.T) {
	const minSize = 1.0
	src := geom.Rect{X: 5, Y: 7, Width: 10, Height: 4}
	ref := geom.Rect{X: 100, Y: 100, Width: 30, Height: 8}

	cases := []struct {
		name string
		rect geom.Rect
		ref  geom.Rect
		mode SameSizeMode
		want geom.Rect
	}{
		{
			"width copies width only",
			src, ref, SameSizeWidth,
			geom.Rect{X: 5, Y: 7, Width: 30, Height: 4},
		},
		{
			"height copies height only",
			src, ref, SameSizeHeight,
			geom.Rect{X: 5, Y: 7, Width: 10, Height: 8},
		},
		{
			"both copies the whole size and keeps the position",
			src, ref, SameSizeBoth,
			geom.Rect{X: 5, Y: 7, Width: 30, Height: 8},
		},
		{
			"a sub-floor reference shrinks a normal target only to the floor",
			geom.Rect{X: 0, Y: 0, Width: 20, Height: 20},
			geom.Rect{X: 0, Y: 0, Width: 0.25, Height: 0.25},
			SameSizeBoth,
			geom.Rect{X: 0, Y: 0, Width: minSize, Height: minSize},
		},
		{
			"a sub-floor reference never grows an equally small target",
			geom.Rect{X: 0, Y: 0, Width: 0.5, Height: 0.5},
			geom.Rect{X: 0, Y: 0, Width: 0.5, Height: 0.5},
			SameSizeBoth,
			geom.Rect{X: 0, Y: 0, Width: 0.5, Height: 0.5},
		},
		{
			"a sub-floor target follows a sub-floor reference, not the floor",
			geom.Rect{X: 0, Y: 0, Width: 0.25, Height: 0.25},
			geom.Rect{X: 0, Y: 0, Width: 0.5, Height: 0.5},
			SameSizeBoth,
			geom.Rect{X: 0, Y: 0, Width: 0.5, Height: 0.5},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameSizeRect(tc.rect, tc.ref, tc.mode, minSize); got != tc.want {
				t.Errorf("sameSizeRect(%+v, %+v, %d, %g) = %+v, want %+v",
					tc.rect, tc.ref, tc.mode, minSize, got, tc.want)
			}
		})
	}
}

// TestSameSizeRectLeavesTheReferenceAlone: the reference is part of the
// selection, and resizing it to its own size must produce the identical rect so
// the generator's "nothing changed" filter drops it instead of recording a
// no-op resize for it.
func TestSameSizeRectLeavesTheReferenceAlone(t *testing.T) {
	ref := geom.Rect{X: 3, Y: 4, Width: 30, Height: 8}
	for mode := SameSizeWidth; mode <= SameSizeBoth; mode++ {
		if got := sameSizeRect(ref, ref, mode, 1.0); got != ref {
			t.Errorf("mode %d changed the reference: %+v, want %+v", mode, got, ref)
		}
	}
}
