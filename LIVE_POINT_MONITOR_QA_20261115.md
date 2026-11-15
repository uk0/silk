# Industrial live points, explicit writes and history — 2026-11-15

Baseline: `275e1c8a920cc1807701d10700c076b044132318`.
This increment adds a reusable monitoring/control workflow, not an alarm engine,
PLC command protocol, access-control system or certified safety interlock.

## SDK and data ownership

- `core.Tag.Snapshot()` atomically reads sample plus tag-local revision. Silent
  `Observe` heartbeats and stale-quality changes advance revisions; unchanged
  `Publish` remains a no-op and no acquisition timer is added.
- `hmi.Runtime.PointState/PointStates` read live memory only, not SQL or the
  managed-writer lock. `PointToken` is opaque and runtime/point-local; it is
  excluded from JSON and cannot survive a runtime restart.
- `WritePoint` checks the token at the start of a serialized runtime-managed
  write, enforces the point type and HTTP ownership, persists before notifying
  live bindings and never retries automatically. Direct mutation of `Tags`
  bypasses managed ownership/persistence and is not a competing-writer API.
- Cancellation is not a rollback guarantee. A failed/cancelled UI attempt is
  one-shot; verify current state/history before any explicit retry.
- `point.HistoryReader` and `SampleRecord` decouple optional history from SQLite.
  Existing `sqlite.StoredSample` / `History` APIs keep their identity and order.
  Recent results are newest-first, with insertion-order ties; range results stay
  oldest-first. Runtime wrappers enforce limits, known point, sample type/quality,
  time, ordering and requested range instead of trusting a custom reader blindly.
- Restored samples are validated before entering tags, including comparable
  typed payloads and reversible timestamp bounds.

## GUI and lifecycle contract

- `gui.ProcessPointMonitor` starts no acquisition or database on construction,
  designer drop, preview or `SetRuntime`. The host owns the runtime. Explicit
  Write/History actions use a two-job bounded executor and cancellable contexts.
- Changes coalesce by point behind one queued UI delivery; reads of unchanged
  acquisition heartbeats do not repeatedly wake the UI. Refresh explicitly reads
  their latest revision/time. This latest-state view is not a lossless alarm log.
- Selection survives live sort updates; int64 values are not sorted through
  float64. Displayed strings are bounded and rune-safe, with UTC timestamps and
  textual quality as well as a theme-aware status marker. Oversized inline values
  are rejected, never silently truncated into a write.
- Float cells use 12 significant display digits to suppress scale-operation
  noise; raw snapshots, integer precision, sorting and write baselines remain
  exact. The inline write editor shows the complete current scalar, not a
  truncated/rounded display cell.
- Read-only cells cannot directly write. The explicit owned dialog rechecks the
  selected point and requires a confirmation action. Invalid/HTTP-owned/stale
  attempts cannot mutate storage; a failed attempt cannot implicitly retry.
- Detach, runtime replacement and Close cancel outstanding work, discard queued
  old updates and close the owned dialog. `Wait(ctx)` belongs after the event loop
  or on a worker, never in a UI callback. The widget does not stop the host runtime.
- `hmiapp.Config.RuntimeChanged` is a UI-thread lifecycle callback; it makes a
  borrowed runtime available to drivers without polling and revokes it on stop.
  `Workbench.Runtime()` returns nil during startup/stopping/close. Hosts must
  detach views and cancel their own drivers before releasing a runtime/store.
- The workbench's Live points toggle is inert until Run. Live mode hides the
  disabled authoring toolbar, preventing it from covering a small live viewport.
- A final lifecycle audit reproduced a panel-toggle bug: a session could retain
  its runtime pointer while not yet available or already stopping. The toggle
  now uses the public availability-checked `Runtime()` seam, never that pointer.
  A deterministic regression fails before the fix and passes afterward, including
  hide/show after Stop while the old session still exists. The first candidate's
  passing suites/native results are retained but are not the final source gate;
  the corrected candidate is independently frozen and rerun.

## Automated regression

Targeted SDK/GUI/workbench/designer race cases pass, including atomic silent
observations, stale/foreign/ABA tokens, competing writes, blocked persistence,
independent SQLite reopen, history order/limits, burst coalescing, real modal
actions, cancel/late completion, precision, generated Go and compact layout.

Frozen-source runs use `-count=1 -p 2 -timeout=20m ./...`, with and without
`-race`. Counts below are named test/subtest events, not package counts. Skips
remain skips; strict real bundled-gopls probes are run separately.

| Platform | Ordinary pass / skip | Race pass / skip | Strict real gopls |
| --- | ---: | ---: | ---: |
| macOS arm64 | 8,842 / 29 | 8,842 / 29 | 4 passed |
| Linux arm64, Xvfb | 8,781 / 37 | 8,781 / 37 | 4 passed |
| Windows amd64 | 8,632 / 42 | 8,632 / 42 | 4 passed |

All three platforms' full, race, LSP and runner exit markers are zero. All 2,695
frozen manifest files match each post-run source tree/hash readback, with no
extra Go source files. The 12 strict real-gopls probes have no skips.
The final pure-renderer-tag core/point/runtime/workbench/designer race suites and
focused Process GUI race cases also pass. An earlier candidate's full pure-tag
GUI race suite passes in 569 seconds; it is not a native UI latency measurement.
The 1 MiB-string cell-format microbenchmark uses 528 B / 2 allocations per call,
not a copy of the full payload; this is not a whole-table or frame-rate benchmark.

## Windows native desktop and persistence

The sequence below passes again on the corrected availability-guard candidate,
with a fresh process, HTTP fixture and database. Normal-exit and independent
read-only database verification are both repeated, not inherited from the first
candidate. The final Windows full/race/strict-LSP gates also pass separately.

The Session 1 native fixture is a separate SDK consumer using the unchanged
frozen production source, `hmiapp.Workbench`, four typed points, an isolated
SQLite database and its own loopback HTTP server. It adds no production debug
route or instrumentation. Actual mouse and keyboard input drives the dialogs.

1. Launch and toggle Live points before Run: no runtime, HTTP requests or DB.
2. Run: POST the exact configured JSON body, extract `/data/pressure`, scale by
   `x / 10`, and display 82.6 bar. HTTP acquisition owns AI11; its write action
   is disabled and cannot open a write dialog.
3. Confirm DI11=true through the actual edit/confirm controls. Cancel=false and
   invalid boolean input do not publish or persist.
4. Open a write baseline, publish a concurrent driver change, then confirm the
   old baseline. The dialog stays open with a conflict and disabled write action;
   another click cannot retry or overwrite the concurrent value.
5. Write BI11=`9223372036854775807` through native input. The monitor and SQL
   retain the exact integer. Existing float-based process cards intentionally
   reject integers outside their safe float range and render a dash; this is
   not an exact-int64 numeric-card implementation.
6. Controlled HTTP 503 changes AI11 to Bad and repaints the selected marker red.
   Recovery changes it to Good / 77.5 bar and repaints the conditional card and
   marker green. The monitor retains the last numeric sample alongside Bad
   quality; the process card shows a dash, not a falsely good value.
7. Actual Recent history action opens an owned read-only dialog with 200 newest
   observations, including failure/recovery. Close returns to the workbench.
8. Stop detaches the panel and stops HTTP; successive settled request counters
   remain equal. Run restores the persisted manual points. Every captured
   state keeps the authored document hash unchanged.
9. WM_CLOSE exits normally with process code 0 and `HMI_QA_NORMAL_EXIT`.
   Independently reopen the closed database **read-only**: `integrity_check=ok`,
   four definitions, exact int64 latest value, recovered HTTP value/quality and
   exactly three manual history entries: DI11=true, DI11=false (driver), BI11=max.
   Cancel, invalid input and stale-token confirmation create no extra entries.

Native production `silkhmi` build and SDK-consumer build both succeed. Replay
scripts, request/state JSON, process markers and database are in the ignored QA
directory. The external fixture logs a nonfatal missing optional `icon` lookup;
normal exit is confirmed, but this is not application-package branding acceptance.
README images are copied byte-for-byte from the corrected candidate's captures:

- `docs/images/hmi-live-points-windows.png` — recovered binding and live monitor.
- `docs/images/hmi-live-point-history-windows.png` — explicit bounded history.

## macOS native observation boundary

These observation attempts used the first candidate, not final native acceptance.
The first wrapper-launched fixture did not reach main; a process sample stayed
in the dynamic loader before any project/DB/HTTP action. A fresh direct Mach-O
app bundle in `/tmp` reaches the UI event loop and its independent memory-only
snapshot confirms the inert state (no DB or HTTP). However CUA cannot observe
that window: ScreenCaptureKit reports `-3811` audio/video capture failure; the
earlier app binding also timed out. No OS security/capture settings were changed.
Do not treat this as macOS mouse/keyboard/IME acceptance. The macOS automated
native-backend full/race and real-gopls results above are separate evidence.

## Source identity and remaining product work

Baseline: `275e1c8`. Corrected frozen archive: 2,695 files / 1,975 Go files.

```text
final-source.tar.gz SHA256
ee86d10114d905b31660656198caf02cf3f699c879ae56786543b1e4c844c7b9
Windows production silkhmi SHA256
ffbdb7ffbbe171b7d43687987b950d830111d9b842fdb1a6a582d6a34de85658
Windows external SDK consumer SHA256
9e737ee42f881b2f4f44d77d6e929a9387ebaf627670808df8f8a228d0cfa4c0
```

The staged tree contains 2,695 files. All 1,997 Go/native/build/resource inputs
match the corrected frozen source byte-for-byte. Post-freeze changes are limited
to QA/workflow/plan/SDK/changelog documentation and native gallery images. The known
noncompiled `core/callback.go.txt` worktree CRLF versus Git LF normalization is
checked explicitly; there is no exception for Go/native/embedded inputs.

Intermediate bootstrap/probe failures are retained in the ignored artifacts and
are not counted as successful tests. The availability regression is fixed and
rerun; a hypothetical oversized legacy SQLite sample issue was independently
falsified because the existing store already rejects it before committing.

IDE-host live-runtime integration, alarm lifecycle, PLC protocols, authorization,
obstacle-aware routing, default SDK-consumer icon handling and full Qt Creator
parity remain separate roadmap work. No new prebuilt release/tag is claimed.

Raw artifacts and replay scripts: `bin/qa-artifacts/live-points-20261115/`
(corrected final candidate in `verified/`; ignored; generated Go stays outside
SDK source packages).
