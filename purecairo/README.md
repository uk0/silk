# purecairo

Pure-Go re-implementation of the cairo 2D graphics API, with zero CGO
dependencies. Builds on every platform Go supports — macOS, Linux,
Windows, BSD — without libcairo, libpng, libfreetype, or fontconfig.

## Status

| API surface              | Implementation                         |
|--------------------------|----------------------------------------|
| Image surface            | `image.RGBA` backed, BGRA mirror for GL upload |
| Path (M/L/C/Z + Arc + Rectangle/RoundRect) | full                |
| Fill / FillPreserve      | `golang.org/x/image/vector` AA rasteriser |
| Stroke / StrokePreserve  | per-segment thin-quad + round cap / join |
| Save / Restore stack     | full (matrix, source, line attrs, clip, font) |
| Transform stack          | cairo column-vector semantics           |
| Clip / ClipBounds        | AABB approximation, `cairo_clip_extents` user-space |
| Operators                | `OPERATOR_CLEAR`, `_SOURCE`, `_OVER`    |
| Patterns                 | solid, surface, linear, radial          |
| Text                     | OpenType via `golang.org/x/image/font/opentype` |
| Font discovery           | per-OS system font walk + bundled Go fallback |
| CJK fallback             | PingFang (macOS), Noto-CJK (Linux), MS YaHei (Windows) |
| Glyph rendering          | `font.Face.Glyph` + `draw.DrawMask`     |
| Group / mask / push pop  | stub                                    |
| Path-shaped clip         | not yet (AABB only)                     |
| PDF / PS / SVG export    | not in scope (silk has dedicated packages) |
| Native platform surfaces | not in scope                            |

## Cross-platform usage

The package itself is universal — `go build` works on every Go target.
The only platform-aware code is `font.go`, which probes per-OS font
directories:

| OS      | Probed paths                                                 |
|---------|--------------------------------------------------------------|
| darwin  | `/System/Library/Fonts`, `/System/Library/Fonts/Supplemental`, `/Library/Fonts`, `~/Library/Fonts` |
| linux   | `/usr/share/fonts`, `/usr/share/fonts/truetype/{dejavu,noto}`, `/usr/share/fonts/{TTF,opentype}`, `/usr/local/share/fonts`, `~/.fonts`, `~/.local/share/fonts` |
| windows | `%SystemRoot%\Fonts`, `C:\Windows\Fonts`                     |

If the platform walk misses the requested family, the bundled
`golang.org/x/image/font/gofont/goregular` TrueType is used — every
binary that imports this package always has a face.

## Algorithm references

The implementation cites the cairo C source where the algorithm is
non-trivial. Files referenced:

- `cairo/src/cairo-path-stroke.c` — offset-line stroke flattening
- `cairo/src/cairo-arc.c` — arc → cubic flatten step count
- `cairo/src/cairo-pattern.c` — gradient stop interpolation
- `cairo/src/cairo-image-surface.c` — ARGB32 pixel layout

Where the C source uses approaches that don't translate naturally to
Go (e.g. cairo's trapezoid rasteriser), the Go side substitutes
`golang.org/x/image/vector` — same coverage-based AA result, but
backed by the standard image/draw composition pipeline instead of a
custom `pixman` fallback.

## Use as a standalone library

```go
import "silk/purecairo"

func main() {
    s := purecairo.NewImageSurface(purecairo.FORMAT_ARGB32, 400, 300)
    c := s.NewContext()

    c.SetSourceRGB(1, 1, 1)
    c.Paint()

    c.SetSourceRGB(0.2, 0.4, 0.9)
    c.Rectangle(50, 50, 300, 200)
    c.Fill()

    s.WritePNG("out.png")
}
```

## Use through silk's `cairo` package

silk's existing `cairo` package re-exports this one when built with
`-tags silk_pure_go`. silk's own callers don't need to change anything:

```bash
go build -tags silk_pure_go ./cmd/silkide
```

The resulting binary has no native libcairo linkage:

```bash
otool -L /tmp/silkide      # macOS
ldd     /tmp/silkide       # linux
```

shows only system frameworks and `libSystem.B.dylib` / `libc.so` —
no `libcairo.dylib`, no `libpng`, no `libfreetype`.

## License

Same license as silk.
