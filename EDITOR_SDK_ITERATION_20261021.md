# Editor / explorer / SDK iteration — 2026-10-21

Baseline: `3a76b1c`. This is an active development checklist, not a release or a
claim that all requirements are complete. Unrelated `gen_image/`, root `silkide`
and restricted `local/` are outside the change set.

Latest verified status: [Windows Stage17 native acceptance](EDITOR_WINDOWS_QA_20261021.md)
and [macOS Stage16 desktop acceptance](docs/test-reports/macos-editor-chrome-2026-10-21.md).
The stage entries below retain historical failures and pending work; their old
"not yet verified" / "no commit" statements describe that stage, not the latest result.

## Acceptance checklist

- [x] SDK structured diagnostics: stable codes, severity, source location,
  remediation hint, wrapped cause, JSON representation, catalog and tests.
- [ ] Silk macro editing: lexical (not regex-only) directive/parameter highlighting,
  definitions/references and quick navigation, unsaved-buffer diagnostics,
  expansion error locations, generated-output safety and regression tests.
- [ ] Editor integration: discoverable commands, outline/completion/diagnostic
  presentation, stale-result retirement, SDK API coverage and examples.
- [ ] Left file explorer: bounded/non-blocking discovery, no symlink traversal
  cycles, selection/reveal/keyboard/filter workflows, safe filesystem mutations
  and open-editor rename/delete coordination, readable theme/font/icon rendering.
- [ ] Review previous editor/LSP lifecycle and error paths; wire actionable
  error codes into user-visible failure paths without losing underlying causes.
- [ ] Reproducible macOS tests plus real native desktop replay.
- [ ] Reproducible Windows 10 race/build tests plus real Session 1 desktop replay.
- [ ] Record commands, source/build identity, screenshots, failures and remaining
  limits; do not infer native UI correctness from unit tests/builds alone.

## Initial source review

- Macro generation exists, but `.silkgo` is currently treated as ordinary Go-like
  text: directives are comments, and there is no macro definition/reference index.
- Macro errors are free-form strings. Several source errors carry positions,
  while generated syntax errors currently identify only generated line numbers.
- FileExplorer recursively scans on the UI thread, follows directory symlinks
  via `os.Stat`, and invokes unbounded synchronous Git commands. It hides entire
  `docs` and `icon` directories. Rename/delete do not notify open editor sessions.
- Native QA must use isolated fixtures and settings; never run whole-tree tests
  that traverse restricted `local/`. Current graph was refreshed before review.

Additional review finding: existing LSP navigation often passes UTF-16 columns
into the shared rune-column navigation callback. Macro adapters now explicitly
use rune columns. The remaining LSP call sites need a separate regression and
conversion pass rather than silently changing every callback's contract.

Evidence and work in progress: `bin/qa-artifacts/editor-sdk-20261021/` (ignored).

## Stage 1 implementation and verified evidence

- Added SDK `diagnostic` catalog / copied snapshots / wrapped causes / source
  positions / JSON diagnostics. Macro generation and safe output writes use
  stable `SILK-MACxxx` codes. `silkgen -macro -diagnostics-json` exposes errors
  to automation; file/LSP historical paths are not yet fully migrated.
- Added `macro.Analyze`: shared lexical directive scanner, macro/parameter
  symbols, forward references and scoped parameter navigation, semantic spans,
  expansion diagnostics mapped back to original source/invocation. Physical
  source positions ignore spoofing by `//line`; embedded CR is retained for
  accurate offsets. Analysis reports the first expansion failure, not an
  exhaustive multi-error parser.
- Added revision-guarded SDK semantic highlight/outline APIs, preserving source
  bytes and long-line limits. SilkIDE consumes these via one bounded worker per
  macro buffer with 90 ms debounce, closed/stale-result guards and no GUI reads
  on the worker. F12, Shift+F12 and explicit macro-name completion use the
  unsaved buffer without gopls. Tests drive the actual completion accept/undo.
- LSP diagnostics now replace only their own document/source rows rather than
  wiping macro/build/design problems. Further LSP position/error-path review
  remains on the checklist.
- macOS arm64 native-Cairo race gate passed for `diagnostic`, `macro`, `core`,
  `paint`, `gui`, `ged`, `cmd/silkgen`, `cmd/silkide`, with real gopls on PATH.
  Final timings: 1.343 / 2.540 / 17.845 / 16.823 / 5.365 / 26.942 / 9.609 /
  32.675 seconds. Vet and IDE build passed. This is not native desktop QA.
- Macro analysis fuzzing passed a 10-second bounded, two-worker run (149,502
  executions); no generated corpus failure. This is not proof against every
  pathological input. The initial UI test compile failure (missing key-repeat
  argument) is retained, fixed and followed by passing race tests.
- Windows isolated QA root: `C:\silk\qa-20261021-editor-sdk`. Uploaded 1,586
  source/build input hashes were verified; GOMOD points at that QA root. Native
  race/build gate is still running; no Windows desktop pass is claimed yet.
- macOS executable started and initialized gopls, but computer-use startup
  failed and AppleScript reported assistive access denied (-1728). No native
  interaction pass. Also found that `SILK_DATA_DIR` does not isolate
  `settings.Default` on macOS: the first launch restored existing repo-file
  tabs. No editor actions were performed; the owned test process was terminated
  without the normal preference-saving UI exit. Next desktop run must isolate
  HOME/config as well and use an authorized automation surface.

### Replay commands

```sh
PATH="$PWD/bin/qa-artifacts/pdf-editor-lsp-20261020/tools:$PATH" \
  go test -race -count=1 ./diagnostic ./macro ./core ./paint ./gui ./ged ./cmd/silkgen ./cmd/silkide
go test ./macro -run '^$' -fuzz '^FuzzAnalyze$' -fuzztime=10s -parallel=2
go vet -composites=false ./diagnostic ./macro ./core ./paint ./gui ./ged ./cmd/silkgen ./cmd/silkide
```

Next: finish the running Windows gate and real macro desktop replay; implement
file-explorer scan/mutation improvements and regression tests; repair remaining
LSP position conversions and migrate error surfaces; complete the macOS desktop
gate when automation access is available. No commit/push in this iteration.

## Stage 2 work in progress

- Windows **stage 1** race/build finished (`STAGE1_NATIVE_BUILD_READY`, executable
  SHA256 `939f6a103f05ef0521622e4c0fb2bc9ef965aed6e6f061d885b9dc147aa50496`).
  Interactive Session 1 task `silk_macro_editor_qa` exited with result 0 and
  `MACRO_DESKTOP_FINISHED`. Saved/read-back tests covered F12 definition cursor,
  undo, completion acceptance/undo, unknown-macro diagnostics, explicit macro
  expansion and clean exit with no remaining gopls child. Screenshots inspected:
  highlighting/outline, completion + reference list, `SILK-MAC008` Problems.
  This evidence belongs to the stage 1 source manifest, not later explorer work.
- Explorer scans moved off the UI thread, lazily traverse expanded directories,
  use `os.Root`, do not follow child symlinks, bound entry count/depth/Git runtime
  and output, cancel old work and coalesce both requests and UI deliveries.
  Added filter, reveal, keyboard navigation, hidden-file policy, retained
  selection, explicit scan-error state and palette commands. `docs`/`icon` are
  no longer silently omitted. New behavior is asynchronous; SDK callers must
  observe `Loading()` / errors rather than assume SetRootDir finishes inline.
- File rows use theme/font preferences, measured elision, a reserved Git badge
  column, visible-row-only painting and a separate fixed-header clip. Row height
  is rounded up to logical pixels after a real-painter test exposed fractional
  clip bounds. Native explorer screenshots/replay are still pending.
- Git porcelain uses NUL paths and subtree-prefix remapping, preserving Unicode,
  embedded newlines, rename destinations and caller paths through root aliases.
  A live Git test exposed macOS `/var` versus `/private/var` badge mismatch;
  fixed and the focused race suite passed.
- File operations now emit typed errors and reject non-portable/reserved names.
  Rename uses atomic no-replace OS primitives on Windows/Linux/macOS; collision
  races and dangling symlinks cannot overwrite the destination. Unsupported
  platforms/filesystems fail closed. Open-editor rename/delete coordination and
  asynchronous mutation dispatch remain **unfinished**, not implicitly safe.
- Restored compilation, then caught a native Cocoa SIGTRAP when a background
  scan called Post before any native window existed. GLFW desktop probing alone
  is insufficient: wake-hook activation now waits for a successfully created
  native window. Added a fresh-process pre-window Post test (skipping TestMain's
  native probe window in that child), retained the failure log, and passed the
  gui/ged/silkide race gate. This is not a replacement for macOS desktop replay.
- Evidence: `explorer-compile-2.log` (original SIGTRAP), `explorer-focused.log`
  (path-alias failure), `explorer-focused-2.log` (pass), `explorer-race-3.log`
  (gui/ged/IDE pass), `fileops-focused.log` (pass), `macos-stage2-race.log`
  (fractional painter clip failure; subsequent complete gate still pending).

### Stage 2 completed gates (inputbox source snapshot)

The earlier stage-2 "pending" notes above are historical. The final inputbox
snapshot is `stage2/source-manifest-inputbox.json` (1,599 verified source/build
input hashes). It passed macOS nine-package race tests, vet and build; Windows
native changed-package race tests (`gui`, `ged`, `cmd/silkide`) passed in
13.930 / 180.533 / 95.909 seconds. Windows executable SHA256:
`5e74bb2c6fc740ddb346ae2c7a46fba6d8871f570c8eb4dc7493b473d299678e`.

Windows Session 1 explorer **v3** and macro tasks both returned result 0 with
`EXPLORER_DESKTOP_FINISHED` / `MACRO_DESKTOP_FINISHED`. Explorer tests verify
actual saved file contents and undo after reveal/Enter, filtered selection from
a different active editor, focused F5 refresh, and hidden-file filtering. F2
opens a preselected filename dialog; Esc preserves the filesystem. Both tasks
exit without leaving their gopls children. Macro replay covers exact F12 caret,
completion accept/undo, `SILK-MAC008` presentation, and generated Go readback.
Screenshots inspected: `v3-explorer-{filter,rename-dialog,final}-windows.png`,
`macro-{highlighting,completion,diagnostic}-windows.png`. Native Git badge
coverage is not claimed: this desktop replay intentionally uses a minimal PATH
without Git. Git behavior has separate real-Git model tests.

Failures retained and corrected, not relabeled as passes:

- Explorer v1 used a non-gofmt-canonical fixture; format-on-save legitimately
  changed it. v2/v3 fixtures are canonical and keep format-on-save enabled.
- v2 exposed a real modal lifetime defect: command callbacks ran before the
  palette's native modal loop unwound, leaving the parent disabled. Palette
  submission now stores the selection and executes it once after ShowModal.
- SDK InputBox now focuses/selects its edit on native activation, accepts Enter
  and participates in the shared Esc cancellation path. Model tests and the v3
  native filter/rename workflow cover the change.
- Extracting the palette builder moved `fillFinderList`; an existing structural
  test pinned the old function. The pin now follows `newCommandPalette`, with
  additional runtime-model tests retaining the initial-selection/refilter checks.

Remaining screenshot issues: selected file icons lack contrast on the blue
selection background; the outline header has a light hardcoded background and
nearly white text in dark mode. These are not considered visually complete.

## Stage 3 — Unicode navigation and language failure feedback

- Reproduced a real caret offset: a UTF-16 column 15 after two emoji was applied
  as rune column 15 instead of 13 (`stage3/lsp-navigation-before.log`). Added
  allocation-free `gui.RuneColumnFromUTF16` and UI-thread
  `CodeEditor.ScrollToUTF16Position`, documented in
  `gui/CODE_EDITOR_POSITIONS.md`. Negative/beyond-line/mid-surrogate, combining
  characters, invalid UTF-8, long lines, and unsaved buffers have regression tests.
- LSP definitions and workspace symbols preserve wire coordinates until the
  destination buffer opens. References retain indentation during conversion;
  diagnostics normalize only on the UI thread after session/version guards.
  The shared non-LSP navigation path remains rune-based. Compiler byte-column
  adapters and reference-row navigation after subsequent edits need further review.
- Workspace-symbol modal selection now navigates after native modal teardown;
  callbacks queued after close/server replacement are ignored. Per-query
  cancellation and bounded symbol-search scheduling remain to be improved.
- Unavailable explicit language actions carry `SILK-LSP001`. Definition,
  references, code actions, formatting and symbol-query failures use
  `SILK-LSP002`, retain the cause, and log a recovery hint. Normal cancellation
  is silent; actual timeout failures remain visible. This is partial migration,
  not a claim that every historical LSP/SDK error path has been converted.
- Coordinate-only macOS nine-package race/vet/build passed. The first full
  feedback gate caught an old assertion requiring an unprefixed error message;
  the updated assertion requires the exact stable prefix plus localized text
  and still verifies unchanged content/cursor and no local fallback. Failure
  logs are retained. The latest snapshot's Windows build/native replay is
  separate from the stage-2 passes above and must finish before release claims.
- macOS Computer Use was rechecked: `cua.getState()` still returns
  `Sky Computer Use native pipe startup failed`. No native desktop actions or
  pass are claimed. Native painter/GL readback and package tests remain distinct.

Open-editor rename/delete coordination, asynchronous mutations, remaining LSP
lifecycle/error surfaces, the visible theme issues and macOS desktop validation
are still required. The overall goal remains active; no commit/push this round.

### Stage 3 verified gates and a newly observed visual defect

The final stage-3 manifest has 1,606 inputs (`source-manifest-verified.json`).
macOS nine-package race tests, vet and build passed; Windows native `gui` and
`cmd/silkide` race tests passed in 15.100 / 96.696 seconds. Windows executable
SHA256: `beb42b4a3104da05f3a9f78a2003494ac60a5b43c72b0c35be51302f89a2526e`.
The Unicode desktop task returned result 0 with `UNICODE_DESKTOP_FINISHED`:
F12 and workspace-symbol selection both land exactly before `Target` following
two emoji and Chinese characters, verified by insert/save/readback/undo. The
symbol dialog relinquishes focus correctly and gopls exits with the app.

However, screenshot inspection exposed a **visual failure despite that
functional pass**: the symbol-results list was only **114x10** logical pixels.
`ListWidget.SizeHints` used the empty result count as a fixed content height,
with no horizontal growth or vertical stretch. An isolated layout regression
reproduces it (`stage4/list-layout-before.log`). Thus the stage-3 symbol dialog
is not considered visually accepted.

## Stage 4 — results-list layout repair

Added opt-in `ListWidget.SetFillAvailable` / `FillAvailable` SDK APIs and designer
property, retaining default content-sized behavior for compatibility. Fill mode
supplies a stable preferred viewport plus stretch/growth hints. The command
palette, Quick Open and workspace-symbol search now all opt in. Tests verify
empty/one/thousand-row states, resizing, selection retention and the palette's
actual content/footer geometry. Quick Open also focuses its selected editor only
after the modal loop unwinds. Documentation: `gui/LIST_WIDGET_LAYOUT.md`.

Focused macOS race tests pass. Full gates and a fresh Windows desktop replay
must validate this newer snapshot separately from the stage-3 functional pass.
The macOS native pipe still fails after an explicit CUA runtime reset; no
alternative unauthorized input mechanism was used.

## Stage 5 — modal focus and verified desktop workflows

The stage-4 desktop replay found a real additional defect: cancelling the
command palette cleared logical widget focus, so subsequent arrow keys, F12
and text input no longer reached the editor. `Dialog.SetInitialFocus` now defers
input focus until activation; the three finders no longer steal focus while
being constructed. Win32 modal teardown restores the owning widget only when
the owner is still visible/enabled and no other widget explicitly claimed focus.
It does not call `SetForegroundWindow`.

The first focused native test also had an invalid environment assumption:
its owner stayed invisible in SSH Session 0, even after explicit show calls.
The same source/tests pass in interactive Session 1. Visibility-dependent tests
now require `SILK_TEST_NATIVE_MODAL=1` in the interactive gate (absence of a visible
owner is a failure there, never a skip). Non-interactive unit runs may skip with
the exact interactive rerun instruction. Tests cover OK/Cancel, WM_CHAR delivery,
preserving explicit focus changes, hidden-owner rejection, and nested dialogs
restoring their immediate owner without enabling the main window too early.

Verified source snapshot: 1,611 inputs in `stage5/source-manifest.json` under the
artifact directory above. Both macOS and Windows passed uncached race tests for
`diagnostic`, `macro`, `internal/fileops`, `core`, `paint`, `gui`, `ged`,
`cmd/silkgen`, `cmd/silkide`, plus `go vet -composites=false` and IDE builds.
Windows tests ran in Session 1 with required native modal coverage. Binary SHA256:
`cb92306515cc1c6c82f409ab5022d73f419461359131aaea8470ee83fd910ea1`.
macOS SHA256: `d5905288da146cba3aa4269edce26cc0e07edd95339149468254e8296ffe0806`.

Three fresh Windows desktop fixtures completed with saved-file readbacks, undo,
task result 0, and clean exit without their gopls children:

- Finder: palette Esc followed by typing/navigation, exact Unicode F12 and
  workspace-symbol cursor positions, and Quick Open destination/focus.
- Explorer: reveal/Enter, filtered cross-file selection, focused F5 refresh,
  hidden-file filtering, and F2 rename cancellation preserving the file.
  This does **not** certify renaming/deleting an open or dirty editor.
- Macro: semantic highlighting/outline, F12/reference display, completion and
  undo, visible `SILK-MAC008` with recovery text, and generated-Go readback.

Inspected command-palette, symbol-finder and Quick Open screenshots now show
readable full-width result viewports with no footer overlap. Explorer and macro
screenshots were also inspected. Known visual gaps remain: selected file icon
contrast, outline header contrast, and dark reference-path text. The Go outline
still misses a declaration preceded by the tested inline emoji/comment.

All original failure logs are retained. An initial replay shell loop consumed
remaining cases through SSH stdin; it was corrected to an explicit case list,
and the two not-yet-run fixtures were then executed and independently verified.
No failed/omitted run is counted as a pass. This report section is a
documentation-only addition after the source snapshot's gates.

macOS `cua.getState()` still fails with `Sky Computer Use native pipe startup
failed`; no real macOS editor desktop pass is claimed. Next source-review work:
modal show-callback panic cleanup on both backends, bounded/cancellable LSP
symbol requests, open-editor filesystem mutation coordination, remaining error
paths and the visual gaps above. Overall goal remains active; no commit/push.

## Stage 6 — bounded symbol search and modal initialization failures (in progress)

- Added `LSPClient.WorkspaceSymbolContext`, preserving the old API/decoder and
  UTF-16 coordinates. Pipe-level tests verify exact query/cancel IDs, precancelled
  no-write behavior, response deadline errors and late-reply isolation.
- Replaced per-keystroke goroutines with one dialog worker, one latest pending
  query, 90 ms debounce, active cancellation and one coalesced GUI delivery.
  Tests include a 10,000-update burst with a non-cooperative prior request,
  blocked GUI delivery, closure, client replacement, empty results, stale Enter
  prevention and selected-row acceptance. UI caps matches at 500 with an
  explicit refinement message; SDK results remain complete. Search states and
  coded error tooltip/cause/hint have English and Chinese text.
- Windows native baseline reproduced initialization-panic failure exactly:
  `modal=true`, live HWND, `ownerEnabled=false`. Both backends now register
  teardown before activation/show callbacks. Recovered failures cannot return
  acceptance, have `SILK-GUI001` + logged stack/cause, and expose
  `Window.ModalError` / `Dialog.LastError`. Finder/palette hosts surface them
  instead of silently treating them as user cancellation. Native Windows tests
  cover keyboard recovery, error propagation and successful dialog reuse.
- Focused macOS race/model/protocol/native-render tests passed after correcting
  a test-only `go/token` import collision; original failure retained. The full
  gates and updated real Windows typing/cancel/reopen replay remain pending
  for this snapshot. macOS real editor desktop acceptance remains separate.
- SDK documentation now explicitly records an existing limitation: synchronous
  stdin/cancellation writes cannot be interrupted solely by context expiry.
  Session Close releases blocked I/O. The bounded dialog scheduler is not a
  claim that all transport backpressure/lifetime problems are solved.

Follow-up remains open: transport backpressure, open-editor rename/delete and
async mutation coordination, remaining coded error surfaces, the screenshot
contrast/outline issues, and native macOS desktop verification. Goal active.


### Stage 6 final acceptance

The 1,621-input Stage 6 snapshot passed Windows interactive native-modal tests,
nine-package `-race -count=1`, vet and build. Binary SHA256:
`89789861a57cd4b40715eaca350edbdfddf7140c0f478c5b65b329114b5d44b6`.
All three fresh Windows desktop replays finished with task result 0:
Unicode F12/symbol selection with exact save/readback/undo; quick-open target and
focus; three symbol-cancel/reopen typing cycles; explorer reveal/filter/F5/hidden
files; macro highlighting/definition/completion/undo/error/generation. Each IDE
exited and its owned gopls child retired. Captured finder screenshots show full
width rows, an empty-result message and no footer/status overlap.
Mac nine-package race/vet/build passed, but that gate did not place gopls on PATH.
The real macOS desktop remains unverified: CUA still reports native pipe startup
failure. Stage 6 source is frozen separately; it does not certify Stage 7 changes.

### Stage 7 — native LSP transport backpressure (verification in progress)

A real `os.Pipe` baseline failed: cancellation could not return while the peer
stopped reading a large frame; only external Close released it. Replaced the
uncancellable write mutex with a context-aware serialization gate. Native write
cancellation closes the possibly partial frame stream rather than corrupting it,
releases waiters and reaps the owned server. Request cancellation after a complete
write uses one bounded protocol-cancellation worker (64 IDs, 500 ms write budget),
not an unbounded goroutine per request. Added `SendNotificationContext`, short-write
checks and stable `SILK-LSP003` including original cause/restart hint in IDE feedback.

Tests cover full native pipes, gate cancellation, queue saturation, concurrent
frame integrity, document notification ordering, late replies, short writes,
pre-cancelled calls, real stalled child cleanup and coded UI feedback. Focused
macOS regressions and 10 repeated race runs passed. Final nine-package gates now
explicitly include a real macOS gopls v0.22.0; Windows native/full/UI gates pending.
No claim of preemption for custom MarshalJSON or non-closeable custom I/O.

Still open: async filesystem mutations coordinated with open/dirty editor
identities; nonblocking Quick Open indexing; remaining contrast/outline issues;
other historical error surfaces; actual macOS desktop and Linux runtime checks.
No commit, push or release in this iteration. Goal remains active.


### Stage 8 — background Quick Open and terminal LSP diagnostics

- Added SDK `ScanProjectFiles`: rooted batched enumeration, no child symlink or
  special-file traversal, 50,000 visited entries / 64-level defaults, cancellation,
  partial-result contract and `SILK-FS005` / `SILK-FS007` causes/hints.
- Quick Open no longer scans or ranks on the GUI thread. One application-wide
  worker owns one scan/current session, a latest debounced query, per-dialog
  index and one coalesced delivery. Reopening rescans, closing invalidates work,
  repeated reopen during a blocked filesystem call cannot multiply workers.
  Empty/indexing/searching/partial/count states are visible; only 500 rows render,
  queries can still find all indexed files. Enter respects the selected row and
  slash-separated paths work on Windows. Error tooltips retain causes/hints.
- Added thread-safe copied `LSPClient.Err()` snapshots and coded session retirement
  guarded by client/generation. Explicit shutdown stays quiet; unexpected failures
  survive stale-request suppression. Startup failures retain code/cause/hint;
  initialize cleanup no longer cancels its posted error out of existence.
- Full macOS regression caught a transport edge missed by prior focused runs:
  peer read completed before Write acknowledgement, then cancellation tore down
  a healthy frame. Original FAIL log is retained. A single per-client writer now
  lets request callers cancel immediately, drains at most 500 ms, and sends one
  protocol cancel after a complete frame; only a still-stalled request stream is
  retired. Notification/reply partial writes remain terminal. Added a deterministic
  delayed-ack test and retained the original cancellation assertions unchanged.

Validation for this source:
- Initial file-finder compile caught an unused import; corrected, failure retained.
- Async scan/dialog/session regressions: 10 repeated race passes.
- Original cancellation regression: 100 repeated race passes; deterministic
  transport suite: 10 repeated race passes, including native stalled child cleanup.
- macOS final nine-package `-race -count=1`, vet and build passed with real gopls
  v0.22.0 on PATH. SHA256:
  `da73310ab901f9dae8d909b8fe83473ec95521591659161d461617757414a2dc`.
- M2 Max synthetic 50k-file ranking benchmark (3 iterations, not a Windows/UI
  latency claim): 11.35 ms/op, 13.2 MB/op, 50,010 allocs/op. Further top-K/cached
  scoring optimization remains worthwhile; current ranking is off the GUI thread.
- Windows Stage 7 gate passed; its restart/unsaved-buffer finder replay passed,
  other replays were still running when this snapshot was prepared. Stage 8
  native/full/UI validation has NOT run yet. Its prepared UI replay adds 520-file
  indexing, search beyond the visible 500-row cap, slash-path open/save/undo,
  repeated cancel/reopen, owned-gopls failure injection and unsaved restart recovery.

Still open: editor-aware async rename/delete, shared filesystem change handling,
contrast/inline-comment outline fixes, remaining historical error paths, real
macOS desktop acceptance and Linux runtime validation. No commit/push/release.

## Stage 9 — async editor-aware rename (in progress)

- Reproduced stale path capture with a real disk move: the changed signal reset
  the tab to `before.go *` and failed to attach macro analysis to `after.silkgo`.
  Baseline failure retained in `stage9/identity-baseline.log`; dynamic identity
  callbacks now cover changed/completion/hover/signature/breakpoints.
- Added SDK async `FileExplorer.RenamePath` + host reservation/commit coordination.
  F2/context menus share this path. One pending operation, no-replace filesystem
  mutation off-thread, root-retirement-safe completion, subtree selection and
  expansion migration; typed `SILK-FS008` for busy operations.
- IDE reserves old/new subtrees, preserves live dirty buffers/undo/cursor, and
  protects save/open/close/quit while pending. Completion migrates editors, tabs,
  LSP/macro identities, references/problems/bookmarks, MRU/session paths; affected
  old coverage is invalidated. Providers restart using latest unsaved text after
  the old URI closes, and old queued provider callbacks lose their identity guard.
- Added deterministic worker-blocked, collision, root-switch and lifecycle tests,
  real filesystem directory moves, failed destination preservation, stale save
  rejection, session persistence readback and real gopls unsaved-definition test.
- Focused race tests and final nine-package macOS race gate pass. Initial test
  compile failures (unused import, nonexistent context API) are retained separately.
  Mac desktop inventory was retried: `Sky Computer Use native pipe startup failed`;
  no mac app was launched and no desktop actions were injected.
- Stage 8 Windows 1630-input snapshot now passed native modal/transport/async tests,
  nine-package race, vet and build; binary SHA256
  `52e7e161a1507f60c2664f2d4942f0ee0539853388d425df33d4677aea01df88`.
  Its finder replay passed (520 files, slash query beyond 500 visible rows, owned
  LSP disconnect/restart and unsaved-buffer undo). Other desktop replays are being
  followed on the same live pipeline; Stage 9 is not covered by those results.

Remaining rename work: active designer/autosave/reloader migration (currently
explicitly rejected before disk changes), running build/test result invalidation,
symlink alias identities, case-only rename portability, asynchronous preference
flush, and current-source Windows desktop verification. Create/delete still need
async editor coordination and explicit dirty-buffer decisions. These are tracked
as incomplete requirements, not waived acceptance gates. Goal remains active;
no commit, push or release this iteration.

## Stage 10 — bounded asynchronous editor Git markers (in progress)

- Confirmed `SetFilePath -> RefreshGitStatus -> GitDiff` synchronously waited on
  a Git process. A real external-repository fixture additionally reproduced
  missing gutter markers because Git used the IDE cwd instead of the file's
  repository. Baseline FAIL retained in `stage10/git-cwd-baseline.log`.
- Editor path binding/refresh now dispatches through a process-wide gutter
  scheduler: at most two active jobs, one latest pending request per editor,
  one queued GUI delivery globally, no idle polling. Close/rebind/shutdown retires
  old work; results cannot repaint a renamed/closed editor. UI getters return
  copied maps/diagnostics. Save now refreshes the IDE gutter asynchronously too.
- Added synchronous off-UI `GitDiffContext`, literal paths, per-file cwd, sanitized
  repository-selection environment, disabled external diff/textconv and Windows
  console creation, request timeout, bounded stdout/stderr/hunk allocation, and
  joined pipe readers/direct-child reaping. Non-repository is normal; actual
  failures carry `SILK-GIT001/002/003` plus cause and recovery hint.
- IDE logs deduplicated automatic failures and exposes **Refresh Editor Git
  Markers** in the command palette for explicit success/error feedback. SDK
  lifecycle and remaining semantics are in `gui/CODE_EDITOR_GIT.md`.
- New tests cover a real Git repository, literal filenames, missing Git, process
  timeout/output flooding, blocked I/O with live edits, 10,000 refresh coalescing,
  1,000 close/reopen, concurrency bound, 64-editor GUI batching, stale deliveries,
  empty/error clearing, ownership, and IDE feedback/action routing. Ten repeated
  race runs passed. A feedback test initially counted the unrelated Opened Info
  row as duplicate output; detailed log proved this, and assertions now measure
  added rows while preserving the existing Info row. Both failure logs retained.
- Final macOS nine-package race/vet/build passed; SHA256
  `046d55c92ca5ccaa1bbb400c6251beca7887fb3d23a1c21c8f2e53583528ddc6`.
  Fresh native desktop inventory still failed with Sky native-pipe startup error;
  no desktop app/input was launched. Linux runtime remains unverified.
- Stage 9 Windows native/full gate passed (1637 hashes, real gopls rename x3,
  race/vet/build; binary
  `b071b321dfe7a78482e4ede81c21dde0f7b871daf6602bb3dc1f2bbd880effff`).
  Its native UI replays remain on the same live pipeline. Before its rename
  replay was launched, the paste fixture was corrected to include gofmt's blank
  line so Undo checks the paste rather than an extra formatting step. Original
  script and independent gofmt observation retained; no product assertion removed.
- Prepared Stage 10 Windows gate plus five fresh UI replays, including a controlled
  direct Git child that stays blocked while real editor input/clipboard readback
  must succeed, save/undo readback, timeout-code screenshot and child cleanup.
  These are prepared tests, NOT passed Windows acceptance for this source yet.

Remaining: asynchronous preference persistence; designer/autosave/reloader rename;
create/delete coordination; filesystem aliases/case-only rename; build/test stale
results; contrast/Go inline-comment outline; other legacy Git/SDK I/O and errors.
Git markers intentionally describe saved worktree vs index, not unsaved buffers.
Custom Git/filter grandchildren and uninterruptible OS startup are not certified
by direct-child cleanup tests. No commit/push/release; goal remains active.

### Stage 10 follow-up: real modal failure and terminal Git cleanup

- Stage 9 finder/explorer/macro desktop runs completed with task result 0. Rename
  disk/buffer/undo/conflict assertions passed, but exit timed out. The actual
  remaining window was the rename-conflict MessageBox, not a proven Git/LSP hang.
  On the same owned process, Enter again failed to dismiss it; clicking its OK
  button then allowed clean exit (`stage9/windows-exit-confirm.log`). The failed
  replay and screenshot remain evidence, not relabeled as a green run.
- New logical baseline tests reproduced unwrapped/clipped diagnostics, missing
  Enter/Escape handling and global shortcuts reaching behind a modal. Fixed
  MessageBox selection/wrapping, safe default/cancel behavior, button layout,
  owner-form centering and modal shortcut isolation in both native backends.
  Added native Windows message-box and GLFW dispatch regression tests. SDK
  contract is documented in `gui/MESSAGE_BOX.md`.
- Added terminal `ShutdownEditorGit(ctx)`: reject new work, cancel pending jobs,
  suppress stale deliveries and wait for actual worker exit after the event loop.
  A deadline explicitly means child cleanup is not yet certified. Tests prove
  cancellation and completed process reaping are not interchangeable.
- Combined MessageBox/modal/input/Git-focused race tests passed 10 repetitions.
  A fresh current-source macOS full gate is running; the earlier Stage 10 SHA and
  pass above predate these changes. Stage 10 Windows acceptance is still pending.
  Its rename replay now also copies the complete code/path/hint and asserts Save
  cannot mutate a background dirty editor while the error is open. Git replay
  requires a live owned Git child immediately before exit.

## Stage 11 — shared declarations and outline correctness (in progress)

- Reproduced missed inline-comment/grouped declarations, false declarations inside
  block comments/raw strings, loss of valid symbols during incomplete edits, and
  compiler `//line` locations incorrectly redirecting editor jumps. The editor and
  SDK now share tolerant lexical/AST declarations and actual buffer positions.
  Definition/reference fallbacks retain recoverable AST results, without the
  comment/string-prone regexp fallback. One old test expected all malformed source
  to yield no references; it now verifies actual recovered identifiers instead,
  with the original failing log retained. Local resolution is still name-based.
- Reproduced old semantic symbols after switching/editing, hardcoded light-theme
  colors, ignored font size, long names outside the dock and scroll rows painting
  over the header. Fixed source/revision retirement, added a guarded async SDK
  setter, themed font-aware rendering, separate clips and visible-row traversal.
  Generic methods group correctly; macro semantic kinds retain their label.
  Keyboard selection, paging, parent/child expand-collapse and Enter navigation
  are covered by tests. The callback remains line-based, not exact-column.
- `Edit.Layout` also passed negative wrap width and scroll maximum on small/short
  diagnostic widgets. A new tiny-widget test failed at width -9; layout now clamps
  text width to 1 and the scroll maximum to 0. This removes those specific invalid
  range warnings without weakening ScrollBar's own defensive checks.
- First Stage 11 macOS nine-package race run passed. After the small Edit fix,
  combined outline/message-box/edit tests passed ten race repetitions. Final
  frozen-source race/vet/build evidence is recorded separately in
  `bin/qa-artifacts/editor-sdk-20261021/stage11/`.
- Real Cairo light/dark 14/24-point images were rendered and visually checked.
  Idle draw benchmark on this Mac measured ~144 us for 100 symbols and ~141 us for
  10,000, with the same visible viewport/allocations. This demonstrates bounded
  visible-row work, NOT allocation-free paint or a whole-app frame-rate guarantee.
  Correct changed-source parsing is slower than the old incorrect line scanner;
  large-file background analysis remains pending.
- Stage 10 Windows initial gate correctly failed when real Git cases were skipped.
  Official hash-verified isolated MinGit upload and the deferred gate/replay runner
  remain separate from Stage 11. No Stage 10 result certifies Stage 11 changes.
  Native macOS desktop remains unavailable (Sky native pipe startup error); the
  Cairo/package results do not replace native desktop or Linux acceptance.

SDK details: `gui/CODE_EDITOR_SYMBOLS.md`. Settings persistence, coordinated
create/delete, designer-aware rename, aliases, stale build results and full native
current-source acceptance remain open. No commit, push or release this iteration.

### Stage 11 activation follow-up

A host-level test additionally reproduced Enter moving the outline target cursor
but leaving keyboard focus in the outline. Added `NavigateToLineCol` for explicit
user jumps (clears old selections/secondary carets/completion, reveals folded
ancestors, focuses the editor, preserves text/undo). SilkIDE outline activation
now uses it. Existing scroll-only SDK methods keep their selection/focus semantics.
The previous 1652-input macOS gate/binary is retained as `.pre-activation`; it does
not certify this follow-up. Final current-source evidence is regenerated separately.

## Stage 12 — Windows directory rename / Git lifecycle barrier (in progress)

After installing verified real Git, the Stage10 native gate failed directory
rename twice in three repetitions with a sharing violation. A controlled native
probe proved the same rename fails while the SDK Git child owns the source CWD,
then succeeds immediately after actual child reaping. This is a product lifecycle
failure, not a reason to skip Git tests or add arbitrary sleeps.

- Added path-scoped `SuspendEditorGitPath` with background `Wait` and idempotent
  `Release`. It includes active jobs from closed/rebound tabs, coalesces matching
  refreshes without workers, and leaves unrelated paths running. Nested gates,
  queued results, reentrant delivery and terminal shutdown honor the reservation.
- Extended explorer coordination compatibly with `RenamePreparation`: UI prepare,
  cooperative bounded off-UI readiness wait, native mutation, UI completion. Wait
  failure performs no mutation and reports `SILK-FS009` with cause/path/hint.
- Wired the actual IDE rename transaction, preserving latest dirty buffers and
  undo histories while it waits. Success migrates identities before release;
  failure resumes original providers; repeated completion cannot double-release.
- Deterministic tests cover delayed child exit despite cancellation, retired jobs,
  10,000 refreshes, nested gates, unrelated paths, callback-batch retirement,
  timeout/root retirement without mutation, and success/rollback. Ten focused
  race repetitions passed, and real macOS gopls rename/unsaved-definition tests
  passed three repetitions. Native current-source gate/replays are next, not yet
  implied by these local results.

SDK contracts: `gui/CODE_EDITOR_GIT.md`, `ged/FILE_EXPLORER_RENAME.md`.
Other process/alias/designer/settings lifecycles remain open. No commit/push/release.

## Stage 13: settings SDK persistence and native keyboard regression coverage

- Preserved the Stage12 native failure: full race gate failed in
  `TestWin32KeyDownRunsTheWindowLayer`, whose source assertion still required
  direct `dispatchShortcut` after modal isolation introduced the wrapper.
  Updated the ordering guard to the real wrapper and added a true HWND
  WM_KEYDOWN / repeat / WM_CHAR test. Native verification is required; no test
  is removed and the modal-isolation gate stays enabled.
- `core.TDoc` now propagates buffered flush, gzip close/trailer, short-write and
  file-close errors. Baseline tests reproduced false-success returns before
  the fix. Caller buffers are fully flushed even for compressed output.
- Settings saves use immutable versioned snapshots outside the settings mutex,
  one coalescing on-demand writer, compatible Sync, optional RequestSync and
  context-aware Flush. Timeout does not pretend I/O is canceled or joined.
- Same-directory temp + sync + close + replace; preserve old file on failure,
  protect detected read/decode failures, validate full gzip stream, retain dirty
  edits on failure/newer generation. Added SILK-SET001/002/003 with path/cause/hint.
- Coverage includes 10,000 coalesced requests, responsive access under a held
  writer, old/new generation ordering, error/retry, concurrent sync, compressed
  corruption, atomic readback, and Windows locked-destination preservation.
- Host preferences setters still invoke Sync synchronously. Follow-up must wire
  asynchronous UI save, bounded error delivery and final lifecycle join. Initial
  lazy load, cross-instance conflict handling, file ACL migration and the broader
  legacy TDoc reader's error handling remain outside this completed SDK slice.
- Keep Stage12 and Stage13 artifacts/snapshots separate. No commit/push/release.

## Stage 14: real Windows atomic snapshot semantics and IDE async preferences

Stage13's native HWND shortcut checks passed 10x. Its settings gate then failed
10/10 in the unchanged concurrent-reader test (Access denied/sharing violations).
A four-way real Win10 probe isolated both necessary conditions: MoveFileEx fails
with either sharing mode; FileRenameInfoEx POSIX succeeds only with a shared-delete
reader, leaving the held handle on the old bytes and new opens on the new bytes.

- Added internal file snapshot read/replace primitives; core TDoc file loading and
  settings loading use cooperating Windows handles. Windows uses extended rename,
  never delete-first; unsupported extended info class falls back to legacy rename
  without bypassing permission/sharing errors. Read-only and exclusive handles
  remain protected. Native unchanged concurrent-reader test, held-old snapshot
  test and locked-file error/retry test all passed 10x on the initial Stage14
  file-subset snapshot. Full current-source gate still required.
- IDE preference setters, appearance and workspace capture now schedule rather
  than synchronously writing on UI callbacks. One host worker joins SDK Sync;
  requests coalesce, at most one current-result UI notification is queued, and
  repeated failures are de-duplicated while keeping code/path/cause/hint.
- Appearance UI says applied/saving in background instead of claiming saved.
- Main joins final preferences after EventLoop return; no pending worker or UI
  wakeup survives successful Shutdown. Deadline ends only a wait, not I/O.
  An indefinitely stalled disk can delay final process exit; shutdown errors go
  to diagnostics/logging after the window closes.
- Session/rename/run-config/appearance readback tests now await actual flush.
  Quit tests exercise the real finalizer, plus an AST guard verifies main invokes
  it on its actual preferences after the dispatch loop. Worker stress covers
  10,000 updates, typing/undo during a held save, timeout and real final join,
  bounded error delivery, retained original file/runtime edits, retry and stale
  callback retirement.
- Remaining: full native Windows gate plus six real UI replays; mac native UI
  unavailable; legacy generic TDoc reader silently accepts truncated/bad gzip
  trailers (the settings-specific reader is already protected); broader editor,
  explorer, macro/LSP and cross-platform work remains active. No commit/push.

## Stage 15: stream-reader correctness, coded diagnostics and native QA isolation

- Reproduced and repaired generic TDoc acceptance of truncated/bad-CRC gzip,
  dropped `n > 0` I/O errors, and broken space/tab key round-trips. Replaced
  unchecked rune reads with a positioned streaming parser; failures return no
  partial document and use `SILK-DOC001/002/003` with hints and wrapped causes.
- Preserved retained-reader framing, all four newline forms, limited-prefix
  inspection, unnamed/nested nodes and legacy printable escapes. Full gzip loads
  validate one member without retaining a second full decompressed copy. Added
  exact-next-member, line-limit, one-byte input, file-path and position regressions.
- Review regressions also proved newline lookahead could swallow a pending I/O
  error and indentation diagnostics pointed past the bad indentation. Both are
  corrected; a lookahead error is now reported, not lost at a completed prefix.
- Fuzzing found an escaped raw control key that parsed successfully but could not
  reload after Save. Reject that nonrepresentable escape with a syntax code and
  retain the minimized input as a regression corpus. Plain canonical round-trip
  fuzz passed 714,855 executions; escaped-key/value fuzz passed 45,989 executions
  on their recorded runs. These are bounded fuzz results, not exhaustive proofs.
- Added real Settings load -> edited escaped/grouped keys -> RequestSync/Flush ->
  fresh-instance readback for both plain and compressed existing files.
- Stage14 Windows full race run ended Ready/result1. Its only failing assertion
  was `TestPostThreadMessageReachesItsQueue`: an unrelated WM_TIMER was read before
  the expected wakeup. The native modal/HWND, persistence/preferences, transport,
  rename/real-gopls, Git and outline focused gates passed before that failure.
  Corrected the test to select its message, verify payload/NULL HWND, and retain
  unrelated queued traffic; added explicit queue-noise coverage. This is a test
  isolation correction, not a claim of a new production scheduler/race fix.
- Stage15 still requires frozen local/native gates and six fresh UI replays.
  Latest mac CUA retry still reports `Sky Computer Use native pipe startup failed`;
  package tests do not replace native desktop acceptance. No commit/push/release.

## Stage 16: compact editor/designer chrome and macOS native acceptance

- Opt-in compact menus/toolbars and canvas actions, font-aware square targets,
  scalable overflow chevrons, trailing-edge popup anchoring and non-overlapping
  editor status fields. Ordinary designed form controls are not globally shrunk.
- Macro outline now reacts to asynchronous provider revisions; stale hover and
  signature responses are retired by editor/path/text/interaction identity.
- Ten-package macOS race gate and vet passed. Real desktop input verified macro
  outline, narrow overflow and Fit, save/undo/redo with disk readback and real-gopls
  F12. The Polished source manifest/binary, not an older intermediate build, is
  the accepted artifact; see the linked macOS report.

## Stage 17: Windows native closure and bounded idle/resource sampling

- Reproduced the original timer test passing alone 20x but failing in the full
  GUI package on an unrelated HWND timer. Isolated the native queue in a bounded
  subprocess without weakening timer identity. Added 64 real wakes and an
  unfiltered negative control; the production fair selector must take the exact
  timer before the remaining 63 wakes without reordering them. No production
  scheduling change is inferred from this test-only correction.
- Fresh Win10 source deployment: 1685 hashes verified before/after build and
  after UI replay. Twenty focused repetitions, ten-package race, vet, native
  windowsgui build and 19-DLL closure passed. Same production code as Stage16;
  the sole code delta is the Windows-specific timer test.
- Seven native editor/SDK scenarios passed: finder/UTF-16/LSP restart, explorer,
  macros, dirty file/directory rename and conflict isolation, blocked Git child
  lifecycle, generic/current-revision outline, and failed/retried/final/restarted
  preference persistence. Toolbar mouse actions used saved-file readback.
- Kept failed harness evidence: read-only PowerShell HOME, unavailable process
  ExitCode and missing foreground acquisition. Added owned-foreground guards;
  explorer then passed from a new fixture. Fit was retried in one foreground
  sequence and verified by before/after canvas screenshots, not menu dismissal.
- Twenty-second IDE-only idle sample: 0.25 CPU seconds / 20.220765 wall seconds
  (1.24% of one core), 0 unresponsive samples. Twenty resize cycles: GDI 83 -> 83,
  USER 41 -> 41. Not an FPS benchmark, large-workspace test or long-term leak proof.
- Linux desktop, multi-DPI/multi-monitor, complete controls/menu parity and
  remaining sidebar/status/language-mode details are still open. See the report
  for screenshots, exact hashes, test commands and narrower acceptance boundaries.
