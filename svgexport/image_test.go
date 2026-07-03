package svgexport

import (
	"encoding/xml"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/uk0/silk/paint"
)

// makeTestPixmap returns a 4×4 paint.Pixmap with a chequerboard
// red/blue pattern. Both Cairo and pure-Go builds support
// paint.NewPixmap + SetImage so the helper works under either build
// tag.
func makeTestPixmap(t *testing.T) paint.Pixmap {
	t.Helper()
	pix := paint.NewPixmap(4, 4)
	if pix == nil {
		t.Fatal("paint.NewPixmap returned nil")
	}
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if (x+y)%2 == 0 {
				src.Set(x, y, color.RGBA{R: 255, A: 255})
			} else {
				src.Set(x, y, color.RGBA{B: 255, A: 255})
			}
		}
	}
	if err := pix.SetImage(src); err != nil {
		t.Fatalf("pixmap.SetImage: %v", err)
	}
	return pix
}

// TestDrawPixmap1EmitsImageElement: SVG <image> element should appear
// with a base64 data URI.
func TestDrawPixmap1EmitsImageElement(t *testing.T) {
	pix := makeTestPixmap(t)
	p := New(200, 200)
	p.DrawPixmap1(20, 30, pix)

	out := p.String()
	for _, want := range []string{
		`<image`,
		`x="20"`,
		`y="30"`,
		`width="4"`,
		`height="4"`,
		`href="data:image/png;base64,`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n----\n%s", want, out)
		}
	}
}

// TestDrawPixmap5EmitsCustomDimensions: explicit (w, h) should appear
// in the output regardless of pixmap's native dimensions.
func TestDrawPixmap5EmitsCustomDimensions(t *testing.T) {
	pix := makeTestPixmap(t)
	p := New(200, 200)
	p.DrawPixmap5(0, 0, 50, 50, pix)

	out := p.String()
	if !strings.Contains(out, `width="50"`) || !strings.Contains(out, `height="50"`) {
		t.Errorf("explicit DrawPixmap5 dimensions not honoured:\n%s", out)
	}
}

// TestDrawPixmapWithCTMFoldsCoordinates: a Translate before
// DrawPixmap1 should shift the resulting <image> coords.
func TestDrawPixmapWithCTMFoldsCoordinates(t *testing.T) {
	pix := makeTestPixmap(t)
	p := New(200, 200)
	p.Translate(50, 50)
	p.DrawPixmap1(10, 10, pix)

	out := p.String()
	if !strings.Contains(out, `x="60"`) || !strings.Contains(out, `y="60"`) {
		t.Errorf("CTM-translated DrawPixmap1 should land at (60, 60); got\n%s", out)
	}
}

// TestPixmapSVGOutputParsesAsXML: full document with embedded image
// must remain well-formed XML — encoding/xml has to consume it
// without error.
func TestPixmapSVGOutputParsesAsXML(t *testing.T) {
	pix := makeTestPixmap(t)
	p := New(200, 200)
	p.SetBrush1(paint.Color{R: 240, G: 240, B: 240, A: 255})
	p.Rectangle(0, 0, 200, 200)
	p.Fill()
	p.DrawPixmap1(20, 20, pix)
	p.SetBrush1(paint.Color{R: 0, G: 0, B: 0, A: 255})
	p.DrawText1(20, 100, "with image")

	dec := xml.NewDecoder(strings.NewReader(p.String()))
	for {
		tok, err := dec.Token()
		if err != nil && err.Error() == "EOF" {
			break
		}
		if err != nil {
			t.Fatalf("xml parse failed: %v\noutput:\n%s", err, p.String())
		}
		_ = tok
	}
}

// TestNilPixmapIsNoOp guards against nil pixmaps crashing the
// painter — common when a host's pixmap-loading code returns nil.
func TestNilPixmapIsNoOp(t *testing.T) {
	p := New(200, 200)
	p.DrawPixmap(nil)
	p.DrawPixmap1(0, 0, nil)
	p.DrawPixmap5(0, 0, 50, 50, nil)
	out := p.String()
	if strings.Contains(out, `<image`) {
		t.Errorf("nil pixmap should not produce <image>\n%s", out)
	}
}
