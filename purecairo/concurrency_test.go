package purecairo

import (
	"bytes"
	"sync"
	"testing"
)

func TestIndependentContextsRenderTextConcurrently(t *testing.T) {
	render := func() []byte {
		s := NewImageSurface(FORMAT_ARGB32, 240, 40)
		c := s.NewContext()
		c.SelectFontFace("silk-no-system-font-concurrency", FONT_SLANT_NORMAL, FONT_WEIGHT_NORMAL)
		c.SetSourceRGB(0.25, 0.5, 0.75)
		c.MoveTo(2, 24)
		c.ShowText("Concurrent render 123")
		return append([]byte(nil), s.img.Pix...)
	}
	want := render() // warm global font discovery/cache before the workers
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 30 {
				if !bytes.Equal(render(), want) {
					t.Error("independent context rendered corrupted glyphs")
					return
				}
			}
		})
	}
	wg.Wait()
}
