# Inspector geometry, allocation and popup retirement — 2026-11-17

Status: final frozen platform, native and actual-consumer gates accepted. The
superseded Windows failure below is retained, not counted as a pass. The
continuing Qt Creator-style/industrial product goal remains active.
No binary release/tag or whole-product certification is implied.

## Narrow failures and changes

Baseline: `568a5505c9361ad6acbffb01d77d014a407f94c1`.

- A 100-field ordinary scroll allocated about 500 objects/event, mostly repeated
  height-configuration parsing and child-list snapshots. Negative tests preserve
  the allocation failure separately from benchmark calibration effects.
- A custom editor's synchronous geometry callback changed a row to 53px, but an
  outer stale bounds write restored 26px while the following row advanced 53px.
  Nonfinite configured height also escaped into control/scroll-range geometry.
- The inspector now commits row-height snapshots and reuses them for positioning,
  visible labels and focus reveal. Public height setters relayout; raw resource
  edits require explicit Layout. Retired metadata cannot mutate a new binding.
- Layout and positioning each permit two corrective passes, then yield through
  the existing bounded UI layout queue. Binding revisions abandon stale work
  after host callbacks; no new idle timer, polling worker or background UI writes.
- `gui.DismissInlinePopups` snapshots only inline HideSubPopup owners, then checks
  current ancestry before callbacks. WtForm/WtPopup remain native-owner boundaries
  even before window creation; WtChild stays inline. Reentrant calls share an
  at-most-once token per owner, reset safely after panic. Common snapshots use
  16 stack slots; unusually large custom composites may allocate spill storage.
  This is not a detach/reattach history-epoch or arbitrary-popup destruction API.

Code: `prop/property.go`, `prop/scroll.go`, `gui/popup_subtree.go`, `gui/widget.go`.
Regression and reproducible benchmark fixtures: `prop/scroll_perf_test.go`,
`gui/popup_subtree_test.go`. Configuration, resize/filter, rebind/scroll callback,
reentry budget, panic recovery, ownership transfer/native boundaries and spill
paths have dedicated controls.

## Windows full-suite failure was not ignored

The first candidate's Windows full test exited 1 at
`TestRuntimePointObserverHeartbeatStaleAndHTTPOwnership`: `stale quality missing`.
The 1ms DI11 point could already become uncertain during NewRuntime's final Tick;
the test then replayed that uncertain sample as a heartbeat and expected a new
stale transition. No production scheduler/UI change explains that assumption.

A derived negative fixture explicitly ages the point before subscription and
reproduces the failure on macOS too. The test now uses an explicit sample timestamp,
checks both fresh and already-stale/recovered starts, exact stale-boundary behavior
and repeated-tick notification suppression. It does not add sleeps, retry until
success, relax assertions or change production point semantics. A 30-count race
preflight passes. All final platform suites are rerun against the final candidate;
the superseded first candidate is retained, not accepted as the final result.

## Frozen final candidate and gates

Final source archive SHA-256:
`300142edf8e250eb7e9dad036a3b893c2c87e5d1af6d2e37e38d7ab0c2260853`.

The manifest contains 2,859 source/resource/doc paths, including 2,090 Go files.
Tests run from isolated source trees, not the root ignored legacy Package tree.
Go 1.26.10; actual correct-architecture bundled gopls is required, never a silent
system-gopls fallback or skipped substitute.

| Platform | Full named passes / skips | Race named passes / skips | Strict actual gopls |
| --- | ---: | ---: | ---: |
| macOS arm64 | 9,648 / 29 | 9,648 / 29 | 4 pass, 0 skip |
| Linux arm64 + Xvfb | 9,587 / 37 | 9,587 / 37 | 4 pass, 0 skip |
| Windows 10 amd64 | 9,445 / 43 | 9,445 / 43 | 4 pass, 0 skip |

All three platforms full/race and strict LSP exit 0. Platform/build-tag/native-only skips are counted, not
misreported as full coverage. Scoped vet and all 171 renderer-subset named cases pass, without skips. Full default
`go vet ./...` still exits 1 with the same 501 baseline diagnostics: nine source
locations moved because lines were inserted; each maps to an unchanged source line
and identical diagnostic. No added/removed diagnostic is hidden by normalization.

Independent Linux and Windows post-test exports match all 2,859 files without
extra Go files. The local frozen tree matches too. Final source/index/media audit
compares every tested source/module byte with the Git index and working tree.
Final result documents/images are explicit reviewed exceptions to pre-test docs;
all 2,096 Go/module paths match the tested source exactly before commit.
Three independent source trees match the manifest; the three new README images
are byte-identical to their original accepted captures. The sole unstaged legacy
non-code index/worktree normalization is `core/callback.go.txt` (CRLF/LF only); it
is not modified or included in this commit.

## Native evidence and actual generated consumer

Final macOS IDE and SDK hosts are rebuilt from the final frozen SDK in fresh,
non-relaunching isolated profiles. Old first-candidate screenshots are not reused.

- IDE: actual editor wheel input, reverse Tab reveals an offscreen X field, Title
  filtering, title edit/save, independent current-SDK reads after saved Undo and
  Redo, New to untitled, and Open with the current profile's selected file URL.
  Exact document and session readback preserve Checkable=true, Checked=false,
  the nested values and typed OnToggled mapping. Normal process exit is observed
  as 0; owned IDE and adjacent bundled gopls processes are gone.
- The actual newly saved `group.silk.go` companion is copied byte-for-byte into an
  isolated consumer. Module identity points to the final SDK; its race runtime test
  verifies nested values, disabled child gates and restoration with exactly one
  GroupBox callback. This is execution, not only generated-code compilation.
- SDK host: a real color strip reports visible/capture true. Public sheet scroll
  closes it and releases capture. The next single native button click increments
  the real Action handler from 0 to 1. A custom OnMove changes row height to 53px;
  row and next-row delta both end at 53, callback count 1, model/history unchanged.
  QA-only loopback calls are marshalled with gui.Post, not mislabelled user wheel
  input. The native button input is separate. Normal exit 0 is observed.
- Small-fixture Mac idle sample over 10.015s: IDE 0.13 CPU seconds (~1.30% of one
  core); both actual bundled-gopls processes 0.00 at ps resolution. This is process
  CPU only, not a startup/GPU/FPS/large-workspace guarantee.
- Windows 10 native IDE: actual editor wheel down/up, first/last Tab reveal,
  scrolled category collapse, filter/reset, title edit with separate saved Undo
  and Redo reads, New/file-tree current-document reopen, scrollbar dragging,
  exact document/session readback and held-process-handle normal exit 0 pass.
  Owned IDE and both actual adjacent gopls processes are gone. Its quiet 10.015s
  sample used 0.171875 IDE CPU seconds (~1.72% of one core); both gopls processes
  used zero at process-time resolution and all sampled processes responded.
- Windows SDK host also passes the actual palette/capture/first-click/53px flow.
  The retired native screenshot is reviewed before admitting exactly one button
  click; current PID/handle/foreground/physical client coordinates are checked.
  There is no focus or activation repair between retirement and that click.
  Values, color and history remain unchanged; normal held-handle exit is 0.
- Windows actual new companion executes with race against the final SDK, with
  byte-identical copied source and the same nested-state/runtime restoration
  checks. Its held native process exits 0. Post-native source verification again
  matches all manifest files without extra Go and with zero owned native/gopls
  processes, including the additional SDK host.

Original README images are unretouched native captures, not image-model mockups.
The SDK palette-open capture can include a larger popup surface than its parent;
its composite capture scaling is not evidence that all native DPI/first-frame
rendering defects are solved. Post-retirement state and first-click behavior are
verified separately.

## Repeatable geometry benchmark, not FPS

Same setup and loop on baseline/final source, Apple M2 Max/macOS arm64, three
500ms samples. Initial control construction and Clear are outside the timer.
The baseline benchmark-only copy is a derived, non-acceptance tree; no extra Go
benchmark helpers enter the SDK's final source exports.

| Fields | Baseline median ns/op | Final median ns/op | Baseline allocs/op | Final B/op / allocs/op |
| --- | ---: | ---: | ---: | ---: |
| 100 | 16,399 | 3,905 | 499 | 0 / 0 |
| 1,000 | 161,283 | 39,139 | 4,998 | 0 / 0 |

This is roughly 4.1–4.2x faster geometry traversal for this fixture, not the whole
IDE. Timing varies with machine load. An additional busy-machine run is retained.
The allocation regression warms the sheet and asserts exactly zero steady-state
objects. Whole-process memory profiling includes setup, text shaping and a few
warmup/profile allocations; it is not claimed to be zero-allocation startup.

Replay after setting up platform Cairo:

```sh
go test -count=1 ./gui ./prop ./hmi
go test -race -count=1 ./gui ./prop ./hmi
go test ./prop -run '^$' -bench '^BenchmarkInspectorScroll$' -benchmem -count=3
```

Detailed local evidence: `bin/qa-artifacts/inspector-geometry-final2-20261117/`.
Negative/superseded evidence: `bin/qa-artifacts/inspector-geometry-20261117/`.
Raw archives, profiles, tasks and binaries stay out of Git.

## Remaining product work

This increment is not full widget virtualization, industrial point/HTTP pipeline
completion, alarm/audit certification, arbitrary popup/menu lifecycle coverage,
Windows per-monitor DPI/IME completion, first-frame damage closure or Qt Creator
feature/performance parity. Continue those as separately reproducible flows; do
not trade the evidence boundaries above for a blanket "all fixed" claim.
