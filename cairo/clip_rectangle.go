//go:build cgo && !silk_pure_go

package cairo

// #include <cairo.h>
import "C"

import (
	"image"
	"math"
)

// DeviceClipRectangle returns an exact, single, integer-pixel clip. Unlike
// ClipExtents, this never substitutes a bounding box for a curved/complex mask.
// The current path, matrix, operator and save stack are preserved. Coordinates
// are device coordinates (hosts must account for target device offsets).
func (c *Context) DeviceClipRectangle() (image.Rectangle, bool) {
	if c == nil {
		return image.Rectangle{}, false
	}
	cr := (*C.cairo_t)(c)
	if C.cairo_status(cr) != C.CAIRO_STATUS_SUCCESS {
		return image.Rectangle{}, false
	}
	C.cairo_save(cr)
	C.cairo_identity_matrix(cr)
	list := C.cairo_copy_clip_rectangle_list(cr)
	C.cairo_restore(cr)
	if list == nil {
		return image.Rectangle{}, false
	}
	defer C.cairo_rectangle_list_destroy(list)
	if list.status != C.CAIRO_STATUS_SUCCESS || list.num_rectangles != 1 || C.cairo_status(cr) != C.CAIRO_STATUS_SUCCESS {
		return image.Rectangle{}, false
	}
	r := list.rectangles
	edges := [4]float64{float64(r.x), float64(r.y), float64(r.x + r.width), float64(r.y + r.height)}
	for _, x := range edges {
		if math.IsNaN(x) || math.IsInf(x, 0) || x != math.Trunc(x) || x < -(1<<29) || x > 1<<29 {
			return image.Rectangle{}, false
		}
	}
	rect := image.Rect(int(edges[0]), int(edges[1]), int(edges[2]), int(edges[3]))
	return rect, !rect.Empty()
}
