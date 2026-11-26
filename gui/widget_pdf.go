package gui

import (
	"fmt"
	"image"
	"io"
	"math"
	"reflect"

	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/pdfexport"
)

// RenderWidgetPDF captures a single viewport at 288 DPI (three samples per
// logical pixel). Call it on the UI thread for a widget you own. The printed
// page has the same physical size as the viewport, with a white backing.
// Text is not selectable; the printer/driver may apply its own color profile.
func RenderWidgetPDF(out io.Writer, widget IWidget) error {
	if out == nil {
		return fmt.Errorf("print: output is required")
	}
	snapshot, err := captureWidgetPDF(widget)
	if err != nil {
		return err
	}
	return snapshot.writePDF(out)
}

// Go-owned pixels only: safe to consume after closing the underlying Cairo
// context and surface. The snapshot is not mutated by PDF encoding.
type widgetPDFSnapshot struct {
	width, height float64 // PDF points
	pixels        image.Image
}

func captureWidgetPDF(widget IWidget) (*widgetPDFSnapshot, error) {
	if widget == nil || (reflect.ValueOf(widget).Kind() == reflect.Pointer && reflect.ValueOf(widget).IsNil()) {
		return nil, fmt.Errorf("print: widget is required")
	}
	w, h := widget.Width(), widget.Height()
	const scale = 3.0
	pw, ph := math.Ceil(w*scale), math.Ceil(h*scale)
	if math.IsNaN(w) || math.IsNaN(h) || math.IsInf(w, 0) || math.IsInf(h, 0) ||
		w <= 0 || h <= 0 || pw*ph > 32*1024*1024 || pw > 32768 || ph > 32768 {
		return nil, fmt.Errorf("print: invalid or oversized widget viewport %g x %g", w, h)
	}
	pix := paint.NewPixmap(int(pw), int(ph))
	defer paint.CloseSurface(pix)
	p := pix.NewPainter()
	defer paint.ClosePainter(p)
	p.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
	p.Rectangle(0, 0, pw, ph)
	p.Fill()
	p.Scale(scale, scale)
	DrawWidgetAll(widget, p, 0, 0, 0, 0, w, h)
	pixels, err := pix.Image()
	if err != nil {
		return nil, fmt.Errorf("print: rasterize widget: %w", err)
	}
	return &widgetPDFSnapshot{w * 72 / 96, h * 72 / 96, pixels}, nil
}

func (s *widgetPDFSnapshot) writePDF(out io.Writer) error {
	if s == nil || s.pixels == nil {
		return fmt.Errorf("print: missing raster snapshot")
	}
	pdf := pdfexport.New(s.width, s.height)
	pdf.SetCompression(true)
	if err := pdf.DrawRaster(0, 0, s.width, s.height, s.pixels); err != nil {
		return err
	}
	_, err := pdf.WriteTo(out)
	return err
}
