package paint

import (
	"github.com/uk0/silk/cairo"
	"github.com/uk0/silk/core"
	//	"fmt"
	"runtime"
)

var cairoSurfaceCount = 0

type Surface interface {
	SurfaceType() cairo.SurfaceType
	NewPainter() Painter
	// NewSimilar(w, h int, color, alpha bool) Pixmap  // should be NewSimilarPixmap
	Flush()
}

type cairoSurface struct {
	*cairo.Surface
}

/*
func (this *cairoSurface) NewSimilar(w, h int, color, alpha bool) Surface {

	var content cairo.Content
	if color && alpha {
		content = cairo.CONTENT_COLOR_ALPHA
	} else if color {
		content = cairo.CONTENT_COLOR
	} else if alpha {
		content = cairo.CONTENT_ALPHA
	} else {
		core.Warn(`both "color" and "aplha" is flase`)
		content = cairo.CONTENT_COLOR_ALPHA
	}

	//	w32s := this.Surface.NewSimilar(content, w, h)

	p := new(cairoSurface)
	p.Surface = this.Surface.NewSimilar(content, w, h)
	p.setFinalizer()
	return p

}
*/
func (this *cairoSurface) setFinalizer() {
	cairoSurfaceCount++
	//fmt.Println("cairoSurfaceCount =", cairoSurfaceCount)
	if cairoSurfaceCount > 2000 && cairoSurfaceCount%100 == 0 {
		core.Warn("seems cairo surface leaks, count = ", cairoSurfaceCount)
	}
	runtime.SetFinalizer(this, func(p *cairoSurface) {
		p.Surface.Destroy()
		cairoSurfaceCount--
	})
}

func (this *cairoSurface) SurfaceType() cairo.SurfaceType {
	return this.Surface.Type()
}

func (this *cairoSurface) NewPainter() Painter {
	painter := new(cairoPainter)
	painter.setFinalizer()
	painter.cairo = this.Surface.NewContext()
	painter.SetPen(NewPen(Color{0, 0, 0, 255}, 1))
	//painter.surface = this
	//core.Warn("x")
	return painter
}
