//go:build silk_no_cairo

package paint

// PixmapBrush in the pure-Go build holds only the source pixmap and
// the Extend mode. There's no cairo.Pattern to manage; non-Cairo
// renderers (silk/glui CairoCompat) read the Pixmap directly and
// upload it as a GL texture.
type PixmapBrush struct {
	pixmap Pixmap
	extend Extend
}

// NewPixmapBrush wraps a pixmap into a brush. No Cairo pattern is
// created — silk/glui's CairoCompat reads PixmapBrush.Pixmap()
// directly when it sees this brush type during a fill.
func NewPixmapBrush(pixmap Pixmap) *PixmapBrush {
	return &PixmapBrush{pixmap: pixmap}
}

func (this *PixmapBrush) Extend() Extend { return this.extend }

func (this *PixmapBrush) SetExtend(ext Extend) {
	this.extend = ext
}
