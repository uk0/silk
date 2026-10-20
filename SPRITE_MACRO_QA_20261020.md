# Satin sprites, HMI and macro iteration — 2026-10-20 (PDT)

Development worktree based on `d17d09c617f33282ae8531d1426075791e155121`.
This iteration is not a new release and does not change the published v0.1.5
tag/package. It is not a claim of full Qt Creator parity.

## Changes and decisive checks

### Generated material assets and rendering

- Actually invoked the built-in image generation model with transparent output.
  The unchanged original is `paint/assets/silk-satin-v1.png`; prompt and measured
  region manifest are beside it. Sixteen symbols cover common IDE actions and
  HMI valve/tank/gauge/Gantt uses. This is not a hand-drawn placeholder sheet.
- The returned 1254 px image was not a perfect 4×4 grid. Equal-grid crops leaked
  neighboring artwork. Explicit object regions plus equal transparent padding
  fix that without modifying the original image. Generic grids use rational
  cell boundaries rather than rejecting non-divisible dimensions.
- Premultiplied RGBA / Catmull-Rom scaling; max eight cached device sizes per
  icon. The painter reuses native surfaces/patterns and retains their lifetime
  through drawing. Disabled raster icons use a neutral theme alpha mask, not
  backdrop-dependent HSL desaturation. Common actions can opt back to vectors
  with `SILK_ICON_STYLE=vector`.
- Tests compare all 16 icons at 16/20/24/32/48/64 device pixels, cache reuse and
  bounds, alpha, caller-source mutation, cell isolation and disabled RGB/path
  preservation. [Rendered light/dark scale sheet](screenshots/silk-satin-icon-scales.png).
- Idle non-menu buttons use a per-widget cached gradient and border. Contrast
  tests check both gradient endpoints; hover/pressed/focus behavior is retained.

Local Apple M2 Max native warm `DrawIcon` benchmark: **856.5 ns/op, 96 B/op,
2 allocs/op**. This is an icon microbenchmark, not total GUI cost and not a
zero-allocation claim. Warm painting does not decode PNG or resample the atlas.

### HMI components and event lifecycle

- `SpriteView`: boolean Tag-driven symbol, optional active symbol, parent-canvas
  rendering, no native child window or idle timer. Clearing `ActiveIconName`
  restores the configured normal symbol rather than a hard-coded valve.
- `ProcessValue`: engineering value/unit/precision, finite-value guards, range
  indicator and visual threshold; caches the large-value font/text. Very large
  finite ranges avoid overflow. Thresholds are not a PLC safety function.
- Gauge now exposes `TagName`. Designer tests use the real `GedView.OnDrop`
  callback, compare properties after save/load/preview, inspect generated
  constructors/bindings and run Go vet against generated forms.
- The first real Windows dashboard capture exposed two visual defects: the
  example did not enable button text, and Gauge combined a hard-coded white
  background with light theme-selected text in dark mode. Fixed the example's
  labels/window icon and Gauge's background, ticks, readout centering, duplicate
  endpoint labels, font reuse and own-bounds clipping. Pixel tests now cover
  Gauge as well as the new components. The range-test painter recorder needed
  Save/Restore/Clip support; those failed test runs were retained and rerun.
- Before the scheduling fix, a 1,000-value burst queued **1,001** UI callbacks;
  tests also reproduced animation continuing after unsubscribe. `BindTagUI`
  now queues at most one pending callback per visual binding, delivers the
  latest value, and invalidates cancelled work. Animated bindings retire their
  animation on the UI thread and suppress later ticks.
- This coalescing is only for visual state. Do not use it for lossless alarm or
  event recording. An already executing setter may finish during cancellation.
- The boolean bridge keeps its previous float32/int32/custom-value coercions;
  it does not narrow them to `core.Value.Bool` while changing delivery timing.

### Explicit compile-time macros

- `.silkgo` directives expand into ordinary Go through `silkgen -macro` and the
  IDE Tools menu / command palette. See [syntax and limits](macro/README.md).
- Tests compile and execute generated accessors with the real Go toolchain;
  check deterministic output, CRLF, inert directive-like strings/comments,
  malformed/oversized/nested/unknown expansions, build constraints and write
  guards. IDE tests cover document-mode guards and unsaved output preservation.
- Initial fuzzing found `;` accepted as a formatting fragment. Added complete
  `parser.ParseFile` validation, retained the failing seed under
  `macro/testdata/fuzz/FuzzExpand/`, and added incomplete-file regressions.
  Generated markers are recognized before the package clause even when gofmt
  moves build tags ahead of them; marker-looking literals do not count.
- After the fix, a 15-second fuzz run completed **1,134,850 executions** without
  failure. Bounded fuzzing is not a proof for all inputs.
- `go generate ./examples/silk-macros` followed by `go run` produces
  `pressure=42.5 enabled=true`.

## Reproduction

```sh
go test -count=1 ./paint ./gui ./ged ./scada ./macro ./cmd/silkgen ./cmd/silkide
go test -race -count=1 ./paint ./gui ./ged ./scada ./macro ./cmd/silkgen ./cmd/silkide
go test -short -count=1 ./core ./geom ./graph ./prop ./decl ./printing ./internal/silkgen ./assets/branding ./ide/taskrunner
go vet -composites=false ./paint ./gui ./ged ./scada ./macro ./cmd/silkgen ./cmd/silkide
CGO_ENABLED=0 go test -count=1 -tags silk_pure_go ./cairo ./purecairo ./paint ./macro
go test -run '^$' -fuzz FuzzExpand -fuzztime=15s ./macro
go test -run '^$' -bench BenchmarkWarmSatinIcon -benchmem ./paint
go generate ./examples/silk-macros
go run ./examples/silk-macros
go run ./examples/satin-dashboard
```

The package regression, race groups, vet, optional pure-Go rendering group and
native macOS builds passed locally. Cairo alias package has no tests; the
purecairo/paint tests actually ran. CI/release workflows include macro tests and
SCADA/macro race coverage; edited YAML parses. No CI run or new release was
triggered by this worktree update.

## Windows desktop verification

Windows 10 22H2, real interactive **Session 1**, Go 1.26.5 / UCRT64 Cairo.
The isolated copy was `C:\silk\qa-20261020-sprite-macro`; 1,483 source/asset
SHA-256 entries matched before the final builds. Native race tests passed for
`paint/gui/ged/scada/macro/cmd/silkgen/cmd/silkide`. After the boolean-bridge and
Gauge follow-ups, affected packages were retested and both executables rebuilt.
GUI runs used native, non-race executables; race instrumentation was in the
separate test binaries, not the screenshot runs.

- IDE: clean child PATH (Windows directories only), all staged Cairo/runtime
  DLL origins checked. Actual Pinyin commit appeared exactly once on disk;
  supplementary Unicode saved/read back, cancelled preedit did not change the
  file, Save All preserved content, and an eight-second native chooser wait
  did not falsely trip the watchdog.
- Clicked **Tools → Expand Silk Macros** on a saved `.silkgo`; read back
  `machine_gen.go` with the generated setter. F7 executed a real project test
  producing 4,000 stdout/stderr lines and an on-disk completion marker while
  menus and twelve resizes were exercised. The IDE closed with exit code 0.
  [Final native macro editor capture](screenshots/silkide-silk-macros-windows.png)
  has debug outlines enabled. The same full IDE sequence was repeated after the
  final Gauge fix, not inferred from the earlier build.
- Final dashboard: verified inactive-to-active valve sprite, 42.5 → 95 alarm
  value/needle movement, dark → light → dark, and 95 → 42.5 normal state by
  inspecting actual native captures. All four buttons show their text; the
  Gauge background/ticks remain readable in both themes. The process remained
  responsive and closed with exit code 0.
- Final dashboard idle sample: **0.078125 CPU seconds over 12 seconds**
  (about **0.65% of one core**, not whole-machine CPU). This is a bounded sample,
  not a frame-rate/latency or all-hardware performance certification.
- [Dark normal state](screenshots/silk-satin-dashboard-windows.png) and
  [light alarm state](screenshots/silk-satin-dashboard-windows-light.png).

The connection briefly failed during the final screenshot upload/launch step;
the primary and backup SSH attempts and failed SCP log were retained. After the
connection recovered, the dashboard interactions were rerun and captured; the
network failure was not counted as a passed UI test.

Final tested native executable SHA-256:

```text
silkide.exe          54471a08d52e7cc9032a135e6048e32c4961f87d0abd08f4ccfd02ba11582194
satin-dashboard.exe ea623c2ed0ebb0b1a06645f2e6327d084b412e132ae558c564eef65df01383bd
```

## Evidence and remaining limits

Local logs/scripts/manifests live in the ignored directory
`bin/qa-artifacts/iteration-20261020-sprites/`. Earlier failed component, Tag
lifecycle and fuzz logs are retained rather than rewritten as first-run passes.
The native test copy excludes unrelated `gen_image/`, the root `silkide` binary
and `local/`. Remote source SHA-256 verification is required before acceptance.

- No Windows per-monitor/system-DPI awareness change in this iteration. Device
  raster tests do not fix OS-level bitmap virtualization.
- macOS/Linux native UI not retested in this iteration. A macOS Computer Use
  attempt failed to start its native pipe; Cairo offscreen HMI drawing was
  inspected instead, not labeled as a native desktop test. Pure-Go tests are
  backend tests, not a full native Linux desktop certification.
- Macro expansion is textual, non-hygienic and non-recursive. No macro source
  maps, cross-file import, macro-aware LSP or automatic build-time regeneration.
  Go type checking still belongs to `go build` / `go test`.
- No real PLC, printer or historian/alarm end-to-end test was performed by the
  hardware-free dashboard example.
- Generated artwork colors are not a promise of exact brand or semantic RGB;
  theme masks and structural vectors retain deterministic colors.
