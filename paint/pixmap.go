package paint

import (
	"silk/cairo"
	"image"
	"io"
	"math"
	"unsafe"
)

type Format int

const (
	FormatInvalid Format = -1
	FormatARGB32         = 0
	FormatRGB24          = 1
	//FormatA8               = 2
	//FormatA1               = 3
	//FormatRGB16_565        = 4
	//FormatRGB30            = 5
)

type Pixmap interface {
	Surface
	Format() Format
	Width() int
	Height() int
	Stride() int
	DataPtr() unsafe.Pointer
	WritePNGToStream(w io.Writer) error
	WritePNG(filename string) error
	Image() (image.Image, error)
	SetData(src []uint8) error
	SetImage(img image.Image) error
}

func NewPixmap(w, h int) *cairoSurface {
	s := cairo.NewImageSurface(cairo.FORMAT_ARGB32, w, h)
	p := new(cairoSurface)
	p.Surface = s
	p.setFinalizer()
	return p
}

func LoadPngFile(filename string) (*cairoSurface, error) {
	//return nil, core.StrErr("dsafsadf")
	s, err := cairo.NewImageSurfaceFromPNG(filename)
	if err != nil {
		return nil, err
	}
	p := new(cairoSurface)
	p.Surface = s
	p.setFinalizer()
	return p, nil
}

func (this *cairoSurface) Format() Format {
	return Format(this.Surface.Format())
}

func TextToPixmap(text string, font Font, color Color, border bool) *cairoSurface {
	te := font.TextExtents(text)
	w := te.Width + 4
	h := te.Height + 4
	pixmap := NewPixmap(int(w), int(h))
	g := pixmap.NewPainter()
	//	g.SetSourceColor(color)
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

func IconTextToPixmap(ico Icon, icoSize float64, grayed bool, text string, font Font, textColor Color, border bool) *cairoSurface {
	if ico == nil {
		return TextToPixmap(text, font, textColor, border)
	}
	te := font.TextExtents(text)
	w := 2 + icoSize + 4 + te.Width + 2
	h := 2 + math.Max(te.Height, icoSize) + 2
	pixmap := NewPixmap(int(w), int(h))
	g := pixmap.NewPainter()
	//	g.SetSourceColor(textColor)
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
