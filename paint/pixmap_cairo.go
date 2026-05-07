//go:build !silk_no_cairo

package paint

import (
	"math"

	"silk/cairo"
)

// NewPixmap allocates a Cairo image surface of the requested size.
// Returns the Pixmap interface so silk_no_cairo callers can swap in
// imagePixmap without code changes. Format is ARGB32.
func NewPixmap(w, h int) Pixmap {
	s := cairo.NewImageSurface(cairo.FORMAT_ARGB32, w, h)
	p := new(cairoSurface)
	p.Surface = s
	p.setFinalizer()
	return p
}

// LoadPngFile decodes a PNG via libpng (through Cairo). The
// silk_no_cairo build provides a pure-Go equivalent using image/png.
func LoadPngFile(filename string) (Pixmap, error) {
	s, err := cairo.NewImageSurfaceFromPNG(filename)
	if err != nil {
		return nil, err
	}
	p := new(cairoSurface)
	p.Surface = s
	p.setFinalizer()
	return p, nil
}

// Format returns the cairoSurface's pixel layout.
func (this *cairoSurface) Format() Format {
	return Format(this.Surface.Format())
}

// TextToPixmap renders text into a fresh pixmap sized to the text's
// extents. Result is suitable for use with PixmapBrush. Cairo-only —
// the pure-Go build provides a stub that returns nil.
func TextToPixmap(text string, font Font, color Color, border bool) Pixmap {
	te := font.TextExtents(text)
	w := te.Width + 4
	h := te.Height + 4
	pixmap := NewPixmap(int(w), int(h))
	g := pixmap.NewPainter()
	if border {
		g.Rectangle(0, 0, w, h)
		g.Stroke()
	}
	g.SetFont(font)
	g.Translate(2-te.XBearing, 2-te.YBearing)
	g.DrawText(text)
	pixmap.Flush()
	return pixmap
}

// IconTextToPixmap renders an icon followed by text into a single
// pixmap. Cairo-only — the pure-Go build provides a stub.
func IconTextToPixmap(ico Icon, icoSize float64, grayed bool, text string, font Font, textColor Color, border bool) Pixmap {
	if ico == nil {
		return TextToPixmap(text, font, textColor, border)
	}
	te := font.TextExtents(text)
	w := 2 + icoSize + 4 + te.Width + 2
	h := 2 + math.Max(te.Height, icoSize) + 2
	pixmap := NewPixmap(int(w), int(h))
	g := pixmap.NewPainter()
	if border {
		g.Rectangle(0, 0, w, h)
		g.Stroke()
	}
	var yoff float64
	if icoSize < te.Height {
		yoff = 0.5 * (te.Height - icoSize)
	}
	g.Translate(2, yoff+2)
	g.DrawIcon(ico, icoSize, grayed)
	g.Translate(0, -yoff)
	g.SetFont(font)
	yoff = 0
	if icoSize > te.Height {
		yoff = 0.5 * (icoSize - te.Height)
	}
	g.Translate(icoSize+4-te.XBearing, yoff-te.YBearing)
	g.DrawText(text)
	pixmap.Flush()
	return pixmap
}
