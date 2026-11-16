package ged

import (
	"fmt"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"image"
	"strconv"
	"testing"
)

// Reference is the bounded ordinary-size ruler painter before viewport culling.
func (this *GedView) legacyDrawRulers(g paint.Painter) {
	viewW, viewH := this.Size()
	sceneW, sceneH := this.SceneSizeMm()
	theme := gui.Theme()

	g.Save()
	defer g.Restore()

	g.SetBrush1(theme.FormColor)
	g.Rectangle(0, 0, viewW, rulerThicknessPx)
	g.Rectangle(0, 0, rulerThicknessPx, viewH)
	g.Fill()

	g.SetPen1(theme.BorderColor, 0)
	g.MoveTo(0, rulerThicknessPx)
	g.LineTo(viewW, rulerThicknessPx)
	g.MoveTo(rulerThicknessPx, 0)
	g.LineTo(rulerThicknessPx, viewH)
	g.Stroke()

	horz := legacyRulerTicks(sceneW, rulerTickMm, rulerLabelMm)
	vert := legacyRulerTicks(sceneH, rulerTickMm, rulerLabelMm)

	g.SetPen1(theme.TextColor, 0)
	for _, tick := range horz {
		x, _ := this.MapFromScene(tick.mm, 0)
		if x < rulerThicknessPx || x > viewW {
			continue
		}
		g.MoveTo(x, rulerThicknessPx-rulerTickLen(tick))
		g.LineTo(x, rulerThicknessPx)
	}
	for _, tick := range vert {
		_, y := this.MapFromScene(0, tick.mm)
		if y < rulerThicknessPx || y > viewH {
			continue
		}
		g.MoveTo(rulerThicknessPx-rulerTickLen(tick), y)
		g.LineTo(rulerThicknessPx, y)
	}
	g.Stroke()

	g.SetFont(paint.NewFont("", rulerLabelFontSize, false, false))
	g.SetBrush1(theme.TextColor)
	for _, tick := range horz {
		if !tick.labeled {
			continue
		}
		x, _ := this.MapFromScene(tick.mm, 0)
		if x < rulerThicknessPx || x > viewW {
			continue
		}
		g.DrawText1(x+2, rulerLabelFontSize, strconv.FormatFloat(tick.mm, 'f', 0, 64))
	}
	for _, tick := range vert {
		if !tick.labeled {
			continue
		}
		_, y := this.MapFromScene(0, tick.mm)
		if y < rulerThicknessPx || y > viewH {
			continue
		}
		g.DrawText1(1, y+rulerLabelFontSize, strconv.FormatFloat(tick.mm, 'f', 0, 64))
	}
}

func TestVisibleRulerNormalCairoRasterMatchesLegacy(t *testing.T) {
	for _, zoom := range []float64{.5, 1, 1.25, 2, 100} {
		for _, scroll := range []float64{0, 37.5, 137} {
			t.Run(fmt.Sprintf("zoom-%g/scroll-%g", zoom, scroll), func(t *testing.T) {
				v := NewGedView()
				defer v.Close()
				v.SetSize(360, 260)
				v.GedScene().SetSize(500, 400)
				v.SetZoomFactor(zoom)
				v.Layout()
				v.SetScrollX(scroll)
				v.SetScrollY(scroll)
				var imgs []image.Image
				for i := 0; i < 2; i++ {
					pix := paint.NewPixmap(360, 260)
					g := pix.NewPainter()
					if i == 0 {
						v.legacyDrawRulers(g)
					} else {
						v.drawRulers(g)
					}
					img, err := pix.Image()
					paint.ClosePainter(g)
					paint.CloseSurface(pix)
					if err != nil {
						t.Fatal(err)
					}
					imgs = append(imgs, img)
				}
				assertGridImagesEqual(t, imgs[0], imgs[1])
			})
		}
	}
}
