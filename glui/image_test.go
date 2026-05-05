package glui

import "testing"

// TestTextureDimensions verifies the Texture accessors report the values
// the constructor stored. We don't call UploadTexture because it requires
// a live GL context; instead we build the struct directly.
func TestTextureDimensions(t *testing.T) {
	tex := &Texture{id: 7, width: 64, height: 32}
	if tex.ID() != 7 {
		t.Errorf("ID() = %d, want 7", tex.ID())
	}
	if tex.Width() != 64 {
		t.Errorf("Width() = %d, want 64", tex.Width())
	}
	if tex.Height() != 32 {
		t.Errorf("Height() = %d, want 32", tex.Height())
	}
}

// TestNilTextureSafe ensures DrawImage and DrawImageRegion early-return
// without panicking when handed a nil texture or one with id == 0.
func TestNilTextureSafe(t *testing.T) {
	r := newTestRenderer()
	dst := Rect{0, 0, 10, 10}
	src := Rect{0, 0, 5, 5}

	r.DrawImage(nil, dst, Color{1, 1, 1, 1})
	r.DrawImageRegion(nil, src, dst, Color{1, 1, 1, 1})

	empty := &Texture{}
	r.DrawImage(empty, dst, Color{1, 1, 1, 1})
	r.DrawImageRegion(empty, src, dst, Color{1, 1, 1, 1})

	// Zero-sized texture also rejected by the region path.
	zeroSize := &Texture{id: 1, width: 0, height: 0}
	r.DrawImageRegion(zeroSize, src, dst, Color{1, 1, 1, 1})

	if len(r.verts) != 0 {
		t.Errorf("nil/empty texture pushed %d verts; want 0", len(r.verts))
	}
	if len(r.indices) != 0 {
		t.Errorf("nil/empty texture pushed %d indices; want 0", len(r.indices))
	}
}
