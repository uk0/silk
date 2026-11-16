# Width-dependent layout and readable diagnostics — verified development increment

Baseline: `66406e211999c1403d2c79ebc2deb515071b6cad`.
Status: **frozen-source three-platform suites and source-built native UI checks pass**.
The full SilkIDE/designer/editor/industrial SDK goal remains active. This is not
Qt Creator parity, a DPI/performance guarantee or a binary release.

## Plan and implemented boundary

1. Reproduce the native prompt's lost final punctuation through the actual
   Dialog -> VBox -> wrapped Label first-layout path, not merely inspect text.
2. Add UI-only `MeasureForWidth`, `ISizeHintsForWidth` and the existing
   `IHeightForWidth` adapter. No probe bounds, Resize, focus/native creation or I/O.
3. Share width-dependent measurement/install solvers in Label, VBox, HBox,
   GroupBox, FormLayout, GridLayout and FlexWrap. Round modal client size up,
   then measure body height at the width actually installed. Explicit heights
   and the existing 500px automatic modal cap remain authoritative.
4. Keep natural/allocated Label layouts in two bounded cache slots. Preserve
   derived SDK SizeHints policy, stretch and constraints; keep a derived natural
   query single-path rather than recursively duplicating all subtrees.
5. Let wrapped HBox children opt into horizontal shrinking without stealing
   fixed siblings/minima. Give flexible grid rows their measured minima before
   extra space; retain explicit tracks and spanning-cell requirements.
6. Reject invalid grid coordinates/tracks, bound spans and deduplicate moving a
   registered widget. Grid/form/group must not move a child reparented elsewhere.
7. Keep warning/error toast preview ends, so a long path cannot hide its actual
   failure reason. Add rune-safe/backend-cluster-aware `ElideTextMiddle`, retain
   full notification details, cache preview/font/extents and clear on dismissal.
   Informational/success toasts retain legacy end-elision; palette/owner/click
   behavior stays unchanged. Preview retains at most 128 runes from each end,
   with bounded prefix/suffix scanning rather than a full-message walk.
8. Freeze final inputs, run full/race/strict gopls on all three platforms,
   replay actual macOS/Windows prompt/error/close behavior, refresh real README
   captures, audit staged source identity, then authorized commit/push.

## Reproductions and bounded performance evidence

Raw evidence: `bin/qa-artifacts/layout-measure-20261116/`.

Core replay commands, from the frozen source with the platform's native Cairo
toolchain and an isolated `SILK_DATA_DIR`:

```sh
go test -json -count=1 -p 2 -timeout=20m ./...
go test -race -json -count=1 -p 2 -timeout=20m ./...
# Use the verified bundle for THIS platform/architecture, not a PATH fallback:
bash scripts/test-bundled-gopls.sh /path/to/bundle-root
```

Linux runs the same commands under `xvfb-run -a`. Windows uses its real amd64
Go/Cairo toolchain; the strict test executable is built next to the verified
`tools/gopls.exe`, with both `SILK_TEST_REQUIRE_GOPLS=1` and
`SILK_TEST_REQUIRE_BUNDLED_GOPLS=1`. Raw scripts preserve all exact flags and
environment. Named subtests count separately; skips are reported, not passes.
Tests alone do not substitute for the actual native UI flows below.

- Initial actual modal regression fails on baseline: 403.8px natural client width
  becomes 403 integer pixels; Label receives one 19.6px row but wraps into two,
  painting all of the quit sentence except `?`. A longer mixed Chinese/Latin
  message gets one row instead of six. Source hashes and failing log retained.
- Initial toast regression fails on baseline: the rendered preview ends in a
  partial path (`Save failed: atomic-write C:/long-pro…`), not the read-only cause.
  Retaining the full message internally alone did not make the cause visible.
- Current focused layout/compatibility/diagnostic regressions: 152 named pass /
  0 skip, exit 0 in `initial/ink-fixed.jsonl` (macOS). Covers first layout, actual
  drawn text, exact measured/install geometry, nested layouts, fixed/spanning
  tracks, moved children, derived hints, cache invalidation and notification
  lifecycle. This is focused coverage, not a full-repository or native-platform verdict.
- Earlier fixtures had an exact floating comparison at 160 vs 160.00000000000006,
  three array-vs-geom.Rect comparisons and a removed private helper still used
  by Gantt/scene dialog builders. Retain those failures; fix the fixtures using
  subpixel tolerance/actual Rect snapshots and retain the intrinsic helper.
  The first compile also retained an unused import. None is a passing gate.
- A separate fallback-ink regression also fails before its fix: single-line
  `A😀` reports 19.60px height but its optical baseline puts ink down to 20.30px.
  Label now includes fallback ascent/descent and clamps only an otherwise
  overflowing optical baseline. The corrected mixed Unicode case passes.
- Warm fake-painter toast probe: 107–108us, 10,416 B /166 allocations becomes
  56–63ns, 0 B /0 allocations. This isolates cached preparation, NOT pixels/FPS.
- Real Cairo 320x48 raster probe (100 iterations, three samples): the exact
  baseline Draw-body overlay takes 165–171us, 10,584 B /170 allocations; current
  Draw takes 51–54us, 172 B /4 allocations. Same benchmark and actual opaque
  pixel readback. It is a test-only old Draw-body comparison, not a rollback of
  the whole product or a Windows latency claim.
- A 32-level warm width query is about 2.7us for VBox and 49us for HBox, with
  zero allocations in these samples. Nested horizontal natural/allocation passes
  remain measurably more costly; do not advertise universal constant-time layout.
- Final frozen/platform/native acceptance is separate from these microbenchmarks.

## Frozen candidate and completed platform gates

`candidate1`: **2,811 files / 2,065 Go files**, archive SHA-256
`1306145f6971881204408792b83a15215468993e8f7dd8f35113c12a422be595`.

| Platform | Full named pass / skip | Race named pass / skip | Strict bundled gopls |
| --- | ---: | ---: | ---: |
| macOS arm64 | 9,457 / 29 | 9,457 / 29 | 4 / 0 skip |
| Linux arm64 | 9,396 / 37 | 9,396 / 37 | 4 / 0 skip |
| Windows amd64 | 9,245 / 43 | 9,245 / 43 | 4 / 0 skip |

All suites and their physical runners exit 0. Add 400 Mac focused race passes
with no skips. Independent Linux/Windows post-source exports match every
manifest file with no extra Go inputs; Mac pre/post checksums also match.
CI-scoped vet, release/platform/branding scripts, pure-Go paint and opt-in native
Mac GL transfer checks pass. Full unfiltered vet remains 501 unchanged baseline
Go diagnostics, no new/resolved; the baseline log is reused only after verifying
every HEAD Go/module file matches that previously tested source.
The single fresh Windows full/race/strict/native-build job is terminal 0.
Linux extracts the immutable archive rather than copying Mac's
live `.git` initialization. Every setup directory/script exists before launch.
Windows root/task: `C:\silk_layout_measure_c1_20261116`; never duplicate a job
because an observer times out. First submission observed absent root/task,
uploaded the pinned archive/manifest, and returned `SUBMITTED_ONCE`.

## Current-source macOS native acceptance

Two isolated, source-built actual SilkIDE runs use the identical C1 payload
`51eb60cbb16525af1007196406f37a68c5708da746364c408a754b12df370077`.
Private `/private/tmp` working/profile directories and once-only launch guards
avoid accidental app relaunches. This is not a startup-performance fix or a
standalone-package acceptance result.

- Read-only editor Save refuses without changing original disk bytes or losing
  dirty Unicode input. The visible toast retains `permission denied`; notification
  details retain the full message. Copy Message -> ordinary clipboard paste ->
  explicit save -> independent file read reproduces the exact 116-character error.
- Repair permission, save, Undo/Redo and saved/dirty transitions preserve the
  exact buffer and disk baseline. Quit Save All refuses read-only output without
  closing; Escape cancels; a permitted retry persists the pending Unicode text
  before native exit 0. Model/session/source and no remaining owned gopls verified.
- A fresh second profile opens two ~170-character filenames. The actual quit
  prompt paints both complete names over three rows each, both `.txt` endings
  and the final `?`, without overlapping its footer/buttons. Escape leaves disk
  unchanged. Subsequent Save All saves both exact Chinese buffers, persists both
  source tabs/active document/design session and exits normally 0.
- Preserve the independent readback fixture's first wrong `.ini` path failure;
  the final read uses the actual `.silkui` settings file and passes. No source
  fix or data deletion was needed. Raw roots: `candidate1/mac-native-evidence/`
  and `candidate1/mac-multiline-evidence/`.
- README adds original, byte-identical current-source captures. Older save
  screenshots stay explicitly labeled for their preceding increment.

## Current-source Windows native acceptance

Windows 10 Session 1, same C1 source, actual owned process/path/handle; every
coordinate step first inspected the fresh original screenshot. No app restart
or mock dialog was used. The native build SHA-256 is
`266cafc9fa6a25fdf78c4e36fd455ac62291603d6e94b86deb03b9b2813ab9af`.

- Drag the design button from (20,25) to (30,30). Read-only Save preserves its
  old hash/attribute and dirty model; retry saves the exact position. Undo/Redo
  retain history and correctly return dirty/clean. Independent model readback passes.
- Source Unicode paste/Save refuses both read-only output and a handle allowing
  direct writes but denying atomic replacement. Old bytes and input remain,
  no temporary file remains; explicit retry after handle release saves exact
  bytes. Undo becomes dirty, Redo clean, neither rewrites disk.
- The real Alt+F4 modal is **427x199**, not the old 426px width. The complete
  final question mark and default Save All are visible. A fresh reviewed C1
  script uses the default Enter action, not stale old coordinates. Read-only
  Save All refuses without closing or losing `QUIT pending` input; a second
  Alt+F4 -> Escape retains it. Explicit Undo returns to the saved baseline.
- The visible error toast retains `permission denied`. This is a readability
  check, not a new Windows frame-rate or per-monitor-DPI verdict.
- Actual normal Alt+F4 exit returns 0 through a process handle held before close.
  Independent source/model/session readback verifies design (30,30), exact
  source, one active source tab/edit mode and no temporary write files.
  The collected evidence also checks no owned bundled-gopls process remains.
- Evidence: `candidate1/windows-native-evidence/`; full/race/strict and post-source
  exports are separately retained. `three-platform-verified.json` aggregates raw
  JSON events/physical exits, not cached or earlier-candidate verdicts.

## Publication gate

Before the authorized commit/push, `final-index-audit.py` must verify every
staged runtime file against C1 and all three source exports, the exact changed
path set, executable modes, no extra Go source and byte-identical raw screenshot
origins. Only these reviewed result documents and three new README images may
differ from the frozen manifest. The unchanged `core/callback.go.txt` CRLF/LF
normalization is verified separately, never rewritten/staged as a source change.
Commit tree and remote HEAD must then match the audited index. No release/tag
or whole-product-completion claim is implied.

## Remaining work and limits

- Auto modal cap/explicit height can still require caller-owned scrollable
  content for very large messages. No general scrolling/reflow engine is claimed.
- Dynamic size-hint propagation through arbitrary custom containers remains a
  separate event-driven invalidation task; this increment fixes measurement and
  first explicit layout, not all update sources in the application.
- Windows source currently requests SYSTEM_AWARE (-2), not PMv2, and uses
  physical client coordinates. Do not flip awareness alone: per-monitor layout,
  input, popup, backbuffer and IME transitions need a coherent implementation.
- Broader designer/editor/industrial SDK, legacy synchronous Save As and project
  discovery work remain active. Prior source/native/release results retain their
  original scope; no new release is claimed here.
