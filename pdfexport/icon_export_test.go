package pdfexport

import (
	"bytes"
	"compress/zlib"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

func TestDrawIconRetainsDisabledPaletteMaskAndAffine(t *testing.T) {
	ico, err := paint.SatinIcon("save")
	if err != nil {
		t.Fatal(err)
	}
	p := New(200, 200)
	var m geom.Mat3x2
	m.Init(2, 1, 0.5, 3, 5, 7)
	p.Transform(&m)
	p.DrawIcon1(ico, 12, 16, 8, true)
	if len(p.images) != 1 || !strings.Contains(p.content.String(), "16 -8 -4 24 41 109 cm\n/Im1 Do") {
		t.Fatalf("missing affine icon placement: %q", p.content.String())
	}
	img := p.images[0]
	inflate := func(data []byte) []byte {
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	mask, rgb := inflate(img.smask), inflate(img.compressed)
	if len(mask) != img.width*img.height || len(rgb) != 3*len(mask) {
		t.Fatal("embedded alpha or RGB size mismatch")
	}
	disabled := paint.CurrentIconPalette().Disabled
	seenZero, seenOpaque := false, false
	for i, a := range mask {
		switch a {
		case 0:
			seenZero = true
		case 255:
			seenOpaque = true
			if v := rgb[3*i : 3*i+3]; !bytes.Equal(v, []byte{disabled.R, disabled.G, disabled.B}) {
				t.Fatalf("disabled PDF icon pixel = %v, want %v", v, disabled)
			}
		}
	}
	if !seenZero || !seenOpaque {
		t.Fatal("PDF icon did not retain both transparency and opaque ink")
	}
}

func TestMillimeterPagesScaleGeometryOnceAndKeepTextUpright(t *testing.T) {
	p := NewMillimeters(25.4, 50.8)
	p.Translate(5, 7)
	p.Scale(2, 3)
	p.DrawText1(2, 3, "Up")
	p.NewPage1(50.8, 25.4)
	p.DrawText1(8, 9, "Page 2")
	pages := p.snapshotPages()
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	for i, want := range [][2]float64{{72, 144}, {144, 72}} {
		page := pages[i]
		if math.Abs(page.width-want[0]) > 1e-9 || math.Abs(page.height-want[1]) > 1e-9 {
			t.Fatalf("page %d = %.12g x %.12g pt, want %g x %g pt", i+1, page.width, page.height, want[0], want[1])
		}
	}
	for _, page := range pages {
		if strings.Count(page.content, "2.834645669291 0 0 2.834645669291 0 0 cm") != 1 {
			t.Fatalf("physical scale is missing or duplicated: %q", page.content)
		}
	}
	if !strings.Contains(pages[0].content, "2 0 0 3 9 34.8 Tm") ||
		!strings.Contains(pages[1].content, "1 0 0 1 8 16.4 Tm") {
		t.Fatalf("text direction or page-reset behaviour is wrong: %v", pages)
	}
	if a, b := p.String(), p.String(); a != b {
		t.Fatal("writing a millimetre document changed the next serialization")
	}
}

func TestQuarterTurnKeepsPDFOperatorsValidAndClipAligned(t *testing.T) {
	p := New(100, 100)
	p.Translate(60, 15)
	p.Rotate(math.Pi / 2)
	p.Rectangle(0, 0, 12, 8)
	p.Clip()
	ico, err := paint.SatinIcon("save")
	if err != nil {
		t.Fatal(err)
	}
	p.DrawIcon1(ico, 0, 0, 8, false)
	p.DrawText1(0, 5, "Up")
	content := p.content.String()
	for _, want := range []string{"52 85 m\n52 73 l\n60 73 l\n60 85 l\nh\nW\nn", "/Im1 Do", "(Up) Tj"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing transformed clip, icon or text %q:\n%s", want, content)
		}
	}
	for _, line := range strings.Split(content, "\n") {
		if (strings.HasSuffix(line, " cm") || strings.HasSuffix(line, " Tm")) &&
			strings.ContainsAny(line, "eE") {
			t.Errorf("PDF real operands cannot have exponents: %q", line)
		}
	}
}
