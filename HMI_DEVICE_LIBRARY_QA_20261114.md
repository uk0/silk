# Reusable device library — 2026-11-14

Increment after `30b68cd`; the continuing product plan remains active. These are
schematic reusable subgraphs, not complete Qt Creator parity, linked prefabs,
PLC drivers, certified P&ID symbols or a safety-rated SCADA control system.

## Implemented and directly exercised

- Pure-Go immutable selection capture, internal pipes/bends and referenced
  definitions; deterministic placement IDs, independent point prefixes and
  explicit compatible point reuse. Node/property/pipe tags are remapped together.
  Source ownership, initial values and stale timeout of shared host points stay
  unchanged. Invalid candidates never partially publish.
- Searchable owned library with inert preview, responsive fields, point mapping,
  selection capture and one-step insertion Undo/Redo. Sorting mapping rows does
  not change their point identity. Close/read-only/runtime transitions retire
  nested library dialogs and owned workers.
- Strict versioned catalogs: 64 templates / 4 MiB, exact large integer initial
  values, unknown/trailing JSON rejection, bounded regular-file load, explicit
  overwrite consent and atomic no-replace publication. Stale async completions
  cannot replace newer catalog state.
- `.silkui` property persistence and preview. A new external consumer test first
  exposed the missing **concrete Go generator catalog setter**; it was fixed,
  not bypassed. Concrete, scene-declarative and silkgen-generated constructors
  now compile and run, retaining the custom catalog exactly.
- The no-CGO example writes and independently reopens 13 nodes / 5 pipes /
  8 points / 4 templates, with no acquisition sources; repeat runs refuse
  overwrite. Files are independent publications, not a multi-file transaction.

## Exact tested source

Frozen archive SHA-256:
`6460c669d9d2c0109ab6dff13478b6d1686f0a04424eda4080cba972f69aed61`.
The 1,947 Go files plus C/Objective-C/header/assembly sources are compared with
final working/staged bytes before commit (1,958 source files in total).
Docs/screenshots are finalized afterward. Raw local evidence, including earlier
failed code-generation tests and native harness attempts, remains under
`bin/qa-artifacts/hmi-device-library-final-20261114/`; retired attempts remain
separate in `bin/qa-artifacts/hmi-device-library-20261114/`.

```sh
go test -json -count=1 -p 2 -timeout=20m ./...
go test -race -json -count=1 -p 2 -timeout=20m ./...
CGO_ENABLED=0 go test -count=1 ./hmi ./point ./examples/processlibrary
SILK_DEBUG=1 go test -race -count=1 ./gui \
  -run 'TestTableScrollOverlay|TestFileDialogExtension|TestProcessDevice|TestProcessEditorComposite'
SILK_DEBUG=1 go test -tags silk_pure_go -race -count=1 ./gui ./ged \
  -run 'TestTableScrollOverlay|TestFileDialogExtension|TestProcessDevice|TestProcessEditorComposite'
```

Named subtests are counted; skips are not passes. Platform/native results below
are filled only after the corresponding final candidate actually finishes.

| Platform | Full ordinary suite | Full race suite |
|---|---|---|
| macOS arm64 | 8,657 pass / 28 skip, exit 0 | 8,657 pass / 28 skip, exit 0 |
| Debian 12 arm64, Xvfb/software | 8,596 pass / 36 skip, exit 0 | 8,596 pass / 36 skip, exit 0 |
| Windows 10 amd64 | 8,437 pass / 51 skip, exit 0 | 8,437 pass / 51 skip, exit 0 |

Strict bundled real-gopls probes are rerun on this source separately from default
optional-tool skips: unsaved Unicode completion/formatting, renamed unsaved
definitions, and type/definition/back/implementation navigation.
All three probes passed on each platform, no skips (nine total).

## Defects found by native screenshots and file readback

The first candidate is retired, even though its automated macOS/Linux suites
passed. Native evidence exposed two defects missed by those tests:

- Table's internal scrollbar-owner child inherited Widget's debug placeholder.
  It painted a translucent green fill/red cross **over already rendered cells**.
  The child now explicitly paints nothing; all Table-based editors benefit.
  A new raster regression fails against the retired code under `SILK_DEBUG=1`
  and passes against the fix, including the pure-Go painter.
- The legacy native Save dialog forced the `.silkui` filter/default extension,
  so Windows wrote `devices.silkhmilibrary.silkui`. The new extension-specific
  SDK chooser selects `.silkhmilibrary` correctly on Windows/Linux/macOS while
  retaining legacy form-dialog behavior. Strict extension validation rejects
  wildcard, separator, NUL and filter-injection input before native calls.

The final candidate is rebuilt and fully retested after both fixes.

## Windows native UI

The final **race-enabled, debug-enabled** native workbench passed, normal exit 0.
Executable SHA-256:
`91a075c5193b26906b4ff35a984197347517aadd03da7b76790e718bfb21f33f`.
Admission checks require Session 1, the candidate executable and HWND owner PID;
assertions read saved files rather than trusting status messages.

The final flow proves:

1. Owned library and nested point-mapping modal disable/restore the correct
   parent; table cells remain readable in debug mode after the overlay fix.
2. Insert a complete pump station with `train1` node IDs and `Line1.` point
   prefix: independently saved document has 9 nodes / 4 pipes / 5 points.
3. One Undo plus Save restores the original bytes, and Redo plus Save restores
   the inserted bytes. No partial point/link transaction remains.
4. Capture the selected nine-node subgraph, save a four-template catalog through
   the real native chooser with the correct extension, then explicitly reopen.
   Catalog I/O leaves the authored project bytes unchanged.
5. Close normally with no race detector or heartbeat warning. A separate
   non-debug process also opens the saved document/library and exits normally.

Evidence SHA-256:

```text
baseline  2c5138963d183a3f01984dd72bd82b1771acd177cb304241b9e3b763ab80d73d
inserted  9d6b7868213479af6dbd19f5f55f735855a6fb34459ee1d0a9e442c884b2603c
catalog   7ae062a4596d6ce7ef7a4d99dbafdb5c49a693e7c78a36c925e282b84fbccab6
```

A fresh-process load followed by Save to a second catalog also reproduced the
exact catalog bytes (independent Python JSON/hash readback, four templates and
nine captured nodes). That repetition's **overall performance gate failed**:
one 5.882-second heartbeat delay/recovery appeared at startup, before its first
library action. It is retained as a failure, not silently reclassified as PASS.
No data race was logged; the app closed with code 0. Its subsequent five-second
inert-library sample used 0 CPU seconds and remained responsive.

The first non-debug quiet-library sample used 0.03125 CPU seconds over 5.06175
wall seconds (about 0.62% of one core), and stayed responsive. These brief
inert-library measurements are **not** active-load, whole-IDE or rendered-FPS
performance guarantees. The compiler-concurrent startup delay remains an open
investigation. A fresh post-suite replay, after this candidate's compiler/tests
finished, passed load/save byte-exact readback, responsiveness, normal exit 0
and the no-race/no-heartbeat-warning gate. Its five-second quiet-library sample
used 0.0625 CPU seconds over 5.0129 wall seconds (about 1.25% of one core),
with the same race-enabled binary; this is not a release-build benchmark.

A bounded three-start debug probe also ended normally for all three processes.
Window appearance took 6.42 s, 1.44 s and 1.20 s under concurrent test activity.
The first probe logged a heartbeat warning **after** the owned window had already
received Close, so shutdown/detector lifecycle is also a specific follow-up; it
does not establish that all earlier warnings share that cause. No root-cause fix
is claimed here. Two earlier startup-only harness attempts omitted foreground
admission and ended with CONTROL_C_EXIT; the corrected probe uses WM_CLOSE on
the PID-owned window rather than unverified global Alt+F4.

The first debug-enabled run inserted a complete 9-node / 4-pipe / 5-point
document. Its harness then failed an ASCII-prefix assertion: injected keystrokes
were composed by the active IME as `Line。` rather than intended `Line1.`.
The saved JSON revealed the actual prefix and the app stayed responsive. This
is not counted as a final pass. The owned process was closed normally (exit 0);
the corrected harness pastes exact text and starts in a separate fresh data root.

## Limits and remaining product work

- Built-in templates never start acquisition or clone HTTP sources/history.
  Placed content is ordinary editable nodes; catalogs require explicit files in
  the workbench/IDE. `.silkhmi` does not contain the catalog. `.silkui` can retain
  the separate Device library JSON widget property.
- Worker I/O cancellation is cooperative; local filesystem calls are not
  forcibly interruptible. No-replace save requires hard-link support. Explicit
  replacement is not a filesystem CAS, hostile-path-race guarantee or complete
  crash-durability proof. See the [SDK example](examples/processlibrary/README.md).
- Full automated macOS tests are not native authoring acceptance. The previous
  ScreenCaptureKit -3811 failure remains unresolved; no native macOS PASS is
  claimed. Windows first-frame capture and an earlier compiler-concurrent
  heartbeat delay also remain separate investigations, not declared fixed here.
- Obstacle-aware routing, full inspector/source selection coordination,
  integrated live runtime panels and an alarm lifecycle SDK remain on the
  [continuing plan](PRODUCT_ITERATION_PLAN.md).
