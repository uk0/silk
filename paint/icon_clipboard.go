package paint

import (
	"math"

	"github.com/uk0/silk/cairo"
)

func drawCutScissors(size int, cc *cairo.Context) {
	s := float64(size)
	setIconStroke(cc, size)
	for _, x := range []float64{.28, .72} {
		cc.NewSubPath()
		cc.Arc(x*s, .72*s, .13*s, 0, 2*math.Pi)
	}
	cc.Stroke()
	setIconColor(cc, currentIconPalette.Accent)
	cc.MoveTo(.36*s, .62*s)
	cc.LineTo(.78*s, .14*s)
	cc.MoveTo(.64*s, .62*s)
	cc.LineTo(.22*s, .14*s)
	cc.Stroke()
}

func drawPasteClipboard(size int, cc *cairo.Context) {
	s := float64(size)
	setIconStroke(cc, size)
	cc.MoveTo(.32*s, .2*s)
	cc.LineTo(.18*s, .2*s)
	cc.LineTo(.18*s, .86*s)
	cc.LineTo(.82*s, .86*s)
	cc.LineTo(.82*s, .2*s)
	cc.LineTo(.68*s, .2*s)
	cc.Stroke()
	setIconColor(cc, currentIconPalette.Accent)
	cc.Rectangle(.34*s, .1*s, .32*s, .22*s)
	cc.Stroke()
	for _, y := range []float64{.48, .66} {
		cc.MoveTo(.34*s, y*s)
		cc.LineTo(.66*s, y*s)
	}
	cc.Stroke()
}

func drawSelectText(size int, cc *cairo.Context) {
	s := float64(size)
	setIconStroke(cc, size)
	for _, corner := range [][2]float64{{.16, .16}, {.84, .16}, {.16, .84}, {.84, .84}} {
		x, y := corner[0], corner[1]
		dx, dy := .18, .18
		if x > .5 {
			dx = -dx
		}
		if y > .5 {
			dy = -dy
		}
		cc.MoveTo((x+dx)*s, y*s)
		cc.LineTo(x*s, y*s)
		cc.LineTo(x*s, (y+dy)*s)
	}
	cc.Stroke()
	setIconColor(cc, currentIconPalette.Accent)
	for _, y := range []float64{.4, .6} {
		cc.MoveTo(.34*s, y*s)
		cc.LineTo(.66*s, y*s)
	}
	cc.Stroke()
}
