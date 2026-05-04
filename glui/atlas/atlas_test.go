package atlas

import "testing"

func TestPackBasic(t *testing.T) {
	a := New(256, 256)
	r, ok := a.Pack(64, 64)
	if !ok {
		t.Fatal("first pack should succeed")
	}
	if r.X != 0 || r.Y != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", r.X, r.Y)
	}
}

func TestPackOverflow(t *testing.T) {
	a := New(128, 128)
	if _, ok := a.Pack(256, 64); ok {
		t.Error("should not pack region wider than atlas")
	}
}

func TestPackManyGlyphs(t *testing.T) {
	// Simulate packing 256 glyphs of varying sizes (typical font subset).
	a := New(512, 512)
	count := 0
	for i := 0; i < 200; i++ {
		w := 12 + (i%8)*2
		h := 16 + (i%4)*2
		_, ok := a.Pack(w, h)
		if !ok {
			break
		}
		count++
	}
	if count < 150 {
		t.Errorf("only packed %d/200 glyphs; expected >=150", count)
	}
}
