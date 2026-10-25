package paint

import "github.com/uk0/silk/cairo"

// Text plus a small eraser, not a generic delete/X: the command removes only
// trailing blanks. Drawn at device scale using the established icon palette.
func drawWhitespaceTrim(size int, cc *cairo.Context) {
	s := float64(size)
	setIconStroke(cc, size)
	for _, y := range []float64{.25, .45} {
		cc.MoveTo(.12*s, y*s)
		cc.LineTo(.52*s, y*s)
		cc.MoveTo(.68*s, y*s)
		cc.LineTo(.70*s, y*s)
		cc.MoveTo(.84*s, y*s)
		cc.LineTo(.86*s, y*s)
	}
	cc.Stroke()
	setIconColor(cc, currentIconPalette.Accent)
	cc.MoveTo(.50*s, .62*s)
	cc.LineTo(.68*s, .79*s)
	cc.LineTo(.55*s, .92*s)
	cc.LineTo(.37*s, .75*s)
	cc.ClosePath()
	cc.Stroke()
	cc.MoveTo(.58*s, .92*s)
	cc.LineTo(.89*s, .92*s)
	cc.Stroke()
}

// A return arrow ending at a new line remains legible without a font glyph.
func drawNewlineInsert(size int, cc *cairo.Context) {
	s := float64(size)
	setIconStroke(cc, size)
	cc.MoveTo(.12*s, .22*s)
	cc.LineTo(.57*s, .22*s)
	cc.MoveTo(.12*s, .85*s)
	cc.LineTo(.55*s, .85*s)
	cc.Stroke()
	setIconColor(cc, currentIconPalette.Accent)
	cc.MoveTo(.84*s, .25*s)
	cc.LineTo(.84*s, .59*s)
	cc.LineTo(.30*s, .59*s)
	cc.MoveTo(.47*s, .43*s)
	cc.LineTo(.30*s, .59*s)
	cc.LineTo(.47*s, .75*s)
	cc.Stroke()
}
