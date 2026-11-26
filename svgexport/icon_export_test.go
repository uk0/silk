package svgexport

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
)

func TestDrawIconPreservesDisabledAppearanceAndAffine(t *testing.T) {
	satin, err := paint.SatinIcon("save")
	if err != nil {
		t.Fatal(err)
	}
	p := New(200, 200)
	var m geom.Mat3x2
	m.Init(2, 1, 0.5, 3, 5, 7)
	p.Transform(&m)
	p.DrawIcon1(satin, 12, 16, 8, true)
	p.DrawIcon1(satin, 12, 16, 8, false)
	var doc struct {
		Images []struct {
			Transform string `xml:"transform,attr"`
			Href      string `xml:"href,attr"`
			Ratio     string `xml:"preserveAspectRatio,attr"`
		} `xml:"image"`
	}
	var data bytes.Buffer
	if _, err := p.WriteTo(&data); err != nil {
		t.Fatal(err)
	}
	if err := xml.Unmarshal(data.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Images) != 2 {
		t.Fatalf("exported %d icons; want two", len(doc.Images))
	}
	disabled := color.NRGBAModel.Convert(paint.CurrentIconPalette().Disabled).(color.NRGBA)
	for i, entry := range doc.Images {
		if entry.Transform != "matrix(2 1 0.5 3 37 67)" || entry.Ratio != "none" {
			t.Fatalf("icon placement = %+v", entry)
		}
		payload, ok := strings.CutPrefix(entry.Href, "data:image/png;base64,")
		if !ok {
			t.Fatal("icon is not an inline PNG")
		}
		b, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		c := img.Bounds()
		if c.Dx() < 24 || c.Dx() != c.Dy() {
			t.Fatalf("unexpected icon raster dimensions: %v", c)
		}
		transparent, opaque, different := 0, 0, 0
		for y := range c.Dy() {
			for x := range c.Dx() {
				v := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
				if v.A == 0 {
					transparent++
				} else if v.A == 255 {
					opaque++
					if v != disabled {
						different++
					}
				}
			}
		}
		if transparent == 0 || opaque == 0 || (i == 0 && different != 0) || (i == 1 && different == 0) {
			t.Fatalf("icon %d: transparent=%d opaque=%d non-disabled color=%d", i, transparent, opaque, different)
		}
	}
}

func TestMillimeterViewportAndTransformedLabel(t *testing.T) {
	p := NewMillimeters(25.4, 50.8)
	p.Translate(5, 7)
	p.Scale(2, 3)
	p.DrawText1(2, 3, "Label")
	s := p.String()
	if !strings.Contains(s, `width="25.4mm" height="50.8mm" viewBox="0 0 25.4 50.8"`) ||
		!strings.Contains(s, `transform="matrix(2 0 0 3 9 16)"`) {
		t.Fatalf("physical viewport or transformed label lost: %s", s)
	}
}
