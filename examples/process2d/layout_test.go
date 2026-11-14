package main

import (
	"github.com/uk0/silk/gui"
	"testing"
)

func TestProcessDemoResponsiveBoundsAndWorkerShutdown(t *testing.T) {
	t.Setenv("SILK_DATA_DIR", t.TempDir())
	f := newDemo()
	defer func() {
		f.cancel()
		<-f.done
		for _, stop := range f.cancels {
			stop()
		}
		f.server.Close()
		f.store.Close()
	}()
	for _, size := range [][2]float64{{1000, 510}, {780, 620}, {760, 580}} {
		f.SetSize(size[0], size[1])
		f.OnResize()
		var widgets []gui.IWidget
		for _, w := range f.buttons {
			widgets = append(widgets, w)
		}
		for _, w := range f.equipment {
			widgets = append(widgets, w)
		}
		for _, w := range f.values {
			widgets = append(widgets, w)
		}
		for _, w := range widgets {
			x, y, width, height := w.Bounds()
			if x < 0 || y < 0 || x+width > size[0]-12 || y+height > size[1]-46 {
				t.Fatalf("%T escapes %v: %v %v %v %v", w, size, x, y, width, height)
			}
		}
	}
}
