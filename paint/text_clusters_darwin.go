//go:build darwin && cgo && !silk_pure_go

package paint

/*
#cgo pkg-config: cairo
#cgo LDFLAGS: -framework CoreText -framework CoreGraphics -framework CoreFoundation
#include "text_clusters_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"math"
	"unicode/utf8"
	"unsafe"

	"github.com/uk0/silk/cairo"
)

const nativeClusterLayoutAvailable = C.SILK_CLUSTER_AVAILABLE != 0

func nativeComplexSpans(text string) [][2]int {
	data := C.CString(text)
	defer C.free(unsafe.Pointer(data))
	var n C.int
	raw := C.silk_cluster_spans(data, C.size_t(len(text)), &n)
	if raw == nil {
		return nil
	}
	defer C.silk_cluster_free(unsafe.Pointer(raw))
	// Map UTF-16 offsets from Core Foundation back to exact UTF-8 boundaries.
	offsets := make([]int, 0, len(text)+1)
	for i, r := range text {
		offsets = append(offsets, i)
		if r > 0xffff {
			offsets = append(offsets, i)
		}
	}
	offsets = append(offsets, len(text))
	var spans [][2]int
	for _, s := range unsafe.Slice(raw, int(n)) {
		a, b := int(s.begin), int(s.end)
		if a < 0 || b >= len(offsets) || a >= b {
			continue
		}
		first, last := offsets[a], offsets[b]
		if isShapeableCluster(text[first:last]) {
			spans = append(spans, [2]int{first, last})
		}
	}
	return spans
}

type nativeClusterKey struct {
	family, text string
	size         float64
	bold, italic bool
}

var nativeClusterShapes = make(map[nativeClusterKey]TextLayout)
var nativeClusterBytes int

func nativeShapeCluster(sf *scaledFont, text string) (TextLayout, bool) {
	size := float64(sf.font.size)
	if sf.font.nativeSize > 0 {
		size = sf.font.nativeSize
	}
	key := nativeClusterKey{sf.font.family, text, size, sf.font.bold, sf.font.italic}
	if cached, ok := nativeClusterShapes[key]; ok {
		return cloneTextLayout(cached), true
	}
	family := C.CString(sf.font.family)
	defer C.free(unsafe.Pointer(family))
	data := C.CString(text)
	defer C.free(unsafe.Pointer(data))
	var bold, italic C.int
	if sf.font.bold {
		bold = 1
	}
	if sf.font.italic {
		italic = 1
	}
	shaped := C.silk_cluster_shape_create(family, C.double(size), bold, italic, data, C.size_t(len(text)))
	if shaped == nil {
		return TextLayout{}, false
	}
	defer C.silk_cluster_shape_free(shaped)
	count := utf8.RuneCountInString(text)
	out := TextLayout{Carets: make([]float64, count+1), Clusters: []TextCluster{{0, count}}}
	// Internal code-point offsets share the leading edge. This keeps visual
	// widths monotonic without pretending that each part is a separate glyph.
	out.Carets[count] = float64(shaped.advance)
	for _, r := range unsafe.Slice(shaped.runs, int(shaped.count)) {
		key := C.GoString(r.key)
		slot := fontFaceCache[key]
		if slot == nil {
			// Take one native reference; the temporary shape owns the other.
			face := (*cairo.FontFace)(unsafe.Pointer(C.cairo_font_face_reference(r.face)))
			slot = &fontFace{face: face}
			slot.setFinalizer()
			fontFaceCache[key] = slot
		}
		owner := &font{family: C.GoString(r.name), size: int(math.Round(float64(r.size))), nativeSize: float64(r.size), face: slot.face}
		glyphs := make([]Glyph, int(r.count))
		copy(glyphs, unsafe.Slice((*Glyph)(unsafe.Pointer(r.glyphs)), int(r.count)))
		out.Runs = append(out.Runs, GlyphRun{Font: owner, Glyphs: glyphs, Begin: 0, End: count})
	}
	cost := len(text) + len(out.Carets)*8
	for _, r := range out.Runs {
		cost += len(r.Glyphs) * 24
	}
	if cost <= 256<<10 {
		if len(nativeClusterShapes) >= 256 || nativeClusterBytes+cost > 256<<10 {
			nativeClusterShapes = make(map[nativeClusterKey]TextLayout)
			nativeClusterBytes = 0
		}
		nativeClusterShapes[key] = cloneTextLayout(out)
		nativeClusterBytes += cost
	}
	return out, true
}
