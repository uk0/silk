package glui

import "testing"

// TestSDFRectangle paints a 10×10 inside-rect in the middle of a 30×30
// image and checks that the centre saturates to ~255 (deep inside) while
// the outer corners saturate to ~0 (far outside). With spread=4 px the
// rect's centre is ~5 px from the nearest outside pixel — far enough to
// clamp the negative distance to the saturation byte. Corners are
// ~14 px away, which is well past spread on the positive side.
func TestSDFRectangle(t *testing.T) {
	const w, h = 30, 30
	mask := make([]byte, w*h)
	// Fill rect [10,20) × [10,20) — a 10×10 block of "inside" pixels.
	for y := 10; y < 20; y++ {
		for x := 10; x < 20; x++ {
			mask[y*w+x] = 255
		}
	}

	out := generateSDF(mask, w, h, 4)
	if len(out) != w*h {
		t.Fatalf("output size = %d, want %d", len(out), w*h)
	}

	center := out[15*w+15]
	if center < 250 {
		t.Errorf("center byte = %d, want >=250 (deep inside saturates high)", center)
	}

	corner := out[0]
	if corner > 5 {
		t.Errorf("corner byte = %d, want <=5 (far outside saturates low)", corner)
	}

	// Sanity: a pixel just outside the rect edge should sit near the
	// midpoint (contour) — not as extreme as the corners.
	edge := out[10*w+9] // immediately left of rect edge
	if edge < 90 || edge > 160 {
		t.Errorf("edge-adjacent byte = %d, want roughly mid (contour)", edge)
	}
}

// TestSDFEmptyInput verifies the early-return guard handles zero
// dimensions without panicking. The spec contract is "return mask
// unchanged on invalid input"; we also verify that with valid dims
// but a too-small backing slice the same guard fires.
func TestSDFEmptyInput(t *testing.T) {
	got := generateSDF(nil, 0, 0, 4)
	if got != nil {
		t.Errorf("empty input: got non-nil slice of len %d", len(got))
	}

	// Negative spread is also invalid input.
	mask := []byte{0, 0, 0, 0}
	got = generateSDF(mask, 2, 2, 0)
	if &got[0] != &mask[0] {
		t.Errorf("zero spread: expected pass-through to original slice")
	}

	// Backing slice shorter than w*h must not over-read.
	short := []byte{0}
	got = generateSDF(short, 2, 2, 4)
	if &got[0] != &short[0] {
		t.Errorf("short slice: expected pass-through")
	}
}
