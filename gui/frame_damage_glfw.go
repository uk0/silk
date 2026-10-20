//go:build !windows

package gui

import (
	"image"
	"math"
	"unsafe"

	"github.com/uk0/silk/geom"
)

// Take only this frame's invalidations. Update called from Draw must survive
// for the NEXT frame; clearing at the end of paint silently loses it.
func (w *Window) takeFrameDamage() (full bool, region geom.Rect) {
	full, region = w.fullDirty, w.dirtyRegion
	w.dirty, w.fullDirty, w.dirtyRegion = false, false, geom.Rect{}
	return
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// framebufferDamage rounds OUTWARDS after adding the antialiasing margin.
// Drawing and texture upload must use exactly the same physical rectangle;
// otherwise fractional HiDPI scaling leaves stale edge pixels on the GPU.
func framebufferDamage(region geom.Rect, full bool, width, height int, sx, sy float64) image.Rectangle {
	frame := image.Rect(0, 0, width, height)
	if full || !finite(sx) || !finite(sy) || sx <= 0 || sy <= 0 ||
		!finite(region.X) || !finite(region.Y) || !finite(region.Width) || !finite(region.Height) ||
		region.Width <= 0 || region.Height <= 0 {
		return frame
	}
	// Clamp while still in float64, before converting potentially huge input
	// to int. Empty/off-screen requests conservatively redraw the frame.
	x0 := math.Max(0, math.Min(float64(width), math.Floor((region.X-1)*sx)))
	y0 := math.Max(0, math.Min(float64(height), math.Floor((region.Y-1)*sy)))
	x1 := math.Max(0, math.Min(float64(width), math.Ceil((region.X+region.Width+1)*sx)))
	y1 := math.Max(0, math.Min(float64(height), math.Ceil((region.Y+region.Height+1)*sy)))
	r := image.Rect(int(x0), int(y0), int(x1), int(y1))
	if x1 <= x0 || y1 <= y0 || float64(r.Dx())*float64(r.Dy()) >= float64(width)*float64(height)*0.85 {
		return frame
	}
	return r
}

// Pack only the upload rectangle into mapped PBO memory. Source rows can have
// padding and a nonzero origin; destination rows are tightly packed BGRA.
func copyFramebufferRegion(dst, src unsafe.Pointer, stride int, region image.Rectangle) {
	rowBytes := region.Dx() * 4
	if rowBytes <= 0 || region.Dy() <= 0 {
		return
	}
	src = unsafe.Add(src, region.Min.Y*stride+region.Min.X*4)
	if stride == rowBytes {
		copyBytes(dst, src, rowBytes*region.Dy())
		return
	}
	for y := 0; y < region.Dy(); y++ {
		copyBytes(unsafe.Add(dst, y*rowBytes), unsafe.Add(src, y*stride), rowBytes)
	}
}
