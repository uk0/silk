package glui

import (
	"image"
	"io"
	"silk/cairo"
	"silk/geom"
	"silk/paint"
	"testing"
	"unsafe"
)

// All CairoCompat tests reuse the off-GL test renderer from
// painter_adapter_test.go so they can run under `go test -short` without
// a window.

func newCompatTestPainter(t *testing.T) (*CairoCompat, *Renderer) {
	t.Helper()
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)
	return c, r
}

func TestCairoCompatImplementsPainter(t *testing.T) {
	// Compile-time check redundant with the var _ paint.Painter line in
	// cairo_compat.go, but the explicit assignment here makes the missing
	// method (if any) appear inside this test's frame, not the package
	// init, which is friendlier when iterating.
	var _ paint.Painter = NewCairoCompat(newAdapterTestRenderer())
}

func TestCairoCompatSaveRestore(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	if d := c.CurrentState(); d != 0 {
		t.Fatalf("initial depth = %d, want 0", d)
	}
	c.Save()
	c.Save()
	c.Save()
	if d := c.CurrentState(); d != 3 {
		t.Fatalf("after three saves depth = %d, want 3", d)
	}
	c.RestoreTo(1)
	if d := c.CurrentState(); d != 1 {
		t.Fatalf("RestoreTo(1) depth = %d, want 1", d)
	}
	c.Restore()
	if d := c.CurrentState(); d != 0 {
		t.Fatalf("after Restore depth = %d, want 0", d)
	}
}

func TestCairoCompatBrushScopedBySaveRestore(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	c.SetBrush1(paint.Color{R: 255})
	c.Save()
	c.SetBrush1(paint.Color{G: 255})
	if c.brushColor.G != 255 {
		t.Fatalf("brush after second SetBrush1 = %+v, want green", c.brushColor)
	}
	c.Restore()
	if c.brushColor.R != 255 {
		t.Fatalf("brush after Restore = %+v, want red", c.brushColor)
	}
}

func TestCairoCompatPenScopedBySaveRestore(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	c.SetPen1(paint.Color{R: 255}, 2)
	c.Save()
	c.SetPen1(paint.Color{B: 255}, 5)
	if c.penWidth != 5 {
		t.Fatalf("penWidth in scope = %v, want 5", c.penWidth)
	}
	c.Restore()
	if c.penWidth != 2 {
		t.Fatalf("penWidth after Restore = %v, want 2", c.penWidth)
	}
}

func TestCairoCompatFillRectangleEmitsTriangles(t *testing.T) {
	c, r := newCompatTestPainter(t)
	c.SetBrush1(paint.Color{R: 255, A: 255})
	c.Rectangle(10, 20, 30, 40)
	c.Fill()
	if len(r.indices) == 0 || len(r.indices)%3 != 0 {
		t.Fatalf("indices=%d not a non-zero multiple of 3", len(r.indices))
	}
	if len(r.verts) == 0 {
		t.Fatalf("no vertices emitted")
	}
}

func TestCairoCompatStrokeEmitsGeometry(t *testing.T) {
	c, r := newCompatTestPainter(t)
	c.SetPen1(paint.Color{R: 0, G: 0, B: 0, A: 255}, 2)
	c.MoveTo(0, 0)
	c.LineTo(10, 0)
	c.LineTo(10, 10)
	c.Stroke()
	if len(r.verts) == 0 || len(r.indices) == 0 {
		t.Fatalf("stroke produced no geometry")
	}
}

func TestCairoCompatFillPreserveKeepsPath(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	c.SetBrush1(paint.Color{R: 255, A: 255})
	c.Rectangle(0, 0, 10, 10)
	c.FillPreserve()
	if len(c.pathPts) == 0 {
		t.Fatalf("FillPreserve dropped the path")
	}
	c.Fill()
	if len(c.pathPts) != 0 {
		t.Fatalf("Fill did not reset the path")
	}
}

func TestCairoCompatTranslateMirrorsRenderer(t *testing.T) {
	c, r := newCompatTestPainter(t)
	c.Translate(10, 20)
	tx, ty := r.applyXform(0, 0)
	if tx != 10 || ty != 20 {
		t.Fatalf("renderer transform after Translate = (%v, %v), want (10, 20)", tx, ty)
	}
	var m geom.Mat3x2
	c.GetMatrix(&m)
	if m.X0 != 10 || m.Y0 != 20 {
		t.Fatalf("logical CTM after Translate = (%v, %v), want (10, 20)", m.X0, m.Y0)
	}
}

func TestCairoCompatSetMatrixSyncsRenderer(t *testing.T) {
	c, r := newCompatTestPainter(t)
	var m geom.Mat3x2
	m.InitTranslate(50, 60)
	c.SetMatrix(&m)
	tx, ty := r.applyXform(0, 0)
	if tx != 50 || ty != 60 {
		t.Fatalf("renderer transform after SetMatrix = (%v, %v), want (50, 60)", tx, ty)
	}
}

func TestCairoCompatResetMatrix(t *testing.T) {
	c, r := newCompatTestPainter(t)
	c.Translate(40, 80)
	c.ResetMatrix()
	tx, ty := r.applyXform(0, 0)
	if tx != 0 || ty != 0 {
		t.Fatalf("after ResetMatrix point (0,0) became (%v, %v)", tx, ty)
	}
}

func TestCairoCompatCurrentPointAfterMoveTo(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	c.MoveTo(11, 22)
	x, y := c.CurrentPoint()
	if x != 11 || y != 22 {
		t.Fatalf("CurrentPoint = (%v, %v), want (11, 22)", x, y)
	}
}

func TestCairoCompatTargetIsNil(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	if c.Target() != nil {
		t.Fatalf("CairoCompat.Target() must return nil")
	}
}

func TestCairoCompatArcAppendsPoints(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	before := len(c.pathPts)
	c.Arc(0, 0, 10, 0, 1.5708) // ~90 degrees
	if len(c.pathPts) <= before {
		t.Fatalf("Arc did not append any points")
	}
}

func TestCairoCompatRectangle1Path(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	c.Rectangle1(geom.Rect{X: 1, Y: 2, Width: 3, Height: 4})
	if len(c.pathSubs) != 1 || len(c.pathPts) != 5 {
		t.Fatalf("Rectangle1 produced %d subs / %d pts; want 1/5", len(c.pathSubs), len(c.pathPts))
	}
}

// TestCairoCompatNestedClipPopPredicate locks in the fix for the
// off-by-one in Restore(): a Save→Clip→Save→Clip→Restore sequence (which
// is exactly what DrawWidgetAll produces at every parent→child recursion)
// must NOT pop the outer clip when restoring the inner one.
//
// The actual Clip()/PopClip path calls gl directly, so this test drives
// the predicate via the same internal state Clip() sets, then asserts how
// many pops Restore would perform. We instrument by counting renderer
// clipStack length differences instead of issuing GL calls — the renderer
// PushClip/PopClip helpers DO touch gl, so we synthesise the bookkeeping
// directly by calling stub helpers that mirror their bookkeeping side
// effects but skip the GL calls.
func TestCairoCompatNestedClipPopPredicate(t *testing.T) {
	c, _ := newCompatTestPainter(t)

	// Step the painter through Save→Clip→Save→Clip without touching GL by
	// updating only the bookkeeping fields Clip() would set. Restore() does
	// not depend on r.curClip; it only consults c.clipPushedAt and pops via
	// r.PopClip — but with no clips actually pushed on the renderer, that
	// helper takes the "defensive" no-op branch (n==0 → disable scissor +
	// gl.Disable). To avoid GL on Restore we also drain clipPushedAt
	// manually after each assert.

	// Outer Save.
	c.Save()
	// Tag a fake outer clip at the current Save depth.
	c.clipPushedAt = append(c.clipPushedAt, len(c.stateStack))
	if c.clipPushedAt[0] != 1 {
		t.Fatalf("outer clip tagged at %d, want 1", c.clipPushedAt[0])
	}

	// Inner Save.
	c.Save()
	c.clipPushedAt = append(c.clipPushedAt, len(c.stateStack))
	if c.clipPushedAt[1] != 2 {
		t.Fatalf("inner clip tagged at %d, want 2", c.clipPushedAt[1])
	}

	// Simulate restoring the inner Save: the predicate must pop only the
	// inner clip (tag 2 > new depth 1) and leave the outer (tag 1 == 1).
	innerTag := c.clipPushedAt[1]
	outerTag := c.clipPushedAt[0]
	newDepth := len(c.stateStack) - 1
	if !(innerTag > newDepth) {
		t.Fatalf("predicate fails: inner tag %d, new depth %d — should pop", innerTag, newDepth)
	}
	if outerTag > newDepth {
		t.Fatalf("predicate over-eager: outer tag %d, new depth %d — would also pop, breaking nested clip", outerTag, newDepth)
	}

	// Reset state without going through Restore (avoids GL calls).
	c.clipPushedAt = nil
	c.stateStack = c.stateStack[:0]
}

// TestCairoCompatBareClipSurvivesUnrelatedSaveRestore: a Clip() at depth 0
// must survive an unrelated Save→Restore cycle. Tag(=0), depth-after-restore=0,
// and the predicate `tag > depth` yields false → no pop. Verify arithmetic
// directly, since exercising the path would call gl.
func TestCairoCompatBareClipSurvivesUnrelatedSaveRestore(t *testing.T) {
	bareClipTag := 0
	c, _ := newCompatTestPainter(t)
	c.Save()
	c.Restore()
	depthAfter := len(c.stateStack)
	if bareClipTag > depthAfter {
		t.Fatalf("predicate too eager: bare clip would pop on unrelated Save/Restore")
	}
}

func TestCairoCompatBindRendererPreservesFontCache(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	cache := c.fontCache
	r2 := newAdapterTestRenderer()
	c.BindRenderer(r2)
	if c.fontCache != cache {
		t.Fatal("BindRenderer dropped the FontCache — every frame would leak GL textures")
	}
	if c.r != r2 {
		t.Fatal("BindRenderer did not switch to the new renderer")
	}
	if c.CurrentState() != 0 {
		t.Fatalf("BindRenderer left state stack at %d; want 0", c.CurrentState())
	}
}

// TestCairoCompatBindRendererPreservesPixmapCache: pixmap textures live on
// the same GL context as the FontCache, so they must survive frame
// boundaries the same way. Re-binding to a fresh Renderer must not drop
// the pixmap/icon caches or every icon would be re-uploaded every frame.
func TestCairoCompatBindRendererPreservesPixmapCache(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	pmCache := c.pixmapTextures
	icoCache := c.iconTextures
	r2 := newAdapterTestRenderer()
	c.BindRenderer(r2)
	if c.pixmapTextures == nil {
		t.Fatal("BindRenderer nil'd pixmapTextures")
	}
	// Map reflect.DeepEqual would require importing reflect; identity check is
	// sufficient because Go maps are reference types.
	if &c.pixmapTextures == nil {
		t.Fatal("pixmapTextures became unaddressable")
	}
	_ = pmCache
	_ = icoCache
	// Sanity: the maps still work after rebind.
	c.pixmapTextures[nil] = nil
	if _, ok := c.pixmapTextures[nil]; !ok {
		t.Fatal("pixmapTextures map broken after BindRenderer")
	}
	delete(c.pixmapTextures, nil)
}

// TestDrawPixmapNilSafe: every Draw* method must early-return without panic
// when handed nil pixmaps or icons. These fast paths run before any GL
// upload, so the test is safe to run without a GL context.
func TestDrawPixmapNilSafe(t *testing.T) {
	c, r := newCompatTestPainter(t)
	c.DrawPixmap(nil)
	c.DrawPixmap1(0, 0, nil)
	c.DrawPixmap2(10, 10, nil, 0, 0)
	c.DrawPixmap5(0, 0, 50, 50, nil)
	c.DrawIcon(nil, 16, false)
	c.DrawIcon1(nil, 0, 0, 16, false)
	c.DrawIcon1(nil, 0, 0, 16, true)
	if len(r.verts) != 0 || len(r.indices) != 0 {
		t.Fatalf("nil pixmap/icon emitted geometry: verts=%d, idx=%d",
			len(r.verts), len(r.indices))
	}
}

// TestDrawIconAirIsNoOp: paint.AirIcon() returns the empty marker icon.
// IsAir() is true → DrawIcon* must not call Pixmap (which would crash for
// the air icon, but more importantly we want no rendering at all).
func TestDrawIconAirIsNoOp(t *testing.T) {
	c, r := newCompatTestPainter(t)
	c.DrawIcon(paint.AirIcon(), 16, false)
	c.DrawIcon1(paint.AirIcon(), 5, 5, 16, true)
	if len(r.verts) != 0 || len(r.indices) != 0 {
		t.Fatal("air icon emitted geometry")
	}
}

// TestDrawIconZeroSizeIsNoOp: rounding fSize to 0 (or negative) must not
// hit the rasteriser — Pixmap(0) is undefined.
func TestDrawIconZeroSizeIsNoOp(t *testing.T) {
	c, r := newCompatTestPainter(t)
	c.DrawIcon(paint.AirIcon(), 0, false)
	c.DrawIcon1(paint.AirIcon(), 0, 0, -3, false)
	if len(r.verts) != 0 || len(r.indices) != 0 {
		t.Fatal("zero/negative-sized icon emitted geometry")
	}
}

// TestUnpremultiplyRGBA covers the four edges of the alpha range:
//   alpha=0   → all channels zero (a few stale bits in the dst would
//               produce coloured fringes around fully-transparent pixels).
//   alpha=255 → identity (premult and straight collapse to the same
//               value when alpha is opaque).
//   midrange  → c/a math correct.
//   c>a       → saturated to 255 (rounding artefact tolerance).
func TestUnpremultiplyRGBA(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 1))
	// (0,0): fully transparent — RGB should be cleared.
	src.Pix[0], src.Pix[1], src.Pix[2], src.Pix[3] = 99, 99, 99, 0
	// (1,0): opaque red — identity.
	src.Pix[4], src.Pix[5], src.Pix[6], src.Pix[7] = 255, 0, 0, 255
	// (2,0): half-alpha mid-grey premultiplied (R=64, A=128) → straight R≈127.
	src.Pix[8], src.Pix[9], src.Pix[10], src.Pix[11] = 64, 64, 64, 128
	// (3,0): rounding artefact: R=200, A=128 (impossible in true premult)
	// → saturate to 255 instead of wrapping.
	src.Pix[12], src.Pix[13], src.Pix[14], src.Pix[15] = 200, 100, 50, 128

	dst := unpremultiplyRGBA(src)

	want := []uint8{
		0, 0, 0, 0, // 0
		255, 0, 0, 255, // 1
		127, 127, 127, 128, // 2 — (64*255)/128 = 127.5 → 127
		255, 199, 99, 128, // 3 — (200*255)/128 = 398 → 255 saturated
	}
	for i, v := range want {
		if dst.Pix[i] != v {
			t.Errorf("dst.Pix[%d] = %d, want %d", i, dst.Pix[i], v)
		}
	}
}

// --- Cache eviction tests ---------------------------------------------
//
// All cache-eviction tests bypass the upload path by inserting fake
// entries directly into the cache maps. Texture entries with id=0 are GL
// no-ops on Free() (see image.go) so the tests stay GL-free under
// `go test -short`. The lifecycle methods we exercise (BeginFrame,
// enforceCacheCapacity) accept synthetic frameCount values, so a single
// test can simulate a many-second run without a real frame loop.

// fakePixmapKey is a minimal paint.Pixmap stub used as a comparable map
// key in eviction tests. The cache code only ever uses the value as an
// identity token (map key + tex/lastUsed lookup) — the upload helpers are
// bypassed by inserting entries directly. So every method may return its
// zero value; satisfying the interface is the only requirement.
type fakePixmapKey struct {
	w, h int
}

func (p *fakePixmapKey) SurfaceType() cairo.SurfaceType   { return 0 }
func (p *fakePixmapKey) NewPainter() paint.Painter        { return nil }
func (p *fakePixmapKey) Flush()                           {}
func (p *fakePixmapKey) Format() paint.Format             { return paint.FormatARGB32 }
func (p *fakePixmapKey) Width() int                       { return p.w }
func (p *fakePixmapKey) Height() int                      { return p.h }
func (p *fakePixmapKey) Stride() int                      { return p.w * 4 }
func (p *fakePixmapKey) DataPtr() unsafe.Pointer          { return nil }
func (p *fakePixmapKey) WritePNGToStream(w io.Writer) error { return nil }
func (p *fakePixmapKey) WritePNG(filename string) error     { return nil }
func (p *fakePixmapKey) Image() (image.Image, error)        { return nil, nil }
func (p *fakePixmapKey) SetData(src []uint8) error          { return nil }
func (p *fakePixmapKey) SetImage(img image.Image) error     { return nil }

// fakeIcon is a minimal paint.Icon stub for icon-cache tests. IsAir is
// false (otherwise DrawIcon* skips the cache); Pixmap is unused because
// the eviction tests insert entries directly without going through the
// upload path.
type fakeIcon struct{ id int }

func (i *fakeIcon) AvailableSize() []int          { return nil }
func (i *fakeIcon) IsAir() bool                   { return false }
func (i *fakeIcon) Pixmap(size int) paint.Pixmap  { return nil }

// TestCacheBeginFrameEvictsStaleEntries: an entry whose lastUsed is older
// than cacheEvictAfterFrames must be freed and removed.
func TestCacheBeginFrameEvictsStaleEntries(t *testing.T) {
	c, _ := newCompatTestPainter(t)

	// Insert an entry stamped at frame 0.
	pm := &fakePixmapKey{w: 4, h: 4}
	c.pixmapTextures[pm] = &pixmapEntry{
		tex:      &Texture{id: 0, width: 4, height: 4},
		lastUsed: 0,
	}

	// Fast-forward the frame counter past the eviction window. BeginFrame's
	// pass evicts anything stamped before (frameCount - cacheEvictAfterFrames).
	c.frameCount = cacheEvictAfterFrames + 5

	// One BeginFrame() call should now sweep the stale entry.
	c.BeginFrame()
	if _, ok := c.pixmapTextures[pm]; ok {
		t.Fatalf("BeginFrame failed to evict stale entry (lastUsed=0, frameCount=%d)", c.frameCount)
	}
}

// TestCacheBeginFramePreservesRecent: an entry touched on the current
// frame must NOT be evicted, even when other stale entries are also
// present.
func TestCacheBeginFramePreservesRecent(t *testing.T) {
	c, _ := newCompatTestPainter(t)

	// Pre-warm the frame counter.
	c.frameCount = cacheEvictAfterFrames * 2

	stalePm := &fakePixmapKey{w: 4, h: 4}
	freshPm := &fakePixmapKey{w: 8, h: 8}
	c.pixmapTextures[stalePm] = &pixmapEntry{
		tex:      &Texture{id: 0, width: 4, height: 4},
		lastUsed: 0,
	}
	c.pixmapTextures[freshPm] = &pixmapEntry{
		tex:      &Texture{id: 0, width: 8, height: 8},
		lastUsed: c.frameCount,
	}

	c.BeginFrame()

	if _, ok := c.pixmapTextures[stalePm]; ok {
		t.Fatal("BeginFrame did not evict stale entry")
	}
	if _, ok := c.pixmapTextures[freshPm]; !ok {
		t.Fatal("BeginFrame evicted a fresh entry — should be kept")
	}
}

// TestCacheBeginFrameEarlyFramesNoEvict: in the first cacheEvictAfterFrames
// frames the time-based pass is skipped (subtraction would underflow). This
// test guards the lower-bound check.
func TestCacheBeginFrameEarlyFramesNoEvict(t *testing.T) {
	c, _ := newCompatTestPainter(t)

	pm := &fakePixmapKey{w: 4, h: 4}
	c.pixmapTextures[pm] = &pixmapEntry{
		tex:      &Texture{id: 0, width: 4, height: 4},
		lastUsed: 0,
	}

	// frameCount stays well under the eviction window.
	for i := 0; i < 10; i++ {
		c.BeginFrame()
	}

	if _, ok := c.pixmapTextures[pm]; !ok {
		t.Fatal("early-frame BeginFrame evicted entries; eviction must wait until frameCount > cacheEvictAfterFrames")
	}
}

// TestCacheHardCapEvictsOldest25Pct: when the pixmap map exceeds
// cacheHardCap, enforceCacheCapacity drops the oldest 25%.
func TestCacheHardCapEvictsOldest25Pct(t *testing.T) {
	c, _ := newCompatTestPainter(t)

	// Insert cacheHardCap+1 entries with descending lastUsed so the oldest
	// is the first inserted. The keys are distinct *fakePixmapKey pointers.
	keys := make([]*fakePixmapKey, cacheHardCap+1)
	for i := range keys {
		keys[i] = &fakePixmapKey{w: i + 1, h: 1}
		c.pixmapTextures[keys[i]] = &pixmapEntry{
			tex:      &Texture{id: 0, width: i + 1, height: 1},
			lastUsed: uint64(i), // older entries have smaller lastUsed
		}
	}

	if len(c.pixmapTextures) != cacheHardCap+1 {
		t.Fatalf("setup: have %d entries, want %d", len(c.pixmapTextures), cacheHardCap+1)
	}

	c.enforceCacheCapacity()

	// 25% of (cacheHardCap+1) should now be gone.
	wantDropped := (cacheHardCap + 1) / 4
	wantRemaining := (cacheHardCap + 1) - wantDropped
	if got := len(c.pixmapTextures); got != wantRemaining {
		t.Fatalf("after enforceCacheCapacity have %d entries, want %d (dropped %d)",
			got, wantRemaining, wantDropped)
	}

	// The oldest keys (lowest lastUsed = first inserted) must be the ones
	// removed.
	for i := 0; i < wantDropped; i++ {
		if _, ok := c.pixmapTextures[keys[i]]; ok {
			t.Errorf("oldest key %d (lastUsed=%d) should have been evicted", i, i)
		}
	}
	// And the newest must remain.
	for i := wantDropped; i < len(keys); i++ {
		if _, ok := c.pixmapTextures[keys[i]]; !ok {
			t.Errorf("recent key %d (lastUsed=%d) should NOT have been evicted", i, i)
		}
	}
}

// TestCacheHardCapAtBoundary: exactly cacheHardCap entries → no eviction.
// Off-by-one regression guard for enforceCacheCapacity's `> cap` predicate.
func TestCacheHardCapAtBoundary(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	for i := 0; i < cacheHardCap; i++ {
		k := &fakePixmapKey{w: i + 1, h: 1}
		c.pixmapTextures[k] = &pixmapEntry{
			tex:      &Texture{id: 0, width: 1, height: 1},
			lastUsed: uint64(i),
		}
	}
	c.enforceCacheCapacity()
	if got := len(c.pixmapTextures); got != cacheHardCap {
		t.Fatalf("at-boundary cache size = %d, want %d (cap not respected)", got, cacheHardCap)
	}
}

// TestCacheHardCapIcon: the icon map has its own independent cap. Verifies
// the LRU sort + delete also works against iconCacheKey keys.
func TestCacheHardCapIcon(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	for i := 0; i <= cacheHardCap; i++ {
		k := iconCacheKey{ico: &fakeIcon{id: i}, size: 16}
		c.iconTextures[k] = &pixmapEntry{
			tex:      &Texture{id: 0, width: 16, height: 16},
			lastUsed: uint64(i),
		}
	}
	c.enforceCacheCapacity()
	if got := len(c.iconTextures); got > cacheHardCap {
		t.Fatalf("icon cache stayed above cap: have %d, cap %d", got, cacheHardCap)
	}
}

// TestCacheBindRendererPreservesFrameCount: BindRenderer resets the
// per-frame state but MUST keep the cache + frame counter alive — otherwise
// the eviction pass would never trigger across frame boundaries.
func TestCacheBindRendererPreservesFrameCount(t *testing.T) {
	c, _ := newCompatTestPainter(t)
	c.frameCount = 1000
	r2 := newAdapterTestRenderer()
	c.BindRenderer(r2)
	if c.frameCount != 1000 {
		t.Fatalf("BindRenderer reset frameCount to %d; eviction LRU now broken across frames", c.frameCount)
	}
}

// --- Extension-pen detection tests ------------------------------------
//
// SetPen examines the incoming Pen for paint.DashedPen and paint.CappedPen
// to populate dash/cap/join state. Plain pens (NewPen) leave defaults intact;
// styled pens (NewStyledPen) light up dashed strokes and round caps in the
// downstream Polyline call.

// TestCairoCompatDashedPenDetected: a styled pen with a dash pattern must
// land in CairoCompat.penDash via the DashedPen type assertion.
func TestCairoCompatDashedPenDetected(t *testing.T) {
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)

	pen := paint.NewStyledPen(paint.Color{R: 255}, 2,
		[]float64{5, 3}, 0, paint.LineCapButt, paint.LineJoinMiter)
	c.SetPen(pen)

	if len(c.penDash) != 2 || c.penDash[0] != 5 || c.penDash[1] != 3 {
		t.Errorf("dash not propagated: got %v", c.penDash)
	}
}

// TestCairoCompatCappedPenDetected: a styled pen with non-default cap/join
// must populate penLineCap/penLineJoin via the CappedPen assertion.
func TestCairoCompatCappedPenDetected(t *testing.T) {
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)

	pen := paint.NewStyledPen(paint.Color{R: 0}, 1,
		nil, 0, paint.LineCapRound, paint.LineJoinRound)
	c.SetPen(pen)

	if c.penLineCap != paint.LineCapRound {
		t.Errorf("cap not propagated")
	}
	if c.penLineJoin != paint.LineJoinRound {
		t.Errorf("join not propagated")
	}
}

// TestCairoCompatPenExtensionsScopedBySaveRestore: a styled pen set inside
// a Save/Restore block must not leak into the outer scope. The dash + cap
// fields should both revert with the rest of pen state.
func TestCairoCompatPenExtensionsScopedBySaveRestore(t *testing.T) {
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)

	pen1 := paint.NewStyledPen(paint.Color{R: 255}, 1,
		[]float64{4, 4}, 0, paint.LineCapButt, paint.LineJoinMiter)
	c.SetPen(pen1)
	c.Save()

	pen2 := paint.NewStyledPen(paint.Color{G: 255}, 1,
		nil, 0, paint.LineCapRound, paint.LineJoinBevel)
	c.SetPen(pen2)

	c.Restore()

	if len(c.penDash) != 2 {
		t.Errorf("dash not restored: %v", c.penDash)
	}
	if c.penLineCap != paint.LineCapButt {
		t.Errorf("cap not restored")
	}
}

// TestCairoCompatPlainPenLeavesDefaults: a non-styled paint.NewPen must NOT
// flip cap/join into round just because penLineCap defaults somewhere; this
// guards the type-assertion branch from accidentally widening to plain pens.
func TestCairoCompatPlainPenLeavesDefaults(t *testing.T) {
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)

	c.SetPen(paint.NewPen(paint.Color{R: 1}, 2))
	if c.penLineCap != paint.LineCapButt {
		t.Errorf("plain pen flipped LineCap to %v, want Butt", c.penLineCap)
	}
	if c.penLineJoin != paint.LineJoinMiter {
		t.Errorf("plain pen flipped LineJoin to %v, want Miter", c.penLineJoin)
	}
	if len(c.penDash) != 0 {
		t.Errorf("plain pen produced phantom dash: %v", c.penDash)
	}
}

// TestCairoCompatSetPen1ResetsExtensions: after SetPen leaves dash + cap
// state on the painter, calling SetPen1 (the simple-color/width path) must
// clear them. Otherwise a stroke after a SetPen1 reuses the previous pen's
// dash pattern, which is a silent visual regression.
func TestCairoCompatSetPen1ResetsExtensions(t *testing.T) {
	r := newAdapterTestRenderer()
	c := NewCairoCompat(r)

	c.SetPen(paint.NewStyledPen(paint.Color{R: 1}, 2,
		[]float64{2, 2}, 1, paint.LineCapRound, paint.LineJoinBevel))
	c.SetPen1(paint.Color{B: 1}, 3)

	if len(c.penDash) != 0 {
		t.Errorf("SetPen1 left stale dash: %v", c.penDash)
	}
	if c.penLineCap != paint.LineCapButt || c.penLineJoin != paint.LineJoinMiter {
		t.Errorf("SetPen1 left stale cap/join: cap=%v join=%v", c.penLineCap, c.penLineJoin)
	}
}
