//go:build windows && !silk_pure_go

package paint

import (
	"testing"
	"unsafe"

	"github.com/uk0/silk/cairo"
)

func TestWindowsOffscreenTextHasNeutralAntialiasedEdges(t *testing.T) {
	if globalFontOptions.Antialias() != cairo.ANTIALIAS_GRAY {
		t.Fatal("offscreen text must not inherit LCD/RDP font smoothing")
	}
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		pix := NewPixmap(int(480*scale), int(42*scale))
		p := pix.NewPainter()
		p.Scale(scale, scale)
		p.SetFont(NewFont("Consolas", 14, false, false))
		p.SetBrush(Color{255, 255, 255, 255})
		p.DrawText1(4, 24, "SilkIDE Aa0123 / 中文抗锯齿")
		pix.Flush()
		data := unsafe.Slice((*byte)(pix.DataPtr()), pix.Stride()*pix.Height())
		partial, solid := 0, 0
		for i := 0; i < len(data); i += 4 {
			if data[i] != data[i+1] || data[i] != data[i+2] {
				t.Fatalf("scale=%g LCD color fringe at byte %d: %v", scale, i, data[i:i+4])
			}
			if data[i+3] > 0 && data[i+3] < 255 {
				partial++
			}
			if data[i+3] == 255 {
				solid++
			}
		}
		if partial < 20 || solid == 0 {
			t.Fatalf("scale=%g aliased/empty text: partial=%d solid=%d", scale, partial, solid)
		}
		t.Logf("scale=%g neutral partial=%d solid=%d", scale, partial, solid)
		ClosePainter(p)
		CloseSurface(pix)
	}
}
