package paint

import "github.com/uk0/silk/cairo"

// Notification bell and a distinct unread dot, using theme palette colors.
func drawNotificationBell(size int, cc *cairo.Context) {
	s := float64(size)
	setIconStroke(cc, size)
	cc.MoveTo(.18*s, .72*s)
	cc.LineTo(.25*s, .60*s)
	cc.LineTo(.25*s, .39*s)
	cc.CurveTo(.25*s, .13*s, .68*s, .13*s, .68*s, .39*s)
	cc.LineTo(.68*s, .60*s)
	cc.LineTo(.76*s, .72*s)
	cc.ClosePath()
	cc.Stroke()
	cc.MoveTo(.38*s, .84*s)
	cc.CurveTo(.40*s, .92*s, .55*s, .92*s, .57*s, .84*s)
	cc.Stroke()
	setIconColor(cc, currentIconPalette.Accent)
	cc.Arc(.80*s, .20*s, .11*s, 0, 6.283185307179586)
	cc.Fill()
}
