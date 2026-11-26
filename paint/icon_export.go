package paint

import "github.com/uk0/silk/geom"

// RasterizeExportIcon renders the same icon colors, coverage and disabled
// treatment as the GUI, onto a transparent surface owned by the caller.
// The 3x output accommodates print and export zoom; cap each icon so a very
// large document transform cannot force an unbounded scratch allocation.
func RasterizeExportIcon(ico Icon, size float64, transform geom.Mat3x2, disabled bool) Pixmap {
	if ico == nil || ico.IsAir() {
		return nil
	}
	pixels := min(1024, iconDeviceSize(size*3, transform))
	if pixels <= 0 {
		return nil
	}
	surface := NewPixmap(pixels, pixels)
	painter := surface.NewPainter()
	painter.DrawIcon(ico, float64(pixels), disabled)
	ClosePainter(painter)
	surface.Flush()
	return surface
}
