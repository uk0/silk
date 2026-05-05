package glui

import (
	"image"
	"math"
	"sort"
	"unsafe"

	"silk/geom"
	"silk/glui/path"
	"silk/paint"
)

// CairoCompat implements paint.Painter on top of a glui.Renderer so the
// existing widget framework — which calls paint.Painter methods inside
// every Draw() — can render through the GPU pipeline without any widget-
// side changes.
//
// Architecture note (vs PainterAdapter): the package-level invariant
// captured in painter_adapter.go ("glui must not import paint+cairo to
// stay pure-OpenGL") is intentionally bent here. Widget compatibility is
// the whole point of this file — we cannot satisfy paint.Painter without
// importing it, and paint transitively pulls in silk/cairo. The existing
// pure-glui PainterAdapter still exists for native glui code; CairoCompat
// is the bridge for the legacy widget set.
//
// What is approximated rather than exactly emulated:
//   - Pattern brushes / gradients (only solid color is honoured)
//   - SetOperator / blend modes (we always run SRC_OVER)
//   - Clip-by-path (only axis-aligned bounding-box clip is supported —
//     proper stencil-buffer path clipping is a TODO)
//   - Cairo glyph IDs in DrawGlyphs/DrawGlyph (no-op; widgets that route
//     text through DrawGlyphs directly will render blank — fortunately
//     the standard widget set goes through DrawText/DrawText1)
//   - Grayed icons use a fixed RGB×0.6, A×0.7 tint instead of the Cairo
//     HSL_LUMINOSITY operator. Visually close, not pixel-equivalent.
//
// State management mirrors paint.cairoPainter: brush + pen + font are
// pushed/popped together with the renderer transform on every Save().
type CairoCompat struct {
	r *Renderer

	// Current path being built up by MoveTo / LineTo / Arc / Rectangle.
	pathPts  [][2]float32
	pathSubs []int
	curX     float32
	curY     float32

	// Pen / brush / font state. brushColor mirrors the SOURCE colour the
	// Cairo painter resolves; pen.color/pen.width is the stroke style.
	penColor   paint.Color
	penWidth   float32
	brushColor paint.Color
	font       paint.Font

	// Logical CTM mirrored from the renderer. We need read access to it
	// for GetMatrix() and to resolve hairline pens (width=0). Renderer.xform
	// already stores the identical matrix — we keep our own float64 copy
	// because the paint.Painter interface speaks float64 + Mat3x2.
	ctm    geom.Mat3x2
	ctmIdx int // future: used to invalidate cached glui.Font lookups

	// Save/Restore stack stores brush+pen+font+path snapshot. The renderer
	// matrix stack is pushed independently in r.Save(), so RestoreTo() is
	// safe.
	stateStack []cairoCompatState

	// Active clip rectangle in logical coordinates (the bounding box of
	// the most recent Clip()). Updated as Save/Restore pop. Empty rect
	// when no clip is active.
	clipRect      geom.Rect
	clipStack     []geom.Rect
	clipDepth     int
	clipPushedAt  []int // matched against len(stateStack) so Restore can pop

	// Default font to use when SetFont(nil) — only resolved on demand so
	// we don't pay the rasteriser cost upfront.
	fontCache *FontCache

	// pixmapTextures caches GL textures uploaded for paint.Pixmap values.
	// Keyed by interface identity (paint.Pixmap concrete type *cairoSurface
	// is comparable). First DrawPixmap* call uploads + caches; later calls
	// reuse the same GL texture so we don't re-upload every frame.
	//
	// Lives on CairoCompat (not Renderer) so it survives BindRenderer and
	// the corresponding GL context is the same Window that created glui.Context.
	//
	// Each entry tracks its last-used frame so BeginFrame can evict stale
	// textures (e.g. theme-change icons that the new theme no longer paints).
	pixmapTextures map[paint.Pixmap]*pixmapEntry

	// iconTextures caches per-(icon, size) renders. icon.Pixmap(size)
	// allocates a fresh *cairoSurface every call, so a pixmap-pointer cache
	// alone would miss every frame and leak GL textures unboundedly. We
	// short-circuit at the icon layer instead — same icon at the same size
	// always maps to the same texture.
	iconTextures map[iconCacheKey]*pixmapEntry

	// frameCount is incremented once per BeginFrame() and stamps the lastUsed
	// field on cache hits + misses. It survives BindRenderer because GL
	// textures live on the same context across frames.
	frameCount uint64
}

// pixmapEntry pairs a cached GL texture with the last frame on which it was
// touched. Used by BeginFrame's eviction pass to free stale uploads.
type pixmapEntry struct {
	tex      *Texture
	lastUsed uint64
}

// iconCacheKey is comparable: paint.Icon is an interface whose concrete type
// (*icon, *airIcon) is a pointer — pointers are valid map keys.
type iconCacheKey struct {
	ico  paint.Icon
	size int
}

type cairoCompatState struct {
	pathLen    int
	subsLen    int
	penColor   paint.Color
	penWidth   float32
	brushColor paint.Color
	font       paint.Font
	ctm        geom.Mat3x2
	clipRect   geom.Rect
}

// Compile-time interface satisfaction. If a paint.Painter method goes
// missing on CairoCompat the compiler points to it instantly.
var _ paint.Painter = (*CairoCompat)(nil)

// NewCairoCompat wraps a Renderer with a paint.Painter facade. The caller
// retains ownership of r — Begin/End around the frame is its job, not
// CairoCompat's.
func NewCairoCompat(r *Renderer) *CairoCompat {
	c := &CairoCompat{
		r:              r,
		penWidth:       1,
		penColor:       paint.Color{R: 0, G: 0, B: 0, A: 255},
		brushColor:     paint.Color{R: 0, G: 0, B: 0, A: 255},
		fontCache:      NewFontCache(),
		pixmapTextures: make(map[paint.Pixmap]*pixmapEntry),
		iconTextures:   make(map[iconCacheKey]*pixmapEntry),
	}
	c.ctm.InitIdentity()
	return c
}

// BindRenderer attaches the painter to a fresh per-frame Renderer while
// keeping its FontCache (and the GL textures behind it) alive. State that
// only makes sense inside a single Begin/End — current path, save stack,
// active clip — is reset.
//
// Hosts should call this once per frame after Context.Begin() instead of
// allocating a fresh CairoCompat: the FontCache owns persistent GL
// textures that would otherwise leak.
func (c *CairoCompat) BindRenderer(r *Renderer) {
	c.r = r
	c.pathPts = c.pathPts[:0]
	c.pathSubs = c.pathSubs[:0]
	c.curX, c.curY = 0, 0
	c.stateStack = c.stateStack[:0]
	c.clipRect = geom.Rect{}
	c.clipStack = c.clipStack[:0]
	c.clipPushedAt = c.clipPushedAt[:0]
	c.ctm.InitIdentity()
	// Pen/brush/font are intentionally retained — Cairo does the same
	// across surfaces, and most widgets re-set them at the top of Draw().
}

// Target returns the painter's underlying surface. CairoCompat has no
// Cairo surface — widgets never read this in the standard set, so a nil
// is safe and avoids inventing a fake.
func (c *CairoCompat) Target() paint.Surface { return nil }

// --- Cache lifecycle --------------------------------------------------

// cacheEvictAfterFrames is the LRU threshold used by BeginFrame: an entry
// last touched more than this many frames ago is freed. At ~60 FPS this is
// roughly 5 seconds of inactivity, which comfortably keeps theme-change
// icons alive across a transient redraw flurry but reclaims memory before a
// long-running designer session balloons.
const cacheEvictAfterFrames uint64 = 60 * 5

// cacheHardCap caps each map (pixmaps, icons) independently. Widget-heavy
// scenes can pile up a few hundred unique pixmaps; 256 keeps the cache
// useful while preventing pathological growth from e.g. a screen-recording
// widget that uploads a fresh frame on every paint. Per-map (not combined)
// because the two caches have very different access patterns and conflating
// them would let an icon flood evict useful pixmaps and vice versa.
const cacheHardCap = 256

// BeginFrame advances the cache LRU clock and evicts stale entries. Hosts
// MUST call this once per frame before any DrawWidgetAll / Draw* call so
// the lastUsed stamps inside the upload helpers track frame-relative time.
//
// Eviction policy:
//   - Entries last touched more than cacheEvictAfterFrames ago are freed
//     immediately. The GL texture is released via Texture.Free() so the
//     driver can reclaim VRAM.
//   - When either map exceeds cacheHardCap after the time-based pass, the
//     oldest 25% are freed (LRU). This is a safety valve, not the primary
//     mechanism — most workloads should see only the time-based pass fire.
//
// BeginFrame must run on the same GL context that uploaded the textures —
// gl.DeleteTextures is context-affine. Window.paintGlui() satisfies this by
// calling MakeContextCurrent before BeginFrame.
func (c *CairoCompat) BeginFrame() {
	c.frameCount++

	// Time-based eviction. A subtle wraparound case: frameCount started at
	// zero, so on early frames (frameCount < cacheEvictAfterFrames) the
	// subtraction would underflow uint64. Skip the pass entirely until we
	// have enough history — the hard cap still protects against bursty
	// uploads during startup.
	if c.frameCount > cacheEvictAfterFrames {
		threshold := c.frameCount - cacheEvictAfterFrames
		for k, e := range c.pixmapTextures {
			if e.lastUsed < threshold {
				e.tex.Free()
				delete(c.pixmapTextures, k)
			}
		}
		for k, e := range c.iconTextures {
			if e.lastUsed < threshold {
				e.tex.Free()
				delete(c.iconTextures, k)
			}
		}
	}

	c.enforceCacheCapacity()
}

// enforceCacheCapacity is the LRU safety valve. Called from upload paths
// (so the cap holds even when BeginFrame is not yet wired up) and from
// BeginFrame's eviction tail. When either map is over cap, drops the oldest
// 25% by lastUsed.
//
// The 25% bulk-evict is intentional: trimming one entry at a time would
// thrash the cache when the workload is genuinely above the cap. Burning
// the next 25% gives the system breathing room before the next eviction.
func (c *CairoCompat) enforceCacheCapacity() {
	if len(c.pixmapTextures) > cacheHardCap {
		c.evictOldestPixmaps(len(c.pixmapTextures) / 4)
	}
	if len(c.iconTextures) > cacheHardCap {
		c.evictOldestIcons(len(c.iconTextures) / 4)
	}
}

// evictOldestPixmaps drops the n least-recently-used entries from the
// pixmap cache. n <= 0 is a no-op.
func (c *CairoCompat) evictOldestPixmaps(n int) {
	if n <= 0 || len(c.pixmapTextures) == 0 {
		return
	}
	type kv struct {
		k paint.Pixmap
		u uint64
	}
	all := make([]kv, 0, len(c.pixmapTextures))
	for k, e := range c.pixmapTextures {
		all = append(all, kv{k, e.lastUsed})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].u < all[j].u })
	if n > len(all) {
		n = len(all)
	}
	for i := 0; i < n; i++ {
		e := c.pixmapTextures[all[i].k]
		e.tex.Free()
		delete(c.pixmapTextures, all[i].k)
	}
}

// evictOldestIcons drops the n least-recently-used entries from the icon
// cache. n <= 0 is a no-op.
func (c *CairoCompat) evictOldestIcons(n int) {
	if n <= 0 || len(c.iconTextures) == 0 {
		return
	}
	type kv struct {
		k iconCacheKey
		u uint64
	}
	all := make([]kv, 0, len(c.iconTextures))
	for k, e := range c.iconTextures {
		all = append(all, kv{k, e.lastUsed})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].u < all[j].u })
	if n > len(all) {
		n = len(all)
	}
	for i := 0; i < n; i++ {
		e := c.iconTextures[all[i].k]
		e.tex.Free()
		delete(c.iconTextures, all[i].k)
	}
}

// --- State management -------------------------------------------------

func (c *CairoCompat) Save() int {
	depth := len(c.stateStack)
	c.stateStack = append(c.stateStack, cairoCompatState{
		pathLen:    len(c.pathPts),
		subsLen:    len(c.pathSubs),
		penColor:   c.penColor,
		penWidth:   c.penWidth,
		brushColor: c.brushColor,
		font:       c.font,
		ctm:        c.ctm,
		clipRect:   c.clipRect,
	})
	c.r.Save()
	return depth
}

func (c *CairoCompat) Restore() int {
	n := len(c.stateStack)
	if n == 0 {
		return 0
	}
	s := c.stateStack[n-1]
	c.stateStack = c.stateStack[:n-1]

	// Truncate the path to its pre-Save length (Cairo behavior).
	if s.pathLen <= len(c.pathPts) {
		c.pathPts = c.pathPts[:s.pathLen]
	}
	if s.subsLen <= len(c.pathSubs) {
		c.pathSubs = c.pathSubs[:s.subsLen]
	}

	c.penColor = s.penColor
	c.penWidth = s.penWidth
	c.brushColor = s.brushColor
	c.font = s.font
	c.ctm = s.ctm
	// Pop clips pushed *deeper* than the new stack depth. clipPushedAt is
	// tagged with the Save depth in effect at Clip-time; a clip at depth K
	// belongs to the Save scope at depth K, so it must pop when we Restore
	// back to a depth strictly less than K.
	//
	// Critical: use strict >, not >=. A naive `>= len+1` predicate (which
	// looks equivalent at first glance) pops *every* clip on the first
	// Restore in a Save→Clip→Save→Clip nesting because, after the inner
	// pop, len(stateStack) drops to the outer scope's depth and the outer
	// clip's tag matches, popping it prematurely.
	for len(c.clipPushedAt) > 0 && c.clipPushedAt[len(c.clipPushedAt)-1] > len(c.stateStack) {
		c.r.PopClip()
		c.clipPushedAt = c.clipPushedAt[:len(c.clipPushedAt)-1]
	}
	c.clipRect = s.clipRect
	c.r.Restore()
	return len(c.stateStack)
}

func (c *CairoCompat) RestoreTo(target int) bool {
	if target < 0 || target > len(c.stateStack) {
		return false
	}
	for len(c.stateStack) > target {
		c.Restore()
	}
	return true
}

func (c *CairoCompat) CurrentState() int { return len(c.stateStack) }

// --- Transforms -------------------------------------------------------

func (c *CairoCompat) Translate(tx, ty float64) {
	c.ctm.Translate(tx, ty)
	c.r.Translate(float32(tx), float32(ty))
}

func (c *CairoCompat) Scale(sx, sy float64) {
	c.ctm.Scale(sx, sy)
	c.r.Scale(float32(sx), float32(sy))
}

func (c *CairoCompat) Rotate(radians float64) {
	c.ctm.Rotate(radians)
	c.r.Rotate(float32(radians))
}

// ResetMatrix clears the modelview transform back to identity. Renderer's
// xform stack is unaffected; only the top of stack is reset, matching the
// Cairo semantics where Save() pushes the post-Reset identity onto the
// stack the next time the caller saves.
func (c *CairoCompat) ResetMatrix() {
	c.ctm.InitIdentity()
	c.r.xform = identityMatrix3()
}

// Transform post-multiplies the current matrix by m. Mirrors the Cairo
// semantic where Transform composes m as a *local* transform — the new
// origin is m's image of the old origin.
func (c *CairoCompat) Transform(m *geom.Mat3x2) {
	if m == nil {
		return
	}
	c.ctm.MultiplyWidth(m)
	c.syncRendererFromCTM()
}

// SetMatrix replaces the modelview transform.
func (c *CairoCompat) SetMatrix(m *geom.Mat3x2) {
	if m == nil {
		c.ResetMatrix()
		return
	}
	c.ctm = *m
	c.syncRendererFromCTM()
}

// GetMatrix copies the current modelview transform into m.
func (c *CairoCompat) GetMatrix(m *geom.Mat3x2) {
	if m == nil {
		return
	}
	*m = c.ctm
}

// syncRendererFromCTM rebuilds the renderer's matrix3 from the float64
// CTM. Renderer expects column-major affine layout (a, b, c, d, tx, ty).
//
//	geom.Mat3x2 row layout:  [Xx Xy X0]   matrix3 column-major:  [m0 m3 m6]
//	                          [Yx Yy Y0]                          [m1 m4 m7]
//	                          [ 0  0  1]                          [m2 m5 m8]
//
// Mat3x2.Transform: x' = Xx*x + Xy*y + X0, y' = Yx*x + Yy*y + Y0.
// matrix3.applyXform: tx = m[0]*x + m[3]*y + m[6], ty = m[1]*x + m[4]*y + m[7].
// Mapping: m[0]=Xx, m[1]=Yx, m[3]=Xy, m[4]=Yy, m[6]=X0, m[7]=Y0.
func (c *CairoCompat) syncRendererFromCTM() {
	c.r.xform = matrix3{
		float32(c.ctm.Xx), float32(c.ctm.Yx), 0,
		float32(c.ctm.Xy), float32(c.ctm.Yy), 0,
		float32(c.ctm.X0), float32(c.ctm.Y0), 1,
	}
}

// --- Path construction ------------------------------------------------

func (c *CairoCompat) MoveTo(x, y float64) {
	c.pathSubs = append(c.pathSubs, len(c.pathPts))
	c.pathPts = append(c.pathPts, [2]float32{float32(x), float32(y)})
	c.curX, c.curY = float32(x), float32(y)
}

func (c *CairoCompat) LineTo(x, y float64) {
	if len(c.pathSubs) == 0 {
		c.pathSubs = append(c.pathSubs, len(c.pathPts))
	}
	c.pathPts = append(c.pathPts, [2]float32{float32(x), float32(y)})
	c.curX, c.curY = float32(x), float32(y)
}

// CurveTo flattens a cubic Bezier from CurrentPoint to (x3, y3) into 16
// linear segments. 16 is plenty for typical UI curves — error stays well
// below 1px at usual radii. For high-curvature segments (s-curves at large
// scale) increase the segment count or fall back to true Casteljau.
func (c *CairoCompat) CurveTo(x1, y1, x2, y2, x3, y3 float64) {
	const segs = 16
	x0 := float64(c.curX)
	y0 := float64(c.curY)
	if len(c.pathSubs) == 0 {
		c.pathSubs = append(c.pathSubs, len(c.pathPts))
	}
	for i := 1; i <= segs; i++ {
		t := float64(i) / float64(segs)
		mt := 1 - t
		// B(t) = (1-t)^3 P0 + 3(1-t)^2 t P1 + 3(1-t) t^2 P2 + t^3 P3
		w0 := mt * mt * mt
		w1 := 3 * mt * mt * t
		w2 := 3 * mt * t * t
		w3 := t * t * t
		x := w0*x0 + w1*x1 + w2*x2 + w3*x3
		y := w0*y0 + w1*y1 + w2*y2 + w3*y3
		c.pathPts = append(c.pathPts, [2]float32{float32(x), float32(y)})
	}
	c.curX, c.curY = float32(x3), float32(y3)
}

// Arc flattens a circular arc (CCW from angle1 to angle2) into segments.
// 16 segments per quarter-turn keeps the deviation under ~0.05 points at
// reasonable radii — visually indistinguishable from a true circle.
func (c *CairoCompat) Arc(xc, yc, radius, angle1, angle2 float64) {
	c.appendArc(xc, yc, radius, angle1, angle2, +1)
}

// ArcNegative is Arc but rotates the other way.
func (c *CairoCompat) ArcNegative(xc, yc, radius, angle1, angle2 float64) {
	c.appendArc(xc, yc, radius, angle1, angle2, -1)
}

func (c *CairoCompat) appendArc(xc, yc, radius, a0, a1 float64, sign float64) {
	// Normalise the sweep so it always advances in `sign` direction.
	if sign > 0 {
		for a1 < a0 {
			a1 += 2 * math.Pi
		}
	} else {
		for a1 > a0 {
			a1 -= 2 * math.Pi
		}
	}

	const stepsPerQuarter = 16
	span := math.Abs(a1 - a0)
	steps := int(span/(math.Pi/2)*stepsPerQuarter) + 1
	if steps < 4 {
		steps = 4
	}
	if len(c.pathSubs) == 0 {
		c.pathSubs = append(c.pathSubs, len(c.pathPts))
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		a := a0 + (a1-a0)*t
		x := xc + radius*math.Cos(a)
		y := yc + radius*math.Sin(a)
		c.pathPts = append(c.pathPts, [2]float32{float32(x), float32(y)})
	}
	if len(c.pathPts) > 0 {
		end := c.pathPts[len(c.pathPts)-1]
		c.curX, c.curY = end[0], end[1]
	}
}

func (c *CairoCompat) Rectangle(x, y, w, h float64) {
	xf, yf, wf, hf := float32(x), float32(y), float32(w), float32(h)
	c.pathSubs = append(c.pathSubs, len(c.pathPts))
	c.pathPts = append(c.pathPts,
		[2]float32{xf, yf},
		[2]float32{xf + wf, yf},
		[2]float32{xf + wf, yf + hf},
		[2]float32{xf, yf + hf},
		[2]float32{xf, yf},
	)
	c.curX, c.curY = xf, yf
}

func (c *CairoCompat) Rectangle1(rect geom.Rect) {
	c.Rectangle(rect.X, rect.Y, rect.Width, rect.Height)
}

func (c *CairoCompat) Line(x1, y1, x2, y2 float64) {
	c.MoveTo(x1, y1)
	c.LineTo(x2, y2)
}

func (c *CairoCompat) CurrentPoint() (x, y float64) {
	return float64(c.curX), float64(c.curY)
}

// --- Fill / Stroke ----------------------------------------------------

func (c *CairoCompat) Fill() {
	c.fillCurrentPath()
	c.resetPath()
}

func (c *CairoCompat) FillPreserve() { c.fillCurrentPath() }

func (c *CairoCompat) Stroke() {
	c.strokeCurrentPath()
	c.resetPath()
}

func (c *CairoCompat) StrokePreserve() { c.strokeCurrentPath() }

func (c *CairoCompat) fillCurrentPath() {
	if len(c.pathPts) < 3 {
		return
	}
	col := paintColorToGlui(c.brushColor)
	for i, start := range c.pathSubs {
		end := len(c.pathPts)
		if i+1 < len(c.pathSubs) {
			end = c.pathSubs[i+1]
		}
		// Strip a trailing close-to-first vertex if present — it's a
		// zero-length edge that confuses the ear-clipper.
		if end-start >= 4 {
			first := c.pathPts[start]
			last := c.pathPts[end-1]
			if first == last {
				end--
			}
		}
		if end-start < 3 {
			continue
		}
		sub := c.pathPts[start:end]
		idx := path.Triangulate(sub)
		for k := 0; k+2 < len(idx); k += 3 {
			a := sub[idx[k]]
			b := sub[idx[k+1]]
			cc := sub[idx[k+2]]
			c.r.FillTriangle(a[0], a[1], b[0], b[1], cc[0], cc[1], col)
		}
	}
}

func (c *CairoCompat) strokeCurrentPath() {
	if len(c.pathPts) < 2 {
		return
	}
	width := c.penWidth
	if width <= 0 {
		// Cairo treats width=0 as a hairline (one pixel regardless of
		// scale). For our purposes the simplest correct fallback is 1pt.
		width = 1
	}
	col := paintColorToGlui(c.penColor)
	style := StrokeStyle{
		Width: width,
		Color: col,
		Join:  JoinMiter,
		Cap:   CapButt,
	}
	for i, start := range c.pathSubs {
		end := len(c.pathPts)
		if i+1 < len(c.pathSubs) {
			end = c.pathSubs[i+1]
		}
		sub := c.pathPts[start:end]
		if len(sub) < 2 {
			continue
		}
		c.r.Polyline(sub, style)
	}
}

func (c *CairoCompat) resetPath() {
	c.pathPts = c.pathPts[:0]
	c.pathSubs = c.pathSubs[:0]
}

// --- Whole-area paint -------------------------------------------------

// Paint fills the entire current clip area (or the framebuffer when no
// clip is active) with the current brush.
func (c *CairoCompat) Paint() {
	c.paintFullClip(c.brushColor)
}

func (c *CairoCompat) PaintWithAlpha(alpha uint8) {
	col := c.brushColor
	col.A = uint8((uint16(col.A) * uint16(alpha)) / 255)
	c.paintFullClip(col)
}

func (c *CairoCompat) paintFullClip(cr paint.Color) {
	col := paintColorToGlui(cr)
	r := c.clipRect
	if r.Width <= 0 || r.Height <= 0 {
		// No active clip — fall back to the whole framebuffer in logical
		// units. This is the right answer for "paint background" calls
		// before any clip has been set.
		r = geom.Rect{X: 0, Y: 0, Width: float64(c.r.frameW), Height: float64(c.r.frameH)}
	}
	c.r.FillRect(Rect{
		X: float32(r.X), Y: float32(r.Y),
		W: float32(r.Width), H: float32(r.Height),
	}, col)
}

// --- Clip -------------------------------------------------------------

// Clip intersects the current clip with the bounding box of the current
// path and resets the path. Sub-path-by-sub-path bounds are not honoured —
// only the union AABB is — which matches what every standard widget needs
// (each one's first call is a Rectangle clip).
func (c *CairoCompat) Clip() {
	c.applyClip()
	c.resetPath()
}

func (c *CairoCompat) ClipPreserve() {
	c.applyClip()
}

func (c *CairoCompat) applyClip() {
	if len(c.pathPts) == 0 {
		return
	}
	// Compute path AABB in logical coords, then transform corners through
	// the current CTM so the scissor rect is in window-space.
	minX, minY := float64(c.pathPts[0][0]), float64(c.pathPts[0][1])
	maxX, maxY := minX, minY
	for _, p := range c.pathPts {
		if float64(p[0]) < minX {
			minX = float64(p[0])
		}
		if float64(p[0]) > maxX {
			maxX = float64(p[0])
		}
		if float64(p[1]) < minY {
			minY = float64(p[1])
		}
		if float64(p[1]) > maxY {
			maxY = float64(p[1])
		}
	}

	// Transform the four corners through the CTM, then re-aabb. Important
	// for scrolled/rotated containers: a Rectangle(0,0,w,h) at a translated
	// origin must scissor at the translated location.
	tx0, ty0 := c.ctm.Transform(minX, minY)
	tx1, ty1 := c.ctm.Transform(maxX, minY)
	tx2, ty2 := c.ctm.Transform(maxX, maxY)
	tx3, ty3 := c.ctm.Transform(minX, maxY)
	wx0 := math.Min(math.Min(tx0, tx1), math.Min(tx2, tx3))
	wy0 := math.Min(math.Min(ty0, ty1), math.Min(ty2, ty3))
	wx1 := math.Max(math.Max(tx0, tx1), math.Max(tx2, tx3))
	wy1 := math.Max(math.Max(ty0, ty1), math.Max(ty2, ty3))

	winRect := geom.Rect{X: wx0, Y: wy0, Width: wx1 - wx0, Height: wy1 - wy0}

	// Intersect with active clip if any.
	if c.clipRect.Width > 0 && c.clipRect.Height > 0 {
		winRect = intersectGeomRect(winRect, c.clipRect)
	}

	c.clipRect = winRect
	c.r.PushClip(Rect{
		X: float32(winRect.X), Y: float32(winRect.Y),
		W: float32(winRect.Width), H: float32(winRect.Height),
	})
	// Tag with the current Save depth so Restore() can pop only clips
	// pushed inside (or below) the scope being restored. See the matching
	// comment in Restore() for the predicate's correctness rationale.
	c.clipPushedAt = append(c.clipPushedAt, len(c.stateStack))
}

func intersectGeomRect(a, b geom.Rect) geom.Rect {
	x0 := math.Max(a.X, b.X)
	y0 := math.Max(a.Y, b.Y)
	x1 := math.Min(a.X+a.Width, b.X+b.Width)
	y1 := math.Min(a.Y+a.Height, b.Y+b.Height)
	if x1 < x0 {
		x1 = x0
	}
	if y1 < y0 {
		y1 = y0
	}
	return geom.Rect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
}

func (c *CairoCompat) ResetClip() {
	for len(c.clipPushedAt) > 0 {
		c.r.PopClip()
		c.clipPushedAt = c.clipPushedAt[:len(c.clipPushedAt)-1]
	}
	c.clipRect = geom.Rect{}
}

func (c *CairoCompat) ClipBounds() (x, y, width, height float64) {
	return c.clipRect.X, c.clipRect.Y, c.clipRect.Width, c.clipRect.Height
}

func (c *CairoCompat) ClipBounds1() geom.Rect { return c.clipRect }

// --- Operator (no-op for now) -----------------------------------------

func (c *CairoCompat) SetOperator(op paint.Operator) {
	// glui hard-codes SRC_OVER blending (set up in Context.Init). Honouring
	// arbitrary operators would require flushing + flipping gl.BlendFunc on
	// each change — deferred until a real widget needs it.
}

// --- Pen / Brush ------------------------------------------------------

func (c *CairoCompat) SetPen(pen paint.Pen) {
	if pen == nil {
		c.penColor = paint.Color{}
		c.penWidth = 0
		return
	}
	c.penColor = pen.Color()
	c.penWidth = float32(pen.Width())
}

func (c *CairoCompat) SetPen1(cr paint.Color, width float64) {
	c.penColor = cr
	c.penWidth = float32(width)
}

func (c *CairoCompat) SetBrush(br paint.Brush) {
	switch p := br.(type) {
	case *paint.SolidBrush:
		c.brushColor = p.Color
	case paint.Color:
		c.brushColor = p
	case nil:
		c.brushColor = paint.Color{}
	default:
		// Pixmap brushes etc. are not supported. Fall back to transparent
		// rather than rendering with stale colour.
		c.brushColor = paint.Color{}
	}
}

func (c *CairoCompat) SetBrush1(cr paint.Color) { c.brushColor = cr }

// --- Font / Text ------------------------------------------------------

func (c *CairoCompat) SetFont(f paint.Font) { c.font = f }

func (c *CairoCompat) Font() paint.Font {
	if c.font == nil {
		// Match cairoPainter.Font(): if nobody set one, allocate a default
		// the next caller can mutate.
		c.font = paint.NewFont("default", 14, false, false)
	}
	return c.font
}

// ScaledFont returns nil — glui does not expose Cairo's scaled-font API.
// Widgets that genuinely need per-glyph metrics should call paint.Font's
// own TextExtents/TextToGlyphs (that interface lives on the paint side and
// works from any backend because it consults Cairo internally).
func (c *CairoCompat) ScaledFont() paint.ScaledFont { return nil }

// DrawText renders text starting at the current point in the current
// brush colour, using a glui Font sized from the active paint.Font.
func (c *CairoCompat) DrawText(text string) {
	c.drawTextAt(float64(c.curX), float64(c.curY), text)
}

func (c *CairoCompat) DrawText1(x, y float64, text string) {
	c.drawTextAt(x, y, text)
}

func (c *CairoCompat) drawTextAt(x, y float64, text string) {
	if text == "" {
		return
	}
	size := 14
	if c.font != nil {
		s := c.font.Size()
		if s > 0 {
			size = s
		}
	}
	gf := c.fontCache.At(float64(size))
	col := paintColorToGlui(c.brushColor)
	c.r.DrawText(gf, text, float32(x), float32(y), col)
}

// DrawGlyphs and DrawGlyph are no-ops: Cairo glyph IDs are not portable,
// and the standard widget set always reaches glyphs through DrawText/DrawText1.
func (c *CairoCompat) DrawGlyphs(glyphs []paint.Glyph) {}
func (c *CairoCompat) DrawGlyph(glyph *paint.Glyph)    {}

// --- Pixmap / Icon ----------------------------------------------------
//
// Bridge: paint.Pixmap is backed by a Cairo image surface holding ARGB32
// data in BGRA byte order with *premultiplied* alpha. We:
//   1. Pull bytes via pm.Image() — that already swaps R/B, returning a
//      premultiplied image.RGBA.
//   2. Un-premultiply per-pixel into a fresh *image.RGBA. Glui's blend func
//      is SRC_ALPHA / ONE_MINUS_SRC_ALPHA (straight alpha), and skipping
//      this step makes anti-aliased icon edges render too dark.
//   3. UploadTexture once, reuse every subsequent draw.
//
// Cache placement: keyed by paint.Pixmap interface value on CairoCompat,
// which is shared across BindRenderer calls so we don't re-upload every
// frame. icon.Pixmap(size) allocates a fresh *cairoSurface on every call,
// so DrawIcon1 caches at the (icon, size) level instead — see iconTextures.

// uploadPixmap returns a GL texture for pm, uploading on first sight.
// Returns nil if pm is nil/empty or its bytes can't be retrieved.
//
// Stamps lastUsed on every hit AND miss so BeginFrame's LRU eviction sees
// recently-touched entries as alive even when they predate the current frame.
func (c *CairoCompat) uploadPixmap(pm paint.Pixmap) *Texture {
	if pm == nil {
		return nil
	}
	if e, ok := c.pixmapTextures[pm]; ok {
		e.lastUsed = c.frameCount
		return e.tex
	}
	tex := c.uploadPixmapNoCache(pm)
	if tex != nil {
		c.pixmapTextures[pm] = &pixmapEntry{tex: tex, lastUsed: c.frameCount}
		c.enforceCacheCapacity()
	}
	return tex
}

// uploadPixmapNoCache reads pm's pixels and uploads a GL texture. Used by
// uploadPixmap (cached) and the per-icon path (cached at the icon level).
//
// Fast path: when the pixmap exposes a raw data pointer + stride we read
// its BGRA bytes directly, unpremultiply in a single linear pass, and feed
// UploadTextureBGRA. This avoids the per-pixel image.RGBA materialisation
// inside pm.Image() — a measurable hit when many icons re-rasterise on a
// theme change (200+ widget designer scenes upload several MB per second).
//
// Slow path: when DataPtr is nil (rare — only synthetic Pixmap values used
// in tests) we fall back to pm.Image() + the original RGBA conversion.
func (c *CairoCompat) uploadPixmapNoCache(pm paint.Pixmap) *Texture {
	if pm == nil {
		return nil
	}
	if c.r == nil || c.r.ctx == nil {
		return nil
	}
	w := pm.Width()
	h := pm.Height()
	if w <= 0 || h <= 0 {
		return nil
	}

	// Fast path: pull raw BGRA bytes directly when the pixmap exposes them.
	if dataPtr := pm.DataPtr(); dataPtr != nil {
		stride := pm.Stride()
		if stride >= w*4 {
			size := h * stride
			// unsafe.Slice creates a Go view onto the C-owned buffer. We
			// MUST copy before any operation that could outlive the
			// surface (uploads complete inside the gl.TexImage2D call,
			// but unpremultiplying in-place would corrupt the source).
			src := unsafe.Slice((*byte)(dataPtr), size)
			straight := make([]byte, size)
			unpremultiplyBGRA(src, straight, w, h, stride)
			return c.r.ctx.UploadTextureBGRA(w, h, stride, straight)
		}
	}

	// Slow path: route through pm.Image() + image.RGBA conversion.
	src, err := pm.Image()
	if err != nil || src == nil {
		return nil
	}
	rgba, ok := src.(*image.RGBA)
	if !ok {
		// We can't un-premultiply non-RGBA inputs without copying first;
		// give up rather than producing wrong colours.
		return c.r.ctx.UploadTexture(src)
	}
	straight := unpremultiplyRGBA(rgba)
	return c.r.ctx.UploadTexture(straight)
}

// unpremultiplyBGRA reads premultiplied BGRA from src and writes straight
// BGRA into dst. Both buffers share the same stride (bytes per row); the
// trailing tail past w*4 bytes per row is left untouched.
//
// Cairo ARGB32 on little-endian: the byte order in memory is B,G,R,A. We
// only need to divide RGB (here B/G/R bytes) by alpha; the ordering is
// otherwise transparent to UploadTextureBGRA which feeds GL_BGRA directly.
func unpremultiplyBGRA(src, dst []byte, w, h, stride int) {
	rowBytes := w * 4
	for y := 0; y < h; y++ {
		off := y * stride
		s := src[off : off+rowBytes]
		d := dst[off : off+rowBytes]
		for x := 0; x < rowBytes; x += 4 {
			a := s[x+3]
			switch a {
			case 0:
				d[x+0] = 0
				d[x+1] = 0
				d[x+2] = 0
				d[x+3] = 0
			case 255:
				d[x+0] = s[x+0]
				d[x+1] = s[x+1]
				d[x+2] = s[x+2]
				d[x+3] = 255
			default:
				d[x+0] = saturatingDivU8(s[x+0], a)
				d[x+1] = saturatingDivU8(s[x+1], a)
				d[x+2] = saturatingDivU8(s[x+2], a)
				d[x+3] = a
			}
		}
	}
}

// unpremultiplyRGBA returns a fresh image.RGBA where each pixel is the
// straight (non-premultiplied) form of src's premultiplied colour. Cairo
// stores ARGB32 with premultiplied alpha; glui's blend stage expects
// straight alpha, so we divide RGB by alpha before upload.
func unpremultiplyRGBA(src *image.RGBA) *image.RGBA {
	w := src.Rect.Dx()
	h := src.Rect.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		srcRow := src.Pix[y*src.Stride : y*src.Stride+w*4]
		dstRow := dst.Pix[y*dst.Stride : y*dst.Stride+w*4]
		for x := 0; x < w*4; x += 4 {
			r := srcRow[x+0]
			g := srcRow[x+1]
			b := srcRow[x+2]
			a := srcRow[x+3]
			if a == 0 {
				dstRow[x+0] = 0
				dstRow[x+1] = 0
				dstRow[x+2] = 0
				dstRow[x+3] = 0
				continue
			}
			if a == 255 {
				dstRow[x+0] = r
				dstRow[x+1] = g
				dstRow[x+2] = b
				dstRow[x+3] = 255
				continue
			}
			// Saturating divide-by-alpha. Premultiplied source guarantees
			// r,g,b <= a, but rounding from upstream code can produce 1-
			// off values; clamp to 255 to avoid wrap.
			dstRow[x+0] = saturatingDivU8(r, a)
			dstRow[x+1] = saturatingDivU8(g, a)
			dstRow[x+2] = saturatingDivU8(b, a)
			dstRow[x+3] = a
		}
	}
	return dst
}

func saturatingDivU8(c, a uint8) uint8 {
	v := (uint32(c) * 255) / uint32(a)
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// DrawPixmap paints pm at the model-space origin (matches Cairo's
// SetSourceSurface(pm,0,0) + Paint() — DrawPixmap1 with x=y=0).
func (c *CairoCompat) DrawPixmap(pm paint.Pixmap) {
	c.DrawPixmap1(0, 0, pm)
}

func (c *CairoCompat) DrawPixmap1(x, y float64, pm paint.Pixmap) {
	tex := c.uploadPixmap(pm)
	if tex == nil {
		return
	}
	c.r.DrawImage(tex, Rect{
		X: float32(x), Y: float32(y),
		W: float32(tex.Width()), H: float32(tex.Height()),
	}, Color{1, 1, 1, 1})
}

// DrawPixmap2 honours the source offset (x0, y0): the rectangle drawn at
// (x, y) is the sub-image of pm starting at (x0, y0).
func (c *CairoCompat) DrawPixmap2(x, y float64, pm paint.Pixmap, x0, y0 float64) {
	tex := c.uploadPixmap(pm)
	if tex == nil {
		return
	}
	pmW := float32(tex.Width())
	pmH := float32(tex.Height())
	srcW := pmW - float32(x0)
	srcH := pmH - float32(y0)
	if srcW <= 0 || srcH <= 0 {
		return
	}
	c.r.DrawImageRegion(tex,
		Rect{X: float32(x0), Y: float32(y0), W: srcW, H: srcH},
		Rect{X: float32(x), Y: float32(y), W: srcW, H: srcH},
		Color{1, 1, 1, 1},
	)
}

// DrawPixmap5 stretches pm into the (x, y, w, h) rect.
func (c *CairoCompat) DrawPixmap5(x, y, w, h float64, pm paint.Pixmap) {
	tex := c.uploadPixmap(pm)
	if tex == nil {
		return
	}
	c.r.DrawImage(tex, Rect{
		X: float32(x), Y: float32(y),
		W: float32(w), H: float32(h),
	}, Color{1, 1, 1, 1})
}

// DrawIcon renders ico at the model-space origin. fSize is requested
// rasteriser size in points; grayed dims the result to mark disabled state.
func (c *CairoCompat) DrawIcon(ico paint.Icon, fSize float64, grayed bool) {
	c.DrawIcon1(ico, 0, 0, fSize, grayed)
}

// DrawIcon1 renders ico at (x, y) sized fSize. Caches the resulting
// pixmap upload at the (icon, size) level — icon.Pixmap(size) allocates
// a fresh surface every call, so a per-pixmap cache would miss + leak.
func (c *CairoCompat) DrawIcon1(ico paint.Icon, x, y, fSize float64, grayed bool) {
	if ico == nil || ico.IsAir() {
		return
	}
	size := int(fSize + 0.5)
	if size <= 0 {
		return
	}
	key := iconCacheKey{ico: ico, size: size}
	var tex *Texture
	if e, ok := c.iconTextures[key]; ok {
		e.lastUsed = c.frameCount
		tex = e.tex
	} else {
		pm := ico.Pixmap(size)
		if pm == nil {
			return
		}
		tex = c.uploadPixmapNoCache(pm)
		if tex == nil {
			return
		}
		c.iconTextures[key] = &pixmapEntry{tex: tex, lastUsed: c.frameCount}
		c.enforceCacheCapacity()
	}
	tint := Color{1, 1, 1, 1}
	if grayed {
		// Approximation of Cairo's OPERATOR_HSL_LUMINOSITY desaturation:
		// 60% colour, 70% alpha keeps the icon legible while reading as
		// disabled. Not pixel-equivalent but visually acceptable for now.
		tint = Color{0.6, 0.6, 0.6, 0.7}
	}
	c.r.DrawImage(tex, Rect{
		X: float32(x), Y: float32(y),
		W: float32(size), H: float32(size),
	}, tint)
}

// --- Helpers ----------------------------------------------------------

func paintColorToGlui(c paint.Color) Color {
	const f = 1.0 / 255
	return Color{
		R: float32(c.R) * f,
		G: float32(c.G) * f,
		B: float32(c.B) * f,
		A: float32(c.A) * f,
	}
}
