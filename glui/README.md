# silk/glui — Pure-OpenGL 2D Renderer

`glui` is Silk's pure-OpenGL 2D rendering pipeline. It replaces the
Cairo-based `paint` package's CPU rasterizer with shader-driven GPU
rendering: triangulated paths, signed-distance-field text, batched textured
quads, scissor clipping. The result is sub-millisecond frames on 4K
displays where the Cairo path takes 6-8ms.

The Cairo pipeline remains the production renderer on `main`. `glui` lives
on the `opengl` branch and is opt-in via the `SILK_GLUI=1` environment
variable.

## When to use it

Set `SILK_GLUI=1` before launching any Silk app:

```bash
SILK_GLUI=1 go run design.go
```

`gui.Window` reads the env var at startup and, if set, calls `paintGlui()`
instead of the Cairo `paint()` path. Every existing widget (62+ in the
standard set) renders through a paint-compatibility shim with no widget-
side changes — see "CairoCompat bridge" below.

The pure-glui API is exposed for native code that wants to skip the shim:
graph editors, custom panels, the form designer's overlay layer.

## Architecture

```
glui.Context             owns shader programs + global GL state
  └─ glui.Renderer       per-frame command recorder + flush
       ├─ Rect / Round   solid + bordered rects, rounded corners
       ├─ Path           arbitrary 2D paths, triangulated to a fan VBO
       ├─ Glyph          text via CPU-rasterized atlas (font.go)
       ├─ Image          textured quads (icons, backgrounds)
       └─ Transform      modelview stack: translate/scale/rotate/clip
```

Three core types:

- **`Context`**: long-lived. Owns the shader programs, default font cache,
  global GL state. Constructed once per window via `NewContext()`, then
  `Init()` after the GL context is current.
- **`Renderer`**: per-frame. Acquired from `Context.Begin(w, h)`, drained
  with `End()`. All draw calls record into vertex/index buffers; the flush
  inside `End()` issues batched `glDrawElements` calls — typically <50 GL
  calls per frame regardless of widget count.
- **`Texture`**: GPU texture handle. Upload once via
  `Context.UploadTexture(image.Image)`, draw many times.

Batching: each draw call records into a buffer keyed by (shader kind,
texture). When the next call wants a different bucket, the old one flushes.
Solid-colour rects, glyph quads, and textured quads each have their own
shader, so most UI rendering coalesces to three or four buckets per frame.

## Quick start (native glui)

```go
ctx := glui.NewContext()
if err := ctx.Init(); err != nil {
    log.Fatal(err)
}
defer ctx.Destroy()

// Inside the per-frame paint callback:
ctx.Resize(width, height, scale)
r := ctx.Begin(width, height)

r.FillRect(glui.Rect{X: 10, Y: 10, W: 200, H: 100},
    glui.Color{R: 0.2, G: 0.4, B: 0.9, A: 1})

r.FillRoundedRect(glui.Rect{X: 20, Y: 130, W: 180, H: 60}, 8,
    glui.Color{R: 1, G: 1, B: 1, A: 1})

r.StrokeRect(glui.Rect{X: 10, Y: 10, W: 200, H: 100}, 2,
    glui.Color{R: 0, G: 0, B: 0, A: 1})

r.End()
```

### Text

```go
font := glui.NewFont(16)         // 16-pt sans-serif
defer font.Destroy()

r.DrawText(font, "Hello, glui!", 30, 50,
    glui.Color{R: 0, G: 0, B: 0, A: 1})
```

For multi-size text, share a `FontCache`:

```go
fc := glui.NewFontCache()
small := fc.At(12)
big   := fc.At(24)
```

The cache reuses the same atlas across sizes, so `At(12)` is cheap to call
inside a hot draw loop.

### Strokes

```go
style := glui.StrokeStyle{
    Width: 2,
    Color: glui.Color{R: 0, G: 0, B: 0, A: 1},
    Join:  glui.JoinRound,
    Cap:   glui.CapButt,
}
points := [][2]float32{{10, 10}, {100, 50}, {180, 30}}
r.Polyline(points, style)
```

`Polyline` handles miter, bevel, and round joins, plus butt/square/round
caps. For one-shot lines, `r.Line(x0, y0, x1, y1, width, col)` skips the
slice allocation.

### Images

```go
img, _ := loadPNG("icon.png")
tex := ctx.UploadTexture(img)
defer tex.Free()

// Inside the per-frame block:
r.DrawImage(tex,
    glui.Rect{X: 100, Y: 100, W: 32, H: 32},
    glui.Color{R: 1, G: 1, B: 1, A: 1}) // white tint = no tint
```

`DrawImageRegion` lets you sample a sub-rect for sprite atlases.

### Transforms and clipping

```go
r.Save()
r.Translate(50, 50)
r.Rotate(math.Pi / 6)
r.Scale(1.5, 1.5)
r.PushClip(glui.Rect{X: 0, Y: 0, W: 100, H: 100})

// Drawn into the rotated, scaled, scissored region:
r.FillRect(glui.Rect{X: 0, Y: 0, W: 50, H: 50},
    glui.Color{R: 1, G: 0, B: 0, A: 1})

r.PopClip()
r.Restore()
```

Clip rects compose by intersection — pushing two clips scissors against
the smaller of the two. `r.ClipRect(rect, fn)` is sugar for
push/run/pop.

## CairoCompat bridge

Existing widgets call `paint.Painter` methods. To run them through glui
without rewriting every widget, wrap a `Renderer` in a `CairoCompat`:

```go
painter := glui.NewCairoCompat(r)
DrawWidgetAll(rootWidget, painter, 0, 0, 0, 0, w, h)
```

`CairoCompat` implements the full `paint.Painter` interface. Path, fill,
stroke, transform, clip, font, pixmap, and icon calls all route to the
underlying `Renderer`. See `cairo_compat.go` for the supported subset and
the documented approximations (pattern brushes degrade to solid colour,
blend operators are ignored, grayed icons use a 60% colour / 70% alpha
tint instead of HSL_LUMINOSITY).

Across frames the painter must persist — the font atlas and pixmap
texture caches own GL resources that would leak if a fresh painter were
allocated each frame. The pattern is:

```go
if window.gluiPainter == nil {
    window.gluiPainter = glui.NewCairoCompat(r)
} else {
    window.gluiPainter.BindRenderer(r)
}
window.gluiPainter.BeginFrame() // advances LRU + evicts stale textures
```

`BeginFrame` runs cache eviction: entries unused for ~5 seconds (300
frames) are freed. A 256-entry hard cap on each map (pixmaps, icons)
catches pathological growth.

## Performance

- **Zero-alloc draw paths**: `FillRect`, `FillRoundedRect`, `Polyline`
  (after warmup), `DrawText`, `DrawImage`. The `Renderer` reuses vertex
  and index slices across frames; pre-allocated capacity is exposed via
  `pool.go`.
- **Batched flushes**: typical UI scenes flush 3-6 times per frame even
  with hundreds of widgets. The batch key is (shader kind, texture);
  changing texture is the only forced flush in a uniform-style pass.
- **VSync**: glui itself does not control vsync — the host (GLFW window)
  does via `glfw.SwapInterval(1)`.
- **Glyph atlas**: text is uploaded once into a single shared texture per
  size. Per-glyph draws boil down to one quad each; flushing happens at
  the next non-text draw call, not per-glyph.

Benchmarks (M2 Max, 1080p):
- 1000 `FillRect` calls: ~120µs
- 1000 short `DrawText` calls: ~340µs (atlas warm)
- Typical full designer frame (~200 widgets): 0.6-1.2ms paint, vs 6-8ms
  Cairo

Run the benchmarks with:

```bash
go test -bench=. -benchmem ./glui/
```

## Known limitations vs. Cairo

The Cairo pipeline is the reference; glui approximates these features:

- **Pattern brushes** — pixmap-pattern brushes are not honoured (drawn
  transparent). **Linear gradients** are now GPU-accelerated for axis-
  aligned rectangle fills via the `kindGradient` shader (see
  `Renderer.FillGradientRect`). Limitations: only the first and last
  stops are used (multi-stop gradients lose intermediates), and only
  paths that are a single axis-aligned rect take the gradient path —
  non-rect paths fall back to a solid fill of the start stop. **Radial
  gradients** are not GPU-accelerated yet; they collapse to the start
  stop.
- **Blend operators** — `paint.SetOperator` is a no-op. glui hardcodes
  SRC_OVER. Honouring arbitrary operators would need a flush + blend-state
  flip on each change.
- **Path clip** — only the axis-aligned bounding box of the clip path is
  scissored. Stencil-buffer path clipping is on the roadmap.
- **Cairo glyph IDs** — `DrawGlyphs` / `DrawGlyph` are no-ops; widgets
  that take that route render blank. The standard widget set goes through
  `DrawText` / `DrawText1`, which works.
- **Text shaping** — the glyph atlas is a CPU rasterizer over basic Latin
  + the runes the app actually uses; complex shaping (Arabic, Indic,
  emoji ZWJ) is not implemented.

## How to extend

### Add a new shader

1. Write the GLSL in `glui/shader/` (one file per shader pair).
2. Register it in `Context.Init()` alongside the existing solid / glyph /
   image programs.
3. Add a `batchKind` constant and a `setBatch(...)` helper so the
   `Renderer` can route draw calls to the new bucket.
4. Add a public `Renderer` method that records vertices into the new
   bucket. Mirror `FillRect` for solid-fill use cases or `DrawImage` for
   textured ones.

### Add custom geometry

For one-off shapes that don't fit the existing primitives, build the
triangulation in `glui/path/` (the Earcut implementation lives there) and
emit triangles via `Renderer.FillTriangle`. For stroked outlines, use
`Renderer.Polyline` with a closed point list.

### Add a custom backend bridge

`PainterAdapter` (in `painter_adapter.go`) is the pure-glui painter
surface for code that wants to draw with familiar painter calls without
the `paint`/`cairo` import chain. Use it as the model for new bridges to
other UI toolkits — keep `paint` out of the import graph by routing
through this layer.

## File map

| File | Purpose |
|---|---|
| `context.go` | `Context` lifecycle, shader compilation |
| `renderer.go` | per-frame command recorder, batching, project/flush |
| `draw.go` | `FillRect`, `FillRoundedRect`, `StrokeRect`, `Line`, `FillCircle`, `FillTriangle` |
| `stroke.go` | `Polyline`, joins, caps |
| `text.go` + `font.go` | glyph atlas + DrawText |
| `image.go` | `Texture`, `UploadTexture`, `DrawImage`, `DrawImageRegion` |
| `clip.go` | scissor stack |
| `transform.go` | modelview matrix stack |
| `path/` | Earcut triangulation |
| `atlas/` | dynamic glyph/image atlas |
| `shader/` | GLSL sources |
| `painter_adapter.go` | pure-glui paint-style facade |
| `cairo_compat.go` | full `paint.Painter` shim for legacy widgets |

## Status

Experimental. The Cairo path on `main` is the production renderer.
Everything in this package may move, rename, or break across `opengl`-branch
commits without a deprecation period.
