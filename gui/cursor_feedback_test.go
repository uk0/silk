package gui

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"github.com/uk0/silk/paint"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinDropCursorAssetsAvailable(t *testing.T) {
	for _, name := range []string{"arrow-no", "arrow", "arrow-copy", "arrow-link"} {
		data, err := LoadCursorData(name)
		if err != nil {
			t.Errorf("%s: no built-in drag feedback: %v", name, err)
			continue
		}
		if data.Pixmap == nil {
			t.Errorf("%s: missing pixels", name)
			continue
		}
		if data.HotX < 0 || data.HotY < 0 || data.HotX >= data.Width() || data.HotY >= data.Height() {
			t.Errorf("%s: invalid hotspot", name)
		}
		paint.CloseSurface(data.Pixmap)
	}
}

func TestDropCursorArtworkAndThumbnail(t *testing.T) {
	seen := map[[32]byte]bool{}
	content := paint.NewPixmap(40, 20)
	defer paint.CloseSurface(content)
	p := content.NewPainter()
	p.SetBrush1(paint.Color{R: 240, G: 40, B: 20, A: 255})
	p.Rectangle(0, 0, 40, 20)
	p.Fill()
	paint.ClosePainter(p)
	for _, name := range dropCursorNames {
		arrow, ok := builtinDropCursorData(name)
		if !ok {
			t.Fatal(name)
		}
		img, err := arrow.Image()
		if err != nil {
			t.Fatal(err)
		}
		var encoded bytes.Buffer
		png.Encode(&encoded, img)
		sum := sha256.Sum256(encoded.Bytes())
		if seen[sum] {
			t.Fatal("actions have indistinguishable artwork", name)
		}
		seen[sum] = true
		data, err := composeDropCursor(content, arrow)
		if err != nil {
			t.Fatal(err)
		}
		if data.Width() != 56 || data.Height() != 36 || data.HotX != 2 || data.HotY != 2 {
			t.Fatal("wrong preview or hotspot", data)
		}
		image, err := data.Image()
		if err != nil {
			t.Fatal(err)
		}
		r, g, b, a := image.At(40, 25).RGBA()
		if r != 240*257 || g != 40*257 || b != 20*257 || a != 65535 {
			t.Fatal("content thumbnail missing", r, g, b, a)
		}
		paint.CloseSurface(data.Pixmap)
		paint.CloseSurface(arrow.Pixmap)
	}
	if content.Width() != 40 {
		t.Fatal("borrowed content was retired")
	}
}

func TestDropCursorBoundsAndInvalidHotspots(t *testing.T) {
	arrow, _ := builtinDropCursorData("arrow-copy")
	defer paint.CloseSurface(arrow.Pixmap)
	huge := paint.NewPixmap(2048, 1024)
	defer paint.CloseSurface(huge)
	data, err := composeDropCursor(huge, arrow)
	if err != nil {
		t.Fatal(err)
	}
	defer paint.CloseSurface(data.Pixmap)
	if data.Width() > 96 || data.Height() > 80 {
		t.Fatal("unbounded drag thumbnail", data.Width(), data.Height())
	}
	for _, point := range [][2]int{{-1, 0}, {0, -1}, {32, 0}, {0, 32}} {
		bad := arrow
		bad.HotX, bad.HotY = point[0], point[1]
		if _, err := composeDropCursor(nil, bad); err == nil {
			t.Fatal("invalid hotspot accepted", point)
		}
	}
	if _, err := composeDropCursor(nil, CursorData{}); err == nil {
		t.Fatal("nil arrow accepted")
	}
}

func TestCursorPNGOverrideAndValidation(t *testing.T) {
	dir := t.TempDir()
	makePNG := func(name string, w, h int) {
		t.Helper()
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, w, h)))
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	makePNG("Arrow-Copy_3_4.PNG", 8, 9)
	makePNG("custom_arrow_1_2.png", 8, 9)
	custom, err := loadCursorDataFromDir(dir, "custom_arrow")
	if err != nil {
		t.Fatal(err)
	}
	if custom.HotX != 1 || custom.HotY != 2 {
		t.Fatal("underscored name lost hotspot")
	}
	paint.CloseSurface(custom.Pixmap)
	data, err := loadCursorDataFromDir(dir, "ARROW-COPY")
	if err != nil {
		t.Fatal(err)
	}
	if data.Width() != 8 || data.Height() != 9 || data.HotX != 3 || data.HotY != 4 {
		t.Fatal("override ignored", data)
	}
	paint.CloseSurface(data.Pixmap)
	for _, name := range []string{"bad_-1_0.png", "badparse_x_0.png", "outside_8_0.png", "oversized.png"} {
		w := 8
		if name == "oversized.png" {
			w = 257
		}
		makePNG(name, w, 8)
	}
	for _, name := range []string{"bad", "badparse", "outside", "oversized"} {
		if _, err := loadCursorDataFromDir(dir, name); err == nil {
			t.Fatal("malformed override accepted", name)
		}
	}
	os.WriteFile(filepath.Join(dir, "arrow-no.png"), []byte("not a PNG"), 0600)
	if _, err := loadCursorDataFromDir(dir, "arrow-no"); err == nil {
		t.Fatal("corrupt override hidden by fallback")
	}
	if _, err := loadCursorDataFromDir(dir, "unknown"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("unknown name not reported", err)
	}
}

func TestReleaseDropCursorsPreservesSharedAndDuplicates(t *testing.T) {
	c := new(Cursor)
	ReleaseDropCursors([]*Cursor{c, c, nil, DefaultCursor(), cursorNo})
	ReleaseDropCursors([]*Cursor{c, DefaultCursor()})
	if LoadCursor("ARROW") != DefaultCursor() {
		t.Fatal("system cursor alias was replaced")
	}
}

func BenchmarkComposeDropCursor(b *testing.B) {
	arrow, _ := builtinDropCursorData("arrow-copy")
	defer paint.CloseSurface(arrow.Pixmap)
	content := paint.NewPixmap(160, 60)
	defer paint.CloseSurface(content)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := composeDropCursor(content, arrow)
		if err != nil {
			b.Fatal(err)
		}
		paint.CloseSurface(data.Pixmap)
	}
}
