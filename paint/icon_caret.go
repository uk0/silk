package paint

import "github.com/uk0/silk/cairo"

// Keep the insertion-point silhouette legible at 16px. Arrows use the existing
// theme palette and are rasterized at the actual HiDPI device transform.
func drawCaretCommand(direction int) proceduralIconDrawer {
	return func(size int, cc *cairo.Context) {
		s := float64(size)
		setIconStroke(cc, size)
		x := 0.3
		if direction == 0 {
			x = 0.5
		}
		cc.MoveTo((x-.12)*s, .2*s)
		cc.LineTo((x+.12)*s, .2*s)
		cc.MoveTo(x*s, .2*s)
		cc.LineTo(x*s, .8*s)
		cc.MoveTo((x-.12)*s, .8*s)
		cc.LineTo((x+.12)*s, .8*s)
		cc.Stroke()
		if direction == 0 {
			return
		}
		setIconColor(cc, currentIconPalette.Accent)
		y := func(v float64) float64 {
			if direction > 0 {
				return (1 - v) * s
			}
			return v * s
		}
		cc.MoveTo(.72*s, y(.75))
		cc.LineTo(.72*s, y(.25))
		cc.MoveTo(.56*s, y(.43))
		cc.LineTo(.72*s, y(.25))
		cc.LineTo(.88*s, y(.43))
		cc.Stroke()
	}
}

// The line stack distinguishes text movement from adding an insertion point.
func drawLineMove(direction int) proceduralIconDrawer {
	return func(size int, cc *cairo.Context) {
		s := float64(size)
		setIconStroke(cc, size)
		for _, y := range []float64{.3, .5, .7} {
			cc.MoveTo(.12*s, y*s)
			cc.LineTo(.46*s, y*s)
		}
		cc.Stroke()
		setIconColor(cc, currentIconPalette.Accent)
		y := func(v float64) float64 {
			if direction > 0 {
				return (1 - v) * s
			}
			return v * s
		}
		cc.MoveTo(.74*s, y(.8))
		cc.LineTo(.74*s, y(.2))
		cc.MoveTo(.58*s, y(.38))
		cc.LineTo(.74*s, y(.2))
		cc.LineTo(.9*s, y(.38))
		cc.Stroke()
	}
}

// A line stack plus/minus distinguishes duplicate/delete from clipboard actions.
func drawLineChange(remove bool) proceduralIconDrawer {
	return func(size int, cc *cairo.Context) {
		s := float64(size)
		setIconStroke(cc, size)
		for _, y := range []float64{.25, .45, .65} {
			cc.MoveTo(.12*s, y*s)
			cc.LineTo(.52*s, y*s)
		}
		cc.Stroke()
		color := currentIconPalette.Accent
		if remove {
			color = currentIconPalette.Danger
		}
		setIconColor(cc, color)
		cc.MoveTo(.62*s, .7*s)
		cc.LineTo(.94*s, .7*s)
		if !remove {
			cc.MoveTo(.78*s, .54*s)
			cc.LineTo(.78*s, .86*s)
		}
		cc.Stroke()
	}
}
