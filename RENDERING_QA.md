# Core rendering and designer iteration — 2026-10-11

This is a bounded core iteration toward Qt Creator-style design workflows, not
full Qt Creator/Qt Widgets feature or performance parity.

## Pixel transfer correctness and allocation

The original native `Surface.SetImage` fallback dereferenced a failed RGBA type
assertion for NRGBA/Gray inputs. Both `SetData` and `SetImage` also wrote Cairo
pixels without marking the surface dirty: a newly clear surface could remain
invisible when painted through a surface pattern.

- Transfers now follow Cairo's [external pixel access contract](https://www.cairographics.org/manual/cairo-cairo-surface-t.html): flush before accessing pixels, mark dirty after writing.
- RGBA and NRGBA write directly into aligned native-endian Cairo pixels.
  PNG's straight alpha is premultiplied exactly like Go's `Color.RGBA()`;
  all 256 color/alpha levels are regression-tested, not approximated with
  a visually similar alpha formula.
- Nonzero image origins, subimage strides, cropped sources, preserved destination
  pixels/padding, ARGB32/RGB24/A8, empty writes and unsupported formats are tested.
- `Surface.Image` converts its owned readback copy in place. Returned images
  still do not alias Cairo's surface memory.

1024×768, Apple M2 Max, Go 1.26.5, Cairo 1.18.4, `GOMAXPROCS=2`, three benchmark
runs per case; approximate ranges, **not whole-application FPS or Windows CPU**:

| Operation | Before | After | Allocation per call |
| --- | --- | --- | --- |
| RGBA SetImage | 1.63–1.70 ms | 1.12–1.14 ms | 16,792,320 B / 38 allocs → 0 B / 0 allocs |
| RGBA Image readback | 1.82–1.96 ms | 1.17–1.21 ms | 19,938,112 B / 40 allocs → 3,145,792 B / 2 allocs |
| NRGBA SetImage | Original implementation panicked; correctness-first generic intermediate: 10.20–10.24 ms | 1.90 ms | Generic intermediate: 786,432 allocs → 0 allocs |

NRGBA's intermediate baseline is explicitly **not** a timing of the original
broken implementation. Large arbitrary custom `image.Image` implementations
still use the generic color-access path; no blanket zero-allocation promise.

## GLFW retained buffer and damage upload

- Pending damage is taken at frame start. An Update issued during Draw remains
  pending for the next frame instead of being cleared at frame completion.
- Physical-pixel clipping and texture upload share one outward-rounded rectangle
  with an antialiasing margin, including fractional/Retina scales.
- Texture storage is allocated only when dimensions change. PBO and direct
  fallback use `TexSubImage2D`; PBO packing copies only damaged rows.
- Resize/new texture, large damage, performance overlays and the previous GPU
  chart-overlay queue take the conservative full path. Partial traversal must
  not erase unchanged GPU charts after the fullscreen blit.
- Nonfinite damage requests are rejected; missing pixel data or GL upload errors
  preserve a full repaint request rather than silently dropping the frame.

Real hidden GLFW window testing on the OS main thread compares **every uploaded
texture pixel** against the Cairo backbuffer and independently verifies the
drawn colors. It covers full/partial uploads, the direct no-PBO fallback, GPU
overlay full-redraw protection, and Update-during-Draw followed by a settled frame.
The physical upload byte counts are printed by the native test.
On the local 160×120 framebuffer probe, the full upload is 76,800 bytes and
the chosen small damaged rectangle uploads 2,688 bytes. This is an upload-size
measurement for that test, not a 96.5% whole-application performance claim.

Legacy `Widget.Update` stays conservative/full to protect shadows and focus
halos. Partial upload benefits explicitly scoped `Window.MarkDirtyRect` callers;
it is not a claim that all IDE interactions now perform small uploads. Its
coordinates are **window-root logical pixels**, not a child's local Bounds.
The existing idle loop does not continuously paint a static window; the 47 ms
timer/hover heartbeat is a wakeup, not an idle redraw rate.

## Designer: existing free layout container

A selected free HBox/VBox/Grid/Form can now activate the **matching** geometry
manager in place, including a single child. Container identity, parentage and
bounds are retained. Locked subtrees, unrelated types and already managed
containers are rejected/no-ops. Undo restores descendant geometry after parent
layout/resize so nested free children are not left floating.

Four kinds are checked through canvas/runtime construction, Undo/Redo, persistence
and declarative construction. Grid/Form generated Go programs are actually
compiled and executed, including a subsequent runtime resize, for both wrapping
sibling widgets and in-place activation. Top-level form layout, layout-type
conversion, arbitrary slot reordering and size-policy editing remain gaps;
see [Designer gaps](DESIGNER_GAPS.md).

## Reproduction and evidence boundaries

```sh
GOMAXPROCS=2 go test -p 1 ./cairo ./paint ./gui ./graph ./ged ./decl ./cmd/silkide -count=1
SILK_TEST_NATIVE_RENDER=1 GOMAXPROCS=2 go test -p 1 ./gui -run '^TestGLFWNativeRenderTransfers$' -count=1 -v
GOMAXPROCS=2 go test -p 1 ./cairo -run '^TestSurface' -bench '^BenchmarkSurface' -benchmem -count=3
GOMAXPROCS=2 go test -race -p 1 ./cairo ./gui ./ged -run 'Test(Surface|FramebufferDamage|FrameDamage|CopyFramebufferRegion|SingleContainerLayout|LayoutSelectionRuntimeRoundTrip)' -count=1
GOMAXPROCS=2 go vet -p 1 -composites=false ./cairo ./paint ./gui ./graph ./ged ./decl ./cmd/silkide
git diff --check
```

- Current macOS evidence: native core regressions, targeted race/checkptr, vet
  and opt-in native GL pixel readback. The newer eleven-package UI/core invocation is
  recorded in `bin/qa-artifacts/windows-ui-20261011/`.
- Native Windows 10/Cairo compilation, eleven-package execution and interactive
  Session 1 desktop QA now run through the requested `win10testcomputer` skill.
  See [Windows UI QA](WINDOWS_UI_QA.md) for ownership, persistence/runtime,
  menu/drag workflows and bounded CPU/resource observations. Cross compilation
  is no longer the only Windows evidence. The CI workflow itself has not been
  run in this iteration.
- CI tests Cairo on all targets and native GL on macOS/Linux (Xvfb). Linux native
  tests have not freshly run here; configuring CI is not evidence it passed.
- Mac Computer Use access remained denied; there is no new Mac interactive
  acceptance claim. Actual Windows interactive screenshots are independent of
  the hidden GL test, which does not replace user-facing desktop QA.
- Startup profiles include startup and external/native calls, not clean
  steady-state CPU measurements. A Windows startup watchdog warning remains
  recorded rather than being described as solved.

## Native resource and font-cache follow-up

Windows repeated menus exposed growth in GDI resources, despite bounded native
window/USER counts. Native blit painters and borrowed-HDC surfaces now release
before EndPaint; resized/destroyed window backbuffers release deterministically.
Finalizers remain a fallback. Optional `paint.ClosePainter` / `CloseSurface`
helpers do not change custom Painter/Surface interfaces. Callers must exclusively
own a resource, retire painters before surfaces, and stop using closed aliases.
Tests cover synchronous retirement and idempotence, without forcing production GC.
Explicitly owned surface retirement calls Cairo Finish before Destroy. The
fallback finalizer only drops its reference: other Cairo aliases may outlive
the Go wrapper, so finishing in that finalizer would be unsafe.

Scaled fonts no longer use widget-position offsets as cache keys. Cairo explicitly
[ignores CTM translation for scaled fonts](https://www.cairographics.org/manual/cairo-cairo-scaled-font-t.html#cairo-scaled-font-get-ctm).
Scale, shear, rotation, size and face remain distinct; a 1,000-position regression
checks cache cardinality. This is not a general bounded-LRU guarantee for every
font size/transform.

A native Windows test also exposed undefined alignment padding in `cairo_glyph_t`:
Windows uses 32-bit unsigned long for the glyph index. The Go-copy padding field
is normalized in both shaping paths; glyph index and x/y positions are unchanged.
The 2026-10-11 follow-up still counted 72/116/164 GDI objects
after 10/20/30 menu cycles, with USER fixed at 23. In a separate QA-only diagnostic
build, counts 126/220/304 did not fall after forced GC; Go wrappers and font-key
counts stayed constant. Instrumentation was removed; production does not force
GC. Those lifetime/cache fixes alone did not establish a native backend leak fix.

The 2026-10-20 follow-up isolated the actual SDK `EndPaint` wrapper calling
`BeginPaint` again. Correcting that entry point changes the native 100-paint
probe from GDI 10→210 to 0→0, with no PAINTSTRUCT mutation. In the real IDE,
120 menu cycles now stay at GDI=18/USER=23, instead of the matched release
baseline's GDI 156→684 samples. Window-generated icon ownership and two other
misrouted SDK forwarders are fixed separately. See
[new native evidence and limits](WINDOWS_RENDERING_QA_20261020.md); bounded
resource stability is not a complete CPU, latency or platform certification.

## Windows editor font hot path

The default editor formerly requested Menlo on every platform. On the native
Windows test machine its proportional fallback failed the real mono-width check,
so ASCII token measurement went through native TextExtents instead of the existing
fast path. Defaults now use Consolas on Windows, Menlo on macOS and the generic
monospace family elsewhere. An unskipped native Windows assertion verifies actual
equal advances; arbitrary SetFont overrides remain metric-checked.

Windows 10, Xeon E5-2686 v4 2.30 GHz VM, Go 1.26.5/Cairo 1.18.4,
GOMAXPROCS=2, the same 48-character ASCII line, three runs per case:

| Requested font | Actual mono check | Measurement | Allocation |
| --- | --- | --- | --- |
| Menlo (previous fallback) | false | 4,961–5,549 ns/op | 160 B / 2 allocs |
| Consolas | true | 77.37–96.54 ns/op | 0 B / 0 allocs |

This measures only `CodeEditor.measureText`, not whole-document rendering,
typing p95 or whole-application CPU/FPS. Tabs, non-ASCII and proportional fonts
retain native measurement. The benchmark source/log is retained in the ignored
Windows QA artifacts; the temporary benchmark file was removed from the test
machine before final regression. The three previous skipped measurement tests
now pass on Windows. See Windows UI QA for actual input/save/build readback.

Reproducible local logs are kept separately from source under the ignored
`bin/qa-artifacts/iteration-20261011/` directory. Original untracked `gen_image/`
and root `silkide` artifacts are unchanged; the installed `bin/SilkIDE.app`
has not been replaced by this iteration.

The earlier rendering-phase isolated Mac test bundle is
`bin/qa-artifacts/iteration-20261011/after/SilkIDE.app`. It builds successfully
and passes ad-hoc signature verification after resealing the bundle. Its signed
executable SHA-256 is
`2d999efbf7fb00a99bed7395041ed808fab50045fb1a05cd15212d992a8a5a8a`.
That earlier bundle predates the Windows UI/menu follow-up. Bundle signing does not establish interactive desktop correctness.
