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
//   - Pixmap-pattern brushes are not supported (rendered transparent)
//   - Linear gradients are GPU-accelerated only when the filled path is a
//     single axis-aligned rectangle. Two-stop gradients use a uniform-
//     based fast path; gradients with three or more stops upload a 256-
//     pixel 1-D ramp texture and sample it per fragment. Non-rect paths
//     still fall back to a solid fill of the start-stop colour.
//   - Radial gradients use the radial-gradient shader against axis-
//     aligned rect paths and fall back to a solid inner-stop fill on any
//     other shape — same gating policy as linear gradients.
//   - SetOperator handles separable Porter-Duff + ADD / MULTIPLY / SCREEN /
//     DARKEN / LIGHTEN via fixed-function blending; non-separable operators
//     (OVERLAY, COLOR_DODGE/BURN, SOFT/HARD_LIGHT, DIFFERENCE, EXCLUSION,
//     HSL_*) silently fall back to OVER until a fragment-shader variant
//     lands
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

	// arcsInPath records every Arc / ArcNegative emission that contributed
	// to the current path. fillCurrentPath inspects this side buffer to
	// detect the canonical "rounded-rect via 4 corner arcs" pattern and
	// dispatch it to Renderer.FillRoundedRect (the SDF rect shader)
	// instead of paying the path-triangulation cost. MoveTo and
	// Rectangle reset the buffer so each fresh path starts at zero.
	arcsInPath []arcRecord

	// Pen / brush / font state. brushColor mirrors the SOURCE colour the
	// Cairo painter resolves; pen.color/pen.width is the stroke style.
	penColor   paint.Color
	penWidth   float32
	brushColor paint.Color
	font       paint.Font

	// Pen extension state captured from paint.DashedPen / paint.CappedPen
	// when SetPen sees a Pen that implements those optional interfaces. Plain
	// solid pens leave these at their defaults so strokeCurrentPath produces
	// the historical CapButt + JoinMiter solid-line output.
	penDash       []float64
	penDashOffset float64
	penLineCap    paint.LineCap
	penLineJoin   paint.LineJoin
	penMiterLimit float64

	// Active linear-gradient brush state. gradientActive flags whether
	// SetBrush most recently selected a *paint.LinearGradient. When set,
	// fillCurrentPath routes axis-aligned rectangle paths through
	// Renderer.FillGradientRect (two-stop) or FillMultiGradientRect
	// (three or more stops); non-rect paths fall back to a solid fill of
	// gradStart so the shape still renders something visible (and the
	// limitation is logged once in package docs).
	gradientActive   bool
	gradientStart    paint.Color
	gradientEnd      paint.Color
	gradientVertical bool

	// gradientStops carries the full multi-stop list when the active
	// gradient has more than two stops. Empty for single- or two-stop
	// gradients (which still go through gradientStart/gradientEnd) so the
	// fast uniform path can stay in use for the common case.
	gradientStops []GradientStop

	// Active radial-gradient brush state. radialActive flags whether
	// SetBrush most recently selected a *paint.RadialGradient. When set,
	// fillCurrentPath routes axis-aligned rectangle paths through
	// Renderer.FillRadialGradientRect. radialCx/Cy are in the same logical
	// coordinate space the path is built in. radialR0/R1 give inner/outer
	// radii; radialStops is always populated (Cairo radial gradients carry
	// at least two stops, so there's no two-stop fast path here — the
	// shader cost is identical at any stop count).
	radialActive bool
	radialCx     float32
	radialCy     float32
	radialR0     float32
	radialR1     float32
	radialStops  []GradientStop

	// Active pixmap-brush state. pixmapBrushActive flags whether
	// SetBrush most recently selected a *paint.PixmapBrush. When set,
	// fillCurrentPath routes axis-aligned rect paths through
	// Renderer.DrawImage with the cached GL texture for the brush's
	// pixmap. Non-rect paths fall back to a solid fill of pixmapAvgColor
	// (the average colour of the source pixmap, computed on first
	// upload) so the shape still renders something visible.
	pixmapBrushActive bool
	pixmapBrush       paint.Pixmap
	pixmapAvgColor    paint.Color

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
	// clipPushedAt tracks every Renderer-level clip currently on the
	// stack. depth is the Save scope at push time (for Restore-time
	// matching). When isStencil is true, stencilPath holds the
	// triangulated polygon for PopClipPath's decrement-write pass.
	clipPushedAt []clipPushRecord

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

// clipPushRecord is one entry on CairoCompat.clipPushedAt. Each
// records whether the active Renderer clip at that point was a
// rectangular scissor (PushClip) or a path-shaped stencil
// (PushClipPath); Restore matches the kind to pick the right Pop.
type clipPushRecord struct {
	depth       int            // len(stateStack) at push time
	isStencil   bool           // false → PushClip; true → PushClipPath
	stencilPath [][2]float32   // triangulated polygon vertices for PopClipPath
}

type cairoCompatState struct {
	pathLen          int
	subsLen          int
	penColor         paint.Color
	penWidth         float32
	penDash          []float64
	penDashOffset    float64
	penLineCap       paint.LineCap
	penLineJoin      paint.LineJoin
	penMiterLimit    float64
	brushColor       paint.Color
	font             paint.Font
	ctm              geom.Mat3x2
	clipRect         geom.Rect
	gradientActive   bool
	gradientStart    paint.Color
	gradientEnd      paint.Color
	gradientVertical bool
	gradientStops    []GradientStop
	radialActive     bool
	radialCx         float32
	radialCy         float32
	radialR0         float32
	radialR1         float32
	radialStops      []GradientStop
	pixmapBrushActive bool
	pixmapBrush       paint.Pixmap
	pixmapAvgColor    paint.Color
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
		penLineCap:     paint.LineCapButt,
		penLineJoin:    paint.LineJoinMiter,
		penMiterLimit:  10,
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
		pathLen:          len(c.pathPts),
		subsLen:          len(c.pathSubs),
		penColor:         c.penColor,
		penWidth:         c.penWidth,
		penDash:          c.penDash,
		penDashOffset:    c.penDashOffset,
		penLineCap:       c.penLineCap,
		penLineJoin:      c.penLineJoin,
		penMiterLimit:    c.penMiterLimit,
		brushColor:       c.brushColor,
		font:             c.font,
		ctm:              c.ctm,
		clipRect:         c.clipRect,
		gradientActive:   c.gradientActive,
		gradientStart:    c.gradientStart,
		gradientEnd:      c.gradientEnd,
		gradientVertical: c.gradientVertical,
		gradientStops:    c.gradientStops,
		radialActive:     c.radialActive,
		radialCx:         c.radialCx,
		radialCy:         c.radialCy,
		radialR0:         c.radialR0,
		radialR1:         c.radialR1,
		radialStops:      c.radialStops,
		pixmapBrushActive: c.pixmapBrushActive,
		pixmapBrush:       c.pixmapBrush,
		pixmapAvgColor:    c.pixmapAvgColor,
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
	c.penDash = s.penDash
	c.penDashOffset = s.penDashOffset
	c.penLineCap = s.penLineCap
	c.penLineJoin = s.penLineJoin
	c.penMiterLimit = s.penMiterLimit
	c.brushColor = s.brushColor
	c.font = s.font
	c.ctm = s.ctm
	c.gradientActive = s.gradientActive
	c.gradientStart = s.gradientStart
	c.gradientEnd = s.gradientEnd
	c.gradientVertical = s.gradientVertical
	c.gradientStops = s.gradientStops
	c.radialActive = s.radialActive
	c.radialCx = s.radialCx
	c.radialCy = s.radialCy
	c.radialR0 = s.radialR0
	c.radialR1 = s.radialR1
	c.radialStops = s.radialStops
	c.pixmapBrushActive = s.pixmapBrushActive
	c.pixmapBrush = s.pixmapBrush
	c.pixmapAvgColor = s.pixmapAvgColor
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
	for len(c.clipPushedAt) > 0 && c.clipPushedAt[len(c.clipPushedAt)-1].depth > len(c.stateStack) {
		rec := c.clipPushedAt[len(c.clipPushedAt)-1]
		if rec.isStencil {
			c.r.PopClipPath(rec.stencilPath)
		} else {
			c.r.PopClip()
		}
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
	// MoveTo starts a fresh sub-path; reset the arc-tracker so an old
	// path's arc records don't contaminate the rounded-rect detector.
	c.arcsInPath = c.arcsInPath[:0]
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

	// Record the arc for the rounded-rect detector. We store the
	// post-normalisation (a0, a1) so the span check downstream sees the
	// effective sweep.
	c.arcsInPath = append(c.arcsInPath, arcRecord{
		cx: float32(xc), cy: float32(yc),
		radius: float32(radius),
		a0:     a0, a1: a1,
	})

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
	// A plain Rectangle starts a fresh sub-path with no arcs. Clear the
	// detector so a leftover arc from a previous path doesn't combine
	// with these four corners and falsely pass the rounded-rect gate.
	c.arcsInPath = c.arcsInPath[:0]
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

	// Gradient fast path: if the brush is a linear gradient and the path is a
	// single axis-aligned rectangle (the common case — buttons, chart areas,
	// gauge fills) route through Renderer.FillGradientRect for a true GPU
	// gradient. Other path shapes fall through to the solid triangulation
	// fill below using the start-stop colour, matching the documented
	// limitation in CairoCompat's package comment.
	//
	// When three or more stops are present, take the multi-stop ramp path
	// instead. The two-stop uniform path stays the default for ordinary
	// buttons because it skips the texture upload and round-trip — the
	// vast majority of UI gradients are 2-stop, so optimising that case is
	// worth the small branching cost here.
	if c.gradientActive {
		if rc, ok := c.singleAxisAlignedRectPath(); ok {
			if len(c.gradientStops) >= 3 {
				c.r.FillMultiGradientRect(rc, c.gradientStops, c.gradientVertical)
			} else {
				start := paintColorToGlui(c.gradientStart)
				end := paintColorToGlui(c.gradientEnd)
				c.r.FillGradientRect(rc, start, end, c.gradientVertical)
			}
			return
		}
	}

	// Radial gradient fast path: same single-axis-aligned-rect gate as the
	// linear gradient. Centre and radii are taken straight from the brush;
	// non-rect paths fall through to the solid-fill triangulation below
	// using brushColor (set to the inner stop in SetBrush) so the shape
	// still renders something visible.
	if c.radialActive {
		if rc, ok := c.singleAxisAlignedRectPath(); ok {
			c.r.FillRadialGradientRect(rc, c.radialCx, c.radialCy, c.radialR0, c.radialR1, c.radialStops)
			return
		}
	}

	// Pixmap brush fast path: an active *paint.PixmapBrush + an axis-
	// aligned rect path routes through DrawImage with the cached brush
	// texture. The image is stretched to fill the rect (Extend modes
	// other than NONE are not honoured yet — same simplification as the
	// gradient paths). Non-rect paths fall through to the triangulated
	// solid fill below using the source pixmap's average colour
	// (computed once at SetBrush time) so the shape stays visible.
	if c.pixmapBrushActive && c.pixmapBrush != nil {
		if rc, ok := c.singleAxisAlignedRectPath(); ok {
			tex := c.uploadPixmap(c.pixmapBrush)
			if tex != nil {
				c.r.DrawImage(tex, rc, Color{1, 1, 1, 1})
				return
			}
		}
	}

	// Rounded-rect fast path: solid-brush fills of the canonical 4-arc
	// + 4-line shape route through Renderer.FillRoundedRect (SDF rect
	// shader) instead of triangulating ~64 line segments per corner
	// (16/quarter × 4). This is the difference between the "glui slower
	// than Cairo on round-rect" benchmark regression and the headline
	// 3-6× win — tessellated arcs dominated CPU time. Brush
	// gating: only solid; gradient/pixmap brushes aren't yet wired
	// through FillRoundedRect on the GPU side.
	if !c.gradientActive && !c.radialActive && !c.pixmapBrushActive {
		if rc, radius, ok := c.detectRoundedRect(); ok {
			col := paintColorToGlui(c.brushColor)
			c.r.FillRoundedRect(rc, radius, col)
			return
		}
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

// arcRecord captures one Arc / ArcNegative emission so fillCurrentPath
// can recognise the canonical rounded-rect path (4 corner arcs + 4
// connecting line segments) and dispatch to the SDF rect shader.
type arcRecord struct {
	cx, cy float32
	radius float32
	a0, a1 float64
}

// detectRoundedRect inspects arcsInPath + pathSubs to decide whether
// the current path is exactly one rounded rectangle. On match it
// returns the outer rect, the corner radius, and ok=true.
//
// Rules (any failure returns ok=false; the caller falls through to
// the slow tessellation path):
//
//   - Exactly one sub-path
//   - Exactly four arcs recorded
//   - All four arcs share the same radius
//   - Each arc spans π/2 (a quarter-turn) within float32 tolerance
//   - The four arc centres form an axis-aligned rectangle in the same
//     order rounded-rect helpers produce: top-left, top-right,
//     bottom-right, bottom-left (or any rotation thereof)
//
// The outer rectangle is computed from min/max arc-centre coords
// expanded by the radius — this is exactly what FillRoundedRect's SDF
// shader expects.
func (c *CairoCompat) detectRoundedRect() (Rect, float32, bool) {
	if len(c.pathSubs) != 1 {
		return Rect{}, 0, false
	}
	if len(c.arcsInPath) != 4 {
		return Rect{}, 0, false
	}

	r := c.arcsInPath[0].radius
	if r <= 0 {
		return Rect{}, 0, false
	}
	const radiusTol = 1e-3
	const spanTol = 1e-3
	for _, a := range c.arcsInPath {
		if math.Abs(float64(a.radius-r)) > radiusTol {
			return Rect{}, 0, false
		}
		span := math.Abs(a.a1 - a.a0)
		if math.Abs(span-math.Pi/2) > spanTol {
			return Rect{}, 0, false
		}
	}

	xs := [4]float32{c.arcsInPath[0].cx, c.arcsInPath[1].cx, c.arcsInPath[2].cx, c.arcsInPath[3].cx}
	ys := [4]float32{c.arcsInPath[0].cy, c.arcsInPath[1].cy, c.arcsInPath[2].cy, c.arcsInPath[3].cy}
	minX, maxX := xs[0], xs[0]
	minY, maxY := ys[0], ys[0]
	for i := 1; i < 4; i++ {
		if xs[i] < minX {
			minX = xs[i]
		}
		if xs[i] > maxX {
			maxX = xs[i]
		}
		if ys[i] < minY {
			minY = ys[i]
		}
		if ys[i] > maxY {
			maxY = ys[i]
		}
	}
	if maxX <= minX || maxY <= minY {
		return Rect{}, 0, false
	}
	// Each centre must hit one of the four corners (minX|maxX, minY|maxY).
	for i := 0; i < 4; i++ {
		if xs[i] != minX && xs[i] != maxX {
			return Rect{}, 0, false
		}
		if ys[i] != minY && ys[i] != maxY {
			return Rect{}, 0, false
		}
	}
	// Each of the four corner positions must appear exactly once across
	// the centre array; otherwise we have two arcs at the same corner
	// and another corner missing — not a rounded rect.
	corners := map[[2]float32]int{}
	for i := 0; i < 4; i++ {
		corners[[2]float32{xs[i], ys[i]}]++
	}
	if len(corners) != 4 {
		return Rect{}, 0, false
	}

	rect := Rect{
		X: minX - r,
		Y: minY - r,
		W: maxX - minX + 2*r,
		H: maxY - minY + 2*r,
	}
	return rect, r, true
}

// singleAxisAlignedRectPath reports whether the current path consists of
// exactly one sub-path made of four corners of an axis-aligned rectangle
// (with an optional duplicated close vertex). Returns the rect on success.
//
// Only Rectangle() / Rectangle1() emit this exact shape; any path produced
// by MoveTo+LineTo with rotation, skew, or extra anchors fails the check
// and the caller falls back to triangulated fill. We accept both 4 unique
// vertices and the 5-vertex closed form Rectangle() emits.
func (c *CairoCompat) singleAxisAlignedRectPath() (Rect, bool) {
	if len(c.pathSubs) != 1 {
		return Rect{}, false
	}
	pts := c.pathPts
	n := len(pts)
	// Drop the trailing close vertex if it duplicates the first.
	if n == 5 && pts[0] == pts[4] {
		n = 4
	}
	if n != 4 {
		return Rect{}, false
	}
	// Axis-aligned check: x and y values should reduce to exactly two
	// distinct values across the four corners.
	xs := [4]float32{pts[0][0], pts[1][0], pts[2][0], pts[3][0]}
	ys := [4]float32{pts[0][1], pts[1][1], pts[2][1], pts[3][1]}
	minX, maxX := xs[0], xs[0]
	minY, maxY := ys[0], ys[0]
	for i := 1; i < 4; i++ {
		if xs[i] < minX {
			minX = xs[i]
		}
		if xs[i] > maxX {
			maxX = xs[i]
		}
		if ys[i] < minY {
			minY = ys[i]
		}
		if ys[i] > maxY {
			maxY = ys[i]
		}
	}
	// Each x must be either minX or maxX; same for y. Otherwise the path is
	// some non-axis-aligned quad that needs triangulation.
	for i := 0; i < 4; i++ {
		if xs[i] != minX && xs[i] != maxX {
			return Rect{}, false
		}
		if ys[i] != minY && ys[i] != maxY {
			return Rect{}, false
		}
	}
	if maxX <= minX || maxY <= minY {
		return Rect{}, false
	}
	return Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}, true
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
	// Translate captured extension state into a glui StrokeStyle. When the
	// Pen didn't implement DashedPen or CappedPen, SetPen left the fields
	// at their CapButt + JoinMiter + solid defaults, so the legacy plain-
	// pen behaviour is preserved without a dedicated branch here.
	style := StrokeStyle{
		Width:      width,
		Color:      col,
		MiterLimit: float32(c.penMiterLimit),
	}
	switch c.penLineCap {
	case paint.LineCapRound:
		style.Cap = CapRound
	case paint.LineCapSquare:
		style.Cap = CapSquare
	default:
		style.Cap = CapButt
	}
	switch c.penLineJoin {
	case paint.LineJoinRound:
		style.Join = JoinRound
	case paint.LineJoinBevel:
		style.Join = JoinBevel
	default:
		style.Join = JoinMiter
	}
	if len(c.penDash) > 0 {
		dash32 := make([]float32, len(c.penDash))
		for i, d := range c.penDash {
			dash32[i] = float32(d)
		}
		style.Dash = dash32
		style.DashOffset = float32(c.penDashOffset)
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
	// Cairo's cairo_clip / cairo_fill / cairo_stroke (without _preserve)
	// also clear the "current point" — subsequent cairo_get_current_point
	// returns (0, 0). Mirror that here so widgets that rely on DrawText
	// (which reads c.curX/c.curY) after a parent's clip-rect setup don't
	// inherit the clip rect's origin as their text position. Without
	// this reset, DrawWidgetAll's prologue (Rectangle + Clip + Translate)
	// leaves curX/curY at the clip rect's origin, and a child widget
	// calling DrawText draws its glyphs at that offset within its
	// already-translated coordinate space — resulting in text rendered
	// far outside the visible region.
	c.curX, c.curY = 0, 0
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

	// Decide between scissor (AABB) and stencil (path-shaped). The CTM
	// has rotation / skew when off-diagonal terms (Xy, Yx) are non-zero;
	// in that case an AABB scissor would clip too generously and let
	// content leak past the rotated container's visual bounds. Scaling-
	// only CTMs (Xy == Yx == 0) still produce axis-aligned clip rects
	// after transformation, so they keep the scissor fast path.
	rotated := c.ctm.Xy != 0 || c.ctm.Yx != 0

	if rotated {
		c.applyStencilClip()
		return
	}
	c.applyScissorClip()
}

// applyScissorClip computes the AABB of the current path under the CTM
// and pushes a rectangular scissor. Historical fast path retained for
// the dominant case (no rotation; widget-tree clips are translation +
// scale only).
func (c *CairoCompat) applyScissorClip() {
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

	// Transform the four corners through the CTM, then re-aabb.
	tx0, ty0 := c.ctm.Transform(minX, minY)
	tx1, ty1 := c.ctm.Transform(maxX, minY)
	tx2, ty2 := c.ctm.Transform(maxX, maxY)
	tx3, ty3 := c.ctm.Transform(minX, maxY)
	wx0 := math.Min(math.Min(tx0, tx1), math.Min(tx2, tx3))
	wy0 := math.Min(math.Min(ty0, ty1), math.Min(ty2, ty3))
	wx1 := math.Max(math.Max(tx0, tx1), math.Max(tx2, tx3))
	wy1 := math.Max(math.Max(ty0, ty1), math.Max(ty2, ty3))

	winRect := geom.Rect{X: wx0, Y: wy0, Width: wx1 - wx0, Height: wy1 - wy0}

	if c.clipRect.Width > 0 && c.clipRect.Height > 0 {
		winRect = intersectGeomRect(winRect, c.clipRect)
	}

	c.clipRect = winRect
	c.r.PushClip(Rect{
		X: float32(winRect.X), Y: float32(winRect.Y),
		W: float32(winRect.Width), H: float32(winRect.Height),
	})
	c.clipPushedAt = append(c.clipPushedAt, clipPushRecord{
		depth:     len(c.stateStack),
		isStencil: false,
	})
}

// applyStencilClip transforms the current path through the CTM and
// pushes a path-shaped stencil clip. Used when the CTM contains
// rotation or skew so an AABB scissor would over-clip.
//
// The resulting stencil mask is in window-space coordinates — the
// renderer's project() applies its own transform stack, so we transform
// each path point here before pushing.
func (c *CairoCompat) applyStencilClip() {
	pts := make([][2]float32, 0, len(c.pathPts))
	for _, p := range c.pathPts {
		// Transform through the CairoCompat CTM (which mirrors the
		// renderer's xform). Renderer.PushClipPath then re-projects
		// to NDC inside the renderer's pipeline.
		x, y := c.ctm.Transform(float64(p[0]), float64(p[1]))
		pts = append(pts, [2]float32{float32(x), float32(y)})
	}
	// PushClipPath emits triangles in identity space because we already
	// applied the CTM. Save the renderer's xform, switch to identity,
	// push the clip, then restore.
	prevXform := c.r.xform
	c.r.xform = identityMatrix3()
	c.r.PushClipPath(pts)
	c.r.xform = prevXform

	// clipRect (the AABB shadow used by ClipBounds and Paint full-clip)
	// stays at its previous value — stencil clip doesn't have a
	// meaningful AABB without re-aabb'ing the rotated polygon's bounds,
	// which we leave for callers that explicitly query.

	c.clipPushedAt = append(c.clipPushedAt, clipPushRecord{
		depth:       len(c.stateStack),
		isStencil:   true,
		stencilPath: pts,
	})
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
		rec := c.clipPushedAt[len(c.clipPushedAt)-1]
		if rec.isStencil {
			c.r.PopClipPath(rec.stencilPath)
		} else {
			c.r.PopClip()
		}
		c.clipPushedAt = c.clipPushedAt[:len(c.clipPushedAt)-1]
	}
	c.clipRect = geom.Rect{}
}

func (c *CairoCompat) ClipBounds() (x, y, width, height float64) {
	return c.clipRect.X, c.clipRect.Y, c.clipRect.Width, c.clipRect.Height
}

func (c *CairoCompat) ClipBounds1() geom.Rect { return c.clipRect }

// --- Operator ---------------------------------------------------------

func (c *CairoCompat) SetOperator(op paint.Operator) {
	// Renderer.SetBlendOp flushes the current batch before reprogramming
	// gl.BlendFunc / gl.BlendEquation, so already-emitted geometry blends
	// against the framebuffer using the previously-active operator. Ops
	// the fixed-function pipeline can't express fall back to OVER inside
	// SetBlendOp.
	c.r.SetBlendOp(op)
}

// --- Pen / Brush ------------------------------------------------------

func (c *CairoCompat) SetPen(pen paint.Pen) {
	if pen == nil {
		c.penColor = paint.Color{}
		c.penWidth = 0
		c.penDash = nil
		c.penDashOffset = 0
		c.penLineCap = paint.LineCapButt
		c.penLineJoin = paint.LineJoinMiter
		c.penMiterLimit = 10
		return
	}
	c.penColor = pen.Color()
	c.penWidth = float32(pen.Width())

	// Reset extensions to defaults; the capability-interface assertions below
	// override them when the Pen carries the relevant data.
	c.penDash = nil
	c.penDashOffset = 0
	c.penLineCap = paint.LineCapButt
	c.penLineJoin = paint.LineJoinMiter
	c.penMiterLimit = 10

	if dp, ok := pen.(paint.DashedPen); ok {
		c.penDash = dp.Dash()
		c.penDashOffset = dp.DashOffset()
	}
	if cp, ok := pen.(paint.CappedPen); ok {
		c.penLineCap = cp.LineCap()
		c.penLineJoin = cp.LineJoin()
		c.penMiterLimit = cp.MiterLimit()
	}
}

func (c *CairoCompat) SetPen1(cr paint.Color, width float64) {
	c.penColor = cr
	c.penWidth = float32(width)
	// SetPen1 is the "simple" path (Color + width only); reset the
	// extension state so a previously-styled SetPen() doesn't bleed dash or
	// cap settings into the next stroke.
	c.penDash = nil
	c.penDashOffset = 0
	c.penLineCap = paint.LineCapButt
	c.penLineJoin = paint.LineJoinMiter
	c.penMiterLimit = 10
}

func (c *CairoCompat) SetBrush(br paint.Brush) {
	// A new brush always replaces every brush-mode flag. The default cleared
	// state below means non-special brushes (solid, nil) read out as all
	// flags false, and the next Fill/Paint takes the solid colour path.
	c.gradientActive = false
	c.gradientStart = paint.Color{}
	c.gradientEnd = paint.Color{}
	c.gradientVertical = false
	c.gradientStops = c.gradientStops[:0]
	c.radialActive = false
	c.radialCx, c.radialCy = 0, 0
	c.radialR0, c.radialR1 = 0, 0
	c.radialStops = c.radialStops[:0]
	c.pixmapBrushActive = false
	c.pixmapBrush = nil
	c.pixmapAvgColor = paint.Color{}

	switch p := br.(type) {
	case *paint.SolidBrush:
		c.brushColor = p.Color
	case paint.Color:
		c.brushColor = p
	case *paint.LinearGradient:
		stops := p.Stops
		if len(stops) >= 2 {
			c.gradientActive = true
			c.gradientStart = stops[0].Color
			c.gradientEnd = stops[len(stops)-1].Color
			// Choose axis: if |dy| > |dx| the gradient is mostly vertical,
			// otherwise horizontal. The shader path only honours the two
			// canonical axes today; a fully-arbitrary axis would need extra
			// per-vertex t computation. Most UI gradients are top-to-bottom
			// or left-to-right, so this covers the common case cleanly.
			dx := p.X1 - p.X0
			dy := p.Y1 - p.Y0
			if dy < 0 {
				dy = -dy
			}
			if dx < 0 {
				dx = -dx
			}
			c.gradientVertical = dy > dx
			// Fallback brush colour mirrors the start stop so non-rect paths
			// still render legibly with a flat colour.
			c.brushColor = stops[0].Color
			// Capture every stop when the gradient has more than two so
			// fillCurrentPath can hit the ramp-texture path. Two-stop
			// gradients deliberately keep gradientStops empty so they
			// continue using the cheaper uniform-only fast path. We snapshot
			// the slice to insulate the renderer from later mutations of
			// the user's *paint.LinearGradient.
			if len(stops) > 2 {
				if cap(c.gradientStops) >= len(stops) {
					c.gradientStops = c.gradientStops[:len(stops)]
				} else {
					c.gradientStops = make([]GradientStop, len(stops))
				}
				for i, s := range stops {
					c.gradientStops[i] = GradientStop{
						Position: s.Offset,
						Color:    paintColorToGlui(s.Color),
					}
				}
			}
		} else if len(stops) == 1 {
			// Single-stop gradient is just a solid brush.
			c.brushColor = stops[0].Color
		} else {
			c.brushColor = paint.Color{}
		}
	case *paint.RadialGradient:
		stops := p.Stops
		if len(stops) >= 2 {
			c.radialActive = true
			c.radialCx = p.Cx
			c.radialCy = p.Cy
			c.radialR0 = p.R0
			c.radialR1 = p.R1
			// Snapshot stops to insulate the renderer from later mutations
			// of the user's *paint.RadialGradient (same defensive copy as
			// the linear-gradient branch).
			if cap(c.radialStops) >= len(stops) {
				c.radialStops = c.radialStops[:len(stops)]
			} else {
				c.radialStops = make([]GradientStop, len(stops))
			}
			for i, s := range stops {
				c.radialStops[i] = GradientStop{
					Position: s.Offset,
					Color:    paintColorToGlui(s.Color),
				}
			}
			// Fallback solid colour for non-rect paths uses the inner stop
			// (offset 0) — matches the visual hint of the gradient centre
			// when the shader path can't apply.
			c.brushColor = stops[0].Color
		} else if len(stops) == 1 {
			c.brushColor = stops[0].Color
		} else {
			c.brushColor = paint.Color{}
		}
	case *paint.PixmapBrush:
		pm := p.Pixmap()
		if pm != nil && pm.Width() > 0 && pm.Height() > 0 {
			c.pixmapBrushActive = true
			c.pixmapBrush = pm
			// Average colour fallback for non-rect paths. We compute
			// once on SetBrush; for typical icon-sized brushes this is
			// near-instant. Larger brushes pay the cost once per
			// SetBrush call rather than per draw, which is fine
			// because PixmapBrush is rarely re-set inside a frame.
			c.pixmapAvgColor = pixmapAverageColor(pm)
			c.brushColor = c.pixmapAvgColor
		} else {
			c.brushColor = paint.Color{}
		}
	case nil:
		c.brushColor = paint.Color{}
	default:
		// Unknown brush type. Fall back to transparent rather than
		// rendering with stale colour.
		c.brushColor = paint.Color{}
	}
}

// SetBrush1 sets a solid brush colour and clears any active gradient
// or pixmap brush.
func (c *CairoCompat) SetBrush1(cr paint.Color) {
	c.brushColor = cr
	c.gradientActive = false
	c.gradientStart = paint.Color{}
	c.gradientEnd = paint.Color{}
	c.gradientVertical = false
	c.gradientStops = c.gradientStops[:0]
	c.radialActive = false
	c.radialCx, c.radialCy = 0, 0
	c.radialR0, c.radialR1 = 0, 0
	c.radialStops = c.radialStops[:0]
	c.pixmapBrushActive = false
	c.pixmapBrush = nil
	c.pixmapAvgColor = paint.Color{}
}

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

// --- Box shadow ------------------------------------------------------
//
// CairoCompat implements paint.ShadowPainter: widgets that want a
// drop shadow can type-assert their Painter to ShadowPainter and call
// FillBoxShadow. On the Cairo back-end the type assertion fails (Cairo's
// painter does not satisfy the interface) and the widget falls back to
// drawing without a shadow — the documented degradation pattern.

// Compile-time check: CairoCompat must satisfy paint.ShadowPainter so
// widget-side type assertions actually find the method.
var _ paint.ShadowPainter = (*CairoCompat)(nil)

// FillBoxShadow renders a soft drop shadow under the given rect. Translates
// from paint's logical units + Color into the renderer's float32 form, then
// delegates to Renderer.FillBoxShadow which uses the rect SDF shader to
// produce a true GPU blur.
//
// The host code should draw the actual rectangle ON TOP of this shadow, not
// beneath — see Renderer.FillBoxShadow for the rendering contract.
func (c *CairoCompat) FillBoxShadow(rc geom.Rect, radius, blur float64, col paint.Color) {
	if c.r == nil {
		return
	}
	c.r.FillBoxShadow(Rect{
		X: float32(rc.X), Y: float32(rc.Y),
		W: float32(rc.Width), H: float32(rc.Height),
	}, float32(radius), float32(blur), paintColorToGlui(col))
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

// pixmapAverageColor returns a representative single colour for pm,
// used by the PixmapBrush fallback path when the active path can't be
// detected as an axis-aligned rect. We sample on a sparse grid (16
// rows × 16 columns at most) rather than averaging every pixel — the
// fallback is best-effort and the user only sees this colour when
// they're filling a non-rect shape with a textured brush, which is
// rare.
//
// Returns transparent black on any error reading the pixmap data.
func pixmapAverageColor(pm paint.Pixmap) paint.Color {
	if pm == nil {
		return paint.Color{}
	}
	w := pm.Width()
	h := pm.Height()
	if w <= 0 || h <= 0 {
		return paint.Color{}
	}
	// Use the pixmap's own pixel iterator via Image() — slow on large
	// pixmaps but precise. We only call this once per SetBrush so the
	// per-frame cost is zero.
	src, err := pm.Image()
	if err != nil || src == nil {
		return paint.Color{}
	}
	bounds := src.Bounds()
	stepX := bounds.Dx() / 16
	if stepX < 1 {
		stepX = 1
	}
	stepY := bounds.Dy() / 16
	if stepY < 1 {
		stepY = 1
	}
	var sumR, sumG, sumB, sumA, n uint64
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			r, g, b, a := src.At(x, y).RGBA()
			sumR += uint64(r >> 8)
			sumG += uint64(g >> 8)
			sumB += uint64(b >> 8)
			sumA += uint64(a >> 8)
			n++
		}
	}
	if n == 0 {
		return paint.Color{}
	}
	return paint.Color{
		R: uint8(sumR / n),
		G: uint8(sumG / n),
		B: uint8(sumB / n),
		A: uint8(sumA / n),
	}
}
