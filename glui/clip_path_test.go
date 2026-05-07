package glui

import "testing"

// Stencil clip tests run against newAdapterTestRenderer (no GL context),
// so they observe state mutation only. Real-GL stencil writes are
// exercised by glui_demo when a Window is available; the unit tests
// pin the bookkeeping invariants.

// TestPushClipPathAcceptsValidPolygon: a triangle path increments
// curStencilRef and lands on the clip stack.
func TestPushClipPathAcceptsValidPolygon(t *testing.T) {
	r := newAdapterTestRenderer()
	pts := [][2]float32{{0, 0}, {10, 0}, {5, 10}}

	if r.curStencilRef != 0 {
		t.Fatalf("initial curStencilRef = %d, want 0", r.curStencilRef)
	}
	r.PushClipPath(pts)
	if r.curStencilRef != 1 {
		t.Errorf("after push: curStencilRef = %d, want 1", r.curStencilRef)
	}
	if len(r.clipStack) != 1 {
		t.Errorf("clipStack len = %d, want 1", len(r.clipStack))
	}
	if r.curClip.kind != clipKindStencil {
		t.Errorf("curClip.kind = %v, want clipKindStencil", r.curClip.kind)
	}
	if r.curClip.depth != 1 {
		t.Errorf("curClip.depth = %d, want 1", r.curClip.depth)
	}
}

// TestPushClipPathDegenerateIsTracked: a path with fewer than 3 points
// still pushes a record so PopClipPath has something to unwind. The
// record's kind is clipKindStencilEmpty so a host can detect it if
// needed; depth tracking continues to work.
func TestPushClipPathDegenerateIsTracked(t *testing.T) {
	r := newAdapterTestRenderer()
	r.PushClipPath([][2]float32{{0, 0}, {1, 1}}) // 2 pts — degenerate
	if len(r.clipStack) != 1 {
		t.Errorf("degenerate push didn't extend stack: len=%d", len(r.clipStack))
	}
	if r.curClip.kind != clipKindStencilEmpty {
		t.Errorf("kind = %v, want clipKindStencilEmpty", r.curClip.kind)
	}
}

// TestPopClipPathRewindsStencilRef: a Push followed by Pop returns
// curStencilRef to 0 and pops the stack entry.
func TestPopClipPathRewindsStencilRef(t *testing.T) {
	r := newAdapterTestRenderer()
	pts := [][2]float32{{0, 0}, {10, 0}, {5, 10}}
	r.PushClipPath(pts)
	r.PopClipPath(pts)
	if r.curStencilRef != 0 {
		t.Errorf("after pop: curStencilRef = %d, want 0", r.curStencilRef)
	}
	if len(r.clipStack) != 0 {
		t.Errorf("stack should be empty after balanced pop, got len=%d", len(r.clipStack))
	}
}

// TestNestedStencilClipBumpsDepth: two nested PushClipPath calls
// reach depth 2; rewinding gets us back to 0.
func TestNestedStencilClipBumpsDepth(t *testing.T) {
	r := newAdapterTestRenderer()
	a := [][2]float32{{0, 0}, {100, 0}, {100, 100}, {0, 100}}
	b := [][2]float32{{20, 20}, {80, 20}, {80, 80}, {20, 80}}

	r.PushClipPath(a)
	if r.curStencilRef != 1 {
		t.Fatalf("first push depth = %d, want 1", r.curStencilRef)
	}
	r.PushClipPath(b)
	if r.curStencilRef != 2 {
		t.Errorf("nested push depth = %d, want 2", r.curStencilRef)
	}
	r.PopClipPath(b)
	if r.curStencilRef != 1 {
		t.Errorf("after inner pop depth = %d, want 1", r.curStencilRef)
	}
	r.PopClipPath(a)
	if r.curStencilRef != 0 {
		t.Errorf("after outer pop depth = %d, want 0", r.curStencilRef)
	}
}

// TestMixedScissorAndStencilStack: PushClip + PushClipPath alternate
// on the same stack; each Pop matches its kind.
func TestMixedScissorAndStencilStack(t *testing.T) {
	r := newAdapterTestRenderer()
	r.frameW, r.frameH = 200, 200
	r.PushClip(Rect{0, 0, 200, 200})
	if len(r.clipStack) != 1 {
		t.Fatalf("after scissor push: len=%d", len(r.clipStack))
	}
	if r.curClip.kind != clipKindScissor {
		t.Errorf("expected scissor kind after PushClip, got %v", r.curClip.kind)
	}

	r.PushClipPath([][2]float32{{10, 10}, {100, 10}, {50, 100}})
	if len(r.clipStack) != 2 {
		t.Fatalf("after stencil push: len=%d", len(r.clipStack))
	}
	if r.curClip.kind != clipKindStencil {
		t.Errorf("expected stencil kind after PushClipPath, got %v", r.curClip.kind)
	}
	if r.curStencilRef != 1 {
		t.Errorf("stencil ref = %d, want 1", r.curStencilRef)
	}

	// PopClipPath unwinds stencil → expect prior scissor on top.
	r.PopClipPath([][2]float32{{10, 10}, {100, 10}, {50, 100}})
	if r.curClip.kind != clipKindScissor {
		t.Errorf("after PopClipPath: kind = %v, want clipKindScissor", r.curClip.kind)
	}
	if r.curStencilRef != 0 {
		t.Errorf("stencil ref after pop = %d, want 0", r.curStencilRef)
	}
}

// TestPopClipPathWithEmptyStackIsSafe: defensive — PopClipPath with no
// outstanding push is a no-op, not a panic.
func TestPopClipPathWithEmptyStackIsSafe(t *testing.T) {
	r := newAdapterTestRenderer()
	// No prior push.
	r.PopClipPath([][2]float32{{0, 0}, {10, 0}, {5, 10}})
	if r.curStencilRef != 0 {
		t.Errorf("ref after lone pop = %d, want 0", r.curStencilRef)
	}
}

// TestCairoCompatRotatedClipUsesStencil: when the CTM has rotation
// (Xy or Yx non-zero), Clip() routes through PushClipPath and the
// resulting clipPushedAt entry has isStencil=true.
func TestCairoCompatRotatedClipUsesStencil(t *testing.T) {
	c, r := newCompatTestPainter(t)
	r.frameW, r.frameH = 200, 200

	// Rotate 45 degrees; Mat3x2.Rotate sets off-diagonal entries.
	c.Rotate(0.7853981633974483) // π/4
	c.Rectangle(0, 0, 100, 100)
	c.Clip()

	if len(c.clipPushedAt) != 1 {
		t.Fatalf("clipPushedAt len = %d, want 1", len(c.clipPushedAt))
	}
	if !c.clipPushedAt[0].isStencil {
		t.Errorf("rotated clip should route to stencil; got isStencil=false")
	}
	if r.curStencilRef != 1 {
		t.Errorf("renderer stencil ref = %d, want 1", r.curStencilRef)
	}
	if r.curClip.kind != clipKindStencil {
		t.Errorf("renderer curClip.kind = %v, want clipKindStencil", r.curClip.kind)
	}
}

// TestCairoCompatNonRotatedClipUsesScissor: a translation-only CTM
// produces axis-aligned clip rects and stays on the scissor fast
// path. No stencil bookkeeping touched.
func TestCairoCompatNonRotatedClipUsesScissor(t *testing.T) {
	c, r := newCompatTestPainter(t)
	r.frameW, r.frameH = 200, 200

	c.Translate(50, 30)
	c.Rectangle(0, 0, 100, 100)
	c.Clip()

	if len(c.clipPushedAt) != 1 {
		t.Fatalf("clipPushedAt len = %d, want 1", len(c.clipPushedAt))
	}
	if c.clipPushedAt[0].isStencil {
		t.Errorf("non-rotated clip should stay on scissor path")
	}
	if r.curStencilRef != 0 {
		t.Errorf("scissor path should not touch stencil ref, got %d", r.curStencilRef)
	}
}

// TestCairoCompatRestoreUnwindsBothKinds: a scope that pushed both a
// stencil clip and a scissor clip in sequence must Restore in the
// correct order so each Pop matches its kind.
func TestCairoCompatRestoreUnwindsBothKinds(t *testing.T) {
	c, r := newCompatTestPainter(t)
	r.frameW, r.frameH = 200, 200

	c.Save()

	// First, a scissor clip (no rotation).
	c.Rectangle(0, 0, 200, 200)
	c.Clip()

	// Then nested rotation + path → stencil clip.
	c.Save()
	c.Rotate(0.5)
	c.Rectangle(20, 20, 80, 80)
	c.Clip()

	if len(c.clipPushedAt) != 2 {
		t.Fatalf("expected 2 clips on stack, got %d", len(c.clipPushedAt))
	}
	if !c.clipPushedAt[1].isStencil {
		t.Errorf("inner clip (rotated) should be stencil")
	}
	if c.clipPushedAt[0].isStencil {
		t.Errorf("outer clip (non-rotated) should be scissor")
	}

	// Restore inner — pops stencil, leaves scissor.
	c.Restore()
	if len(c.clipPushedAt) != 1 {
		t.Fatalf("after inner Restore: %d clips, want 1", len(c.clipPushedAt))
	}
	if r.curStencilRef != 0 {
		t.Errorf("after stencil Pop: ref = %d, want 0", r.curStencilRef)
	}
	if c.clipPushedAt[0].isStencil {
		t.Errorf("remaining clip should be scissor")
	}

	// Restore outer — pops scissor.
	c.Restore()
	if len(c.clipPushedAt) != 0 {
		t.Errorf("after outer Restore: %d clips, want 0", len(c.clipPushedAt))
	}
}
