# Changelog

## Unreleased — active call parameter

- Highlight the active parameter in the editor's signature hint and dismiss it
  when the closing parenthesis is entered. Both UTF-16 parameter ranges and
  unambiguous text labels from language servers are supported.
- Keep the existing SDK signature-help interfaces available for callers that
  only need plain text.
- Keep signature help current during burst typing by requesting the latest
  synchronized caret. Obsolete requests are cancelled; completion does not wait
  for a slower signature response.
- Keep signature help beside, rather than on top of, the completion list.
  Typing over the existing call closer closes both without editing the file.
- Do not request local or gopls automatic completions for a number literal.
  Numeric input still updates the document and keeps parameter help current.

## Unreleased — owned editor hints

- Keep editor diagnostics, LSP hover documentation and call signatures
  inside the editor's owner window, without allocating a popup or changing
  keyboard focus. Retiring one editor cannot hide another editor's hint.

## Unreleased — passive control tooltips

- Show registered control hints after the existing pointer idle delay. Paint
  the hint inside its owning client without changing focus, hit-testing or
  layout; dismiss it on leave, click, scroll, capture, or owner changes.
- Refresh or remove a visible hint when its registration changes, without
  a separate native window or a repeating tooltip timer.

## Unreleased — visual layout editing

- Drag controls from the palette to an explicit slot in horizontal and
  vertical layouts, or drag an existing control to reorder it within the
  same layout. The insertion line previews the exact position.
- Keep the reordered control selected and its identity intact. Each drop
  is one undoable action; leaving the layout, canceling, or crossing a
  locked item never commits a reorder.
- Use separate vector badges for insert, reorder, and invalid targets.
  Position hints follow the IDE's language.

## Unreleased — native Wayland

- Paint a newly configured GLFW window even if its widget has not received
  logical bounds yet; reconcile geometry against the valid native drawable.
- In native Wayland builds, use an xdg-foreign owner for the GTK file chooser
  and release it on cancel or completion. Keep the existing X11 route.
- Draw owned Wayland popups inside their form. Menus retain their anchors,
  nesting, logical input capture and viewport clipping without a second
  top-level surface. X11, Cocoa and Win32 keep their existing popup routes.

## Unreleased — Gantt interaction

- Reduce allocations and latency when dragging large Gantt plans: validate
  only the new schedule and reuse it on drop rather than rescheduling again.
- Discard a drag when its source model has changed. Undo remains a single
  edit, and host callbacks cannot mutate live dependencies or history.

## Unreleased — typed HMI alarms

- Require Go 1.26.11 for Silk and the map SDK, and build the bundled
  gopls with Go 1.27.4, for the November 2026 Go security fixes.
  Rebuild existing binaries to pick up the patched toolchains. Upgrade x/net to v0.62.0
  and Gorilla WebSocket to the upstream revision that patches the MQTT
  client's WebSocket dependency.

- Search live rules and history by rule ID, point and priority (and event
  type in history). **Pending only** shows live rules awaiting acknowledgment
  without changing the runtime or dropping retained events.
- Author point alarm rules with exact numeric hysteresis or typed bool/text
  matching, individual raise/clear delays, quality gating and optional latching.
- Browse live alarms and bounded session history in the standalone workbench
  and the IDE's **Process Runtime** dock. Switching views keeps acquisition
  running. Both live and historical IDE rows can jump to the authored rule.
- Acknowledge the selected occurrence without writing a point. Retired sessions
  and replacement occurrences reject old acknowledgment tokens.
- Keep the selected history event when new alarms arrive, even when multiple
  events share one rule ID. Expired events clear selection; changing back to
  Live rules does not silently select an acknowledgment target.
- Remove platform timer-resolution dependence from the expired file-search
  deadline test.

## Unreleased — print preparation and PDF transparency

- Capture a design only after Print is confirmed, then compress the detached
  raster and submit on a worker. Cancel does not generate or stage a PDF.
- Keep fully transparent pixels visible as cutouts even when a source image
  contains no partially transparent pixels. Compress raster data by scanline
  to reduce peak memory when exporting or printing large designs.

## Unreleased — designer appearance

- Use the measured size when opening GLFW popup menus. On X11, the first
  open no longer shrinks a newly attached menu to one pixel.
- Preserve the option to show a caption alongside a button icon when reopening
  a design or generating Go code. Existing designs still use their widget defaults.
- Refresh cached Designer previews after theme, UI font, or icon palette changes.
- Keep the light and dark details inside disabled raster icons without reviving
  saturated asset colors. Reuse the mask and apply the current theme tint on draw.

## Unreleased — designer SVG/PDF export

- Prefer the desktop file chooser portal on Linux/X11 when available.
  The chooser receives an owner window, format filter and suggested save
  name; minimal desktops still use Zenity.
- Choose SVG or PDF before opening the native save panel, so the filter and
  suggested extension match the export rather than the SilkUI design file.
- Export canvas dimensions in millimetres instead of interpreting them as
  pixels or PDF points. Raster widgets keep their authored position and size,
  and icon exports preserve transparency and disabled-state colors.
- Keep PDF text upright and apply the same rotation and scale to image,
  label and rectangular clip geometry. SVG labels remain selectable.

## Unreleased — Process Runtime item navigation

- **Run latest** in the dock explicitly revalidates and runs its bound source
  after an edit, without changing the active tab or writing the draft.
- Navigate from a live point row or a selected device/pipe to its original
  source declaration, with editor Back/Forward and exact snapshot ownership.
- Inspect the rendered live diagram without enabling authoring or writes;
  stale or retired snapshots cannot move the source caret.

## Unreleased — 2D process source navigation

- Open the visual editor with the node or pipe under the current `.silkhmi`
  caret selected; **Go to source** returns to that item's ID in the current
  unsaved JSON buffer, with normal Back/Forward history.
- Resolve original Unicode/CRLF/compact JSON positions on the decode worker.
  Reject stale tab/revision results and never apply or discard visual draft
  edits as part of navigation. No authored schema or generated code changes.

## Unreleased — bounded obstacle-aware process pipes (2026-11-17)

- Add pure-Go `hmi.PipeRouter` with outward endpoint directions, conservative
  clearance, mandatory waypoints, interval-indexed collision queries and bounded
  deterministic search/work. Round envelopes outward to prevent ULP boundary leaks.
- Keep absent/`orthogonal` routing compatible with existing documents; newly
  connected pipes use `avoid`. Blocked or exhausted routes explicitly fall back
  with amber ink and a viewport-fixed diagnostic, never a false avoidance claim.
- Share geometry between painting, picking and waypoint insertion. Cache by
  geometry revision, including live position/rotation/visibility and canceled
  previews; ignore color/selection/camera-only changes and unchanged grid snaps.
  Budget priority follows document link order, not observer order.
- Preserve routing modes through staged Pipes edits, device templates, `.silkui`,
  preview and generated Go. Add the inert `examples/processrouting` workbench,
  independent collision oracles, large-scene budget and raster/cache regressions.
  [Verification and remaining limits](PIPE_ROUTING_QA_20261117.md).
- Pass frozen macOS/Linux/Windows full and race suites, 181 renderer-subset cases
  and twelve strict actual bundled-gopls checks; full vet retains 501 unchanged
  baseline diagnostics. Native Windows drag/history/menu/fallback, real point
  write/Stop, SQLite readback and the actual exported Go program's Save/normal
  exit pass. Add three original README images; macOS native UI is not accepted.

## Unreleased — exact industrial integers and status words (2026-11-17)

- Fix integral JSON expression comparisons silently merging adjacent values
  above 2^53. Cache signed integer literals and add checked int64 arithmetic,
  exact integer/float comparison, integer remainder and bounded bit/shift operators.
  Preserve short circuit and the 1,024-byte/128-node no-calls expression boundary.
- Reject lossy integer JSON-to-float casts while preserving exact token text
  for string conversion. Integer overflow and invalid shifts fail, never wrap.
- Add `ProcessValue.IntegerValue()` and exact integer card text, condition colors,
  alarm comparisons and close large-integer range geometry. Legacy `Value()` is
  an approximate visual projection, not an exact counter/persistence API.
- Keep staged expression-table help visible on invalid Apply, and add a runnable
  loopback-only `examples/processstatus` workbench with normal/overflow/recovery.
- Add oracle, concurrent-evaluation, real HTTP/SQLite/UI recovery and designer
  persistence/preview/generated-code regressions. Scope and platform/native
  boundaries are recorded in [QA](INDUSTRIAL_INTEGER_QA_20261117.md).
- Pass macOS/Linux/Windows full and race suites, 175 renderer-subset cases and
  twelve strict actual bundled-gopls cases. Full vet retains 501 unchanged
  baseline diagnostics; it is not a clean full-vet pass.
- Add an original Windows workbench capture. Native Run, overflow isolation,
  recovery, Stop, authored-only Save/Generate, independent SQLite readback and
  normal exit pass. macOS native capture remains unverified due to
  ScreenCaptureKit -3811/-3812; source tests do not replace that gap.

## Unreleased — coherent inspector geometry and popup retirement (2026-11-17)

- Commit row-height snapshots once per layout and reuse them for scroll/focus,
  editor bounds and visible-label painting. Public height setters invalidate
  layout; reject nonfinite heights and stale metadata from retired bindings.
- Fence synchronous move/resize/popup callbacks that filter, relayout or rebind
  the inspector. Bound correction to two passes before yielding to the existing
  UI layout queue; do not introduce a timer, polling worker or background UI writes.
- Add UI-thread-only `gui.DismissInlinePopups`: snapshot owners before callbacks,
  revalidate current ancestry, preserve independent native forms/popups and invoke
  each owner at most once across reentrant requests. Ordinary/small snapshots
  allocate nothing; larger custom composites may spill. Cover panic recovery.
- Preserve negative controls for allocation, 26/53px reentry geometry, nonfinite
  ranges, ownership transfer and bounded correction. The reproducible 100/1,000
  ordinary-field geometry benchmark now reports 0 B/op and 0 allocs/op; this is
  steady-state geometry, not zero-allocation startup/drawing or a whole-IDE FPS claim.
- Fix a load-sensitive existing HMI heartbeat test found by the Windows full
  suite: use explicit sample time, fresh/stale recovery, exact-boundary and
  repeated-tick checks rather than assuming constructor work fits within 1ms.
  Preserve the failed run; production point behavior is unchanged.
- Pass final macOS/Linux/Windows full/race suites, 171 renderer-subset cases
  and twelve strict actual bundled-gopls cases. Native IDE saved Undo/Redo,
  current-file reopen, actual generated consumers and SDK palette/capture/
  first-click/53px reentry flows pass on both desktop platforms with normal
  exits. All 2,859 manifest files match three source trees without extra Go;
  full vet retains its 501 existing diagnostics with no additions or removals.
- Add three original native README captures, public SDK contracts and the
  [verification record](INSPECTOR_GEOMETRY_PROGRESS_20261117.md). Development
  source only; broader product/platform acceptance remains explicitly scoped.

## Unreleased — bounded, scrollable property inspector (2026-11-17)

- Add a true clipped content viewport below the fixed search bar, with a
  correctly ranged vertical scrollbar, fractional wheel input and matching
  logical paint/header-hit coordinates. Filter, collapse, resize and Clear clamp
  or reset stale offsets; hidden editors remain lazy and cannot revive on access.
- Forward wheel input from single-line property text/numeric editors to the
  inspector; preserve custom multiline text scrolling. This fixes an actual
  native-macOS failure that direct sheet-handler tests had missed.
- Add optional SDK `IChildFocusReveal` and `IChildFocusOrder` hooks. Offscreen
  fields reveal on Tab without rebinding; visual order survives lazy creation.
  Independent native popup/form ownership and callback reentry are fenced.
- Restrict label/header paint traversal to visible entries and dismiss moved
  inline drop-downs without deactivating text fields or splitting typing history.
  No polling worker, animation timer or idle frame loop is introduced.
- Preserve negative controls, correct the designer regression fixture to walk
  nested editors, and isolate the modal-frame wiring test from pre-existing
  queued layout work without weakening the production UI-task budget.
- Pass final frozen macOS/Linux/Windows full/race suites and twelve strict real
  bundled-gopls cases without skips. Native macOS/Windows wheel, focus, saved
  Undo/Redo, exact-file reopen and normal exits pass; actual new generated
  companions execute with race against the same SDK. Three exports match every
  one of 2,853 manifest files, with no extra Go files; full vet has no changes
  to its 501 baseline diagnostics. Add two original native README images.
- Publication evidence and remaining limits are tracked in
  [the verification record](PROPERTY_SCROLL_PROGRESS_20261117.md). Development
  source only; full widget virtualization, DPI/IME and whole-product parity are
  separate work.

## Unreleased — coherent property inspector (2026-11-17)

- Classify built-in Chinese/English property IDs explicitly: geometry and grid
  placement in Layout, titles/icons/colors in Appearance, Checkable/Checked and
  value/range state in Behavior, HTTP/conversion/device configuration in Data Binding.
  Unknown extension IDs retain existing heuristics; no persisted IDs are renamed.
- Search by built-in category names in English/Chinese and legacy English labels,
  while preserving custom labels, hidden/multi-selection rows and stored collapse.
- Replace duplicated half-pixel header text with cached real bold fonts, theme
  colors and clipped/elided ink. Reject header hits outside the visible client;
  whitespace-only searches no longer show a falsely expanded chevron.
- Restore the caller font after inspector text painting on legacy Cairo, whose
  Save/Restore does not restore its Go-side font field. Preserve negative tests.
- Add three original macOS/Windows README images. Frozen full/race pass on macOS
  (9,608 named passes /29 skips each), Linux (9,547 /37), Windows (9,405 /43);
  twelve strict actual bundled-gopls cases pass without skips. All three independent
  source exports match 2,845 files, with no extra Go files.
- Pass native bilingual filtering/collapse, saved title edits and Undo/Redo, exact
  current-file reopen, independent document/session reads and normal exits 0.
  Actual new IDE companions execute constructor/state/input tests with race
  instrumentation against the same SDK. Full vet retains 501 unchanged baseline
  diagnostics, none new. Development source only, not a binary release or a
  universal native/DPI/performance guarantee; exact index/remote readback gates apply.
  [Current verification and limits](PROPERTY_INSPECTOR_PROGRESS_20261117.md).

## Unreleased — checkable group authoring and native capture (2026-11-17)

- Add GroupBox header Space/Tab operation, repeat rejection and a clipped focus cue.
- Expose localized Checkable/Checked designer properties, persist/reproduce their
  state and add typed OnToggled creation/navigation/binding.
- Connect typed generated handlers after construction, authored values and layout,
  preventing initialization-time business callbacks; runtime signals remain.
- Register decl GroupBox children through AddWidget, restoring actual content
  ownership, nested layout and input gating in ordinary IDE-generated companions.
- Fix internal Windows capture acknowledgements mistaken for external loss;
  strengthen uncaptured/legacy/nested cancellation, reentry and backing-slot cleanup.
- Pass frozen-source full/race: macOS arm64 9,553 named passes /29 skips each;
  Linux arm64 9,492 /37; Windows amd64 9,350 /43. Twelve strict actual bundled-gopls
  cases have no skips; all three post-test exports match 2,842 files /2,081 Go files.
- Pass native macOS/Windows SDK keyboard, disabled/enabled same-coordinate controls,
  inspector Undo/Redo, typed event creation, save/reopen/readback and normal exits.
  Final Windows first toolbar Save retry succeeds after a read-only refusal;
  three earlier fresh controls also pass. Actual IDE output compiles and executes.
- Add three original native README captures and SDK/QA documentation. Renderer,
  native GL, platform scripts and CI-scoped vet pass; full vet retains 501 unchanged
  baseline diagnostics, none new. Retain superseded runtime failures and harness
  failures separately. Development source only, not a binary release, universal
  native/DPI/performance guarantee or full Qt Creator parity.
  [Verification and remaining work](GROUPBOX_AUTHORING_PROGRESS_20261117.md).

## Unreleased — inherited input availability and GroupBox rendering (2026-11-16)

- Fix GroupBox inward-curl corner arcs, constrained title overflow, empty-title
  background erasure and leaked painter font; cache bounded title measurements.
  Include complete titles/checkmarks in hints and queue changed-title layout.
- Add `Widget.IsExplicitlyEnabled`, `IChildEnablement` and
  `Widget.InvalidateEnabledState` without expanding required `IWidget` methods.
  Effective enabled state includes logical ancestors and container gates while
  independently owned native forms/popups retain their own input state.
- Gate nested GroupBox content without rewriting authored child flags. Preserve
  detached/replacement content ownership and factory checked defaults. Persist
  local enabled flags in Button/CheckBox/ComboBox/ToggleSwitch properties.
- Gate native mouse routing on GLFW/Win32; cancel stale gestures, scrollbar
  repeat, clipboard reads and transient editor completion/hints without a fake
  release, document replacement or loss of remembered focus. Reject parent cycles
  before changing tree/input state; cancellation-time reparent cannot revive capture.
- Clear discarded capture-stack backing slots on both native backends, including
  pop/replacement during teardown. Isolate the line-splice allocation regression
  from other asynchronous test work/collector accounting without relaxing its
  four-allocation bound or changing application GC policy.
- Independently validate native input, wheel, Tab and capture scopes for older
  SDK controls whose local-only `IsEnabled` overrides omit ancestor checks.
- Add platform mouse-dispatch and SDK regression tests. Extend the no-polling
  layout example with a long Unicode GroupBox title toggle.
- Pass frozen full/race suites: macOS arm64 9,538 named passes /29 skips each;
  Linux arm64 9,477 /37; Windows amd64 9,326 /43. Four strict actual
  bundled-gopls tests pass per platform, plus 1,340 focused race passes,
  pure renderer/GUI, native GL and platform/release script gates. All three
  source exports match 2,832 files /2,075 Go files. CI-scoped vet passes;
  full vet retains 501 unchanged baseline diagnostics, with no new warnings.
- Add three original macOS/Windows README captures. Real nested-input probes
  accept one mouse and one Space action while checked, block both before/after
  unchecking, and exit normally. Native IDE and generated-consumer results,
  failed automation attempts and remaining platform limits are recorded in
  [verification and remaining work](GROUPBOX_INPUT_PROGRESS_20261116.md).
  Development source only; no packaged release or whole-product parity claim.

## Unreleased — owned dynamic layout invalidation (2026-11-16)

- Add UI-only queued layout/size-hint invalidation, coalesced ancestors, bounded
  turns, hidden/zero-size quiet state and hide/reparent cancellation.
- Preserve normal synchronous invalidators; defer reentry. Wire ordinary
  Label setters and layout/hierarchy changes without a polling timer.
- Fix stale layout snapshots moving foreign children, grid callback coordinate
  indexing and FormLayout duplicate/replaced fields left independently painted.
- Add a no-polling SDK consumer and retain failing regressions, including a
  superseded prototype's nested-pump self-posting failure. Blocked successor
  work now waits for explicit completion instead of polling the UI queue.
- Pass frozen full/race suites: macOS 9,479 named passes /29 skips each;
  Linux 9,418 /37; Windows 9,267 /43. Four strict actual bundled-gopls tests
  pass per platform, plus 440 focused race passes, pure-Go/native GL and script
  gates. All three source exports match 2,819 files /2,068 Go files.
- Verify all SDK example toggles and native resizing on macOS/Windows; actual
  IDE drag, File Save/toolbar retry, editor Unicode, read-only/refused replacement,
  Undo/Redo, About, quit cancellation/refusal and successful dirty Save All exit.
  Independent disk/model/session readback and owned gopls shutdown pass.
  Compile both actual IDE-generated companions in isolated consumer packages.
- Add three original native README captures and bounded layout benchmarks.
  CI-scoped vet passes; full vet retains 501 unchanged baseline diagnostics,
  no new. Quiet CPU samples are scoped measurements, not a whole-product/FPS
  guarantee. This is not a new binary release or full Qt Creator parity.
  [Verification and boundaries](LAYOUT_INVALIDATION_PROGRESS_20261116.md).

## Unreleased — width-dependent layout and readable diagnostics (2026-11-16)

- Add nonmutating width-dependent SDK measurement and shared first-layout
  geometry for wrapped labels, boxes, forms, grids, groups and flowing children.
- Contain fallback Unicode ink and preserve ordinary optical baselines.
- Retain explicit tracks/constraints, derived SizeHints, ownership and bounded
  natural/allocated text caches; reject invalid/duplicate grid registration.
- Preserve warning/error preview ends and cache toast font/text/extents instead
  of rebuilding them on every draw; full notification details stay unchanged.
- Pass frozen-source full/race suites on macOS (9,457 named passes /29 skips
  each), Linux (9,396 /37) and Windows (9,245 /43), plus four strict real
  bundled-gopls checks per platform and 400 focused race passes. Source exports
  match all 2,811 frozen files /2,065 Go files. CI-scoped vet passes; full vet
  retains 501 unchanged baseline diagnostics, with no new warnings.
- Verify actual macOS/Windows first-layout quit prompts, visible read-only
  failure causes, refusal without data loss, Undo/Redo, cancellation and normal
  exit 0. macOS additionally verifies full-error clipboard readback and actual
  multi-file long-name wrapping/Save All. Refresh README with three original
  current-source captures. This is not a new binary release, full DPI/scrolling
  support, universal performance guarantee or Qt Creator feature parity.
  [Verification and limitations](LAYOUT_MEASURE_PROGRESS_20261116.md).

## Unreleased — asynchronous save and owned document transitions (2026-11-16)

- Add UI-only save receipts, Undo epoch checkpoints and independent once-only
  document writers. Separate physical write success from current UI adoption;
  keep newer text/history and reject stale path/edit/owner completion.
- Add a bounded side-effect queue: one active physical task plus one latest
  queued task; cancellation/retirement cannot free blocked I/O or erase a real
  committed result. Preserve a failing completion-reentry replay and its fix.
- Capture typed code inputs without live widgets or scene references. Defer
  handler parsing, formatting and split-file generation to a worker; preserve
  machine ownership, developer source/stubs/import checks and output compatibility.
- Pair primary and companion inputs in the owned internal save API; retain the
  physical slot through companion completion and report companion failures as
  warnings, including panics after primary success.
- Connect toolbar/File/shortcut/palette/source-tab Save to actual owned admission;
  format source on the worker and validate observed content baselines inside the
  atomic writer. Advance pure physical baselines even if UI adoption was retired.
- Sequence Save All documents without silently superseding unrelated targets;
  preserve newer input and stop remaining targets after a conflict/change.
- Serialize recovery and manual cleanup with primary/companion writes. Preserve
  a recovery changed during primary I/O and keep cleanup errors as warnings.
- Reserve outputs through both physical completion and UI publication; refuse
  conflicting rename/search/close/quit and protect edited generated buffers.
- Add captured async tab-close/New/Open/native-close/shortcut-quit continuations.
  Wait for prior writes and the UI gaps of an already-pending Save All; post and
  revalidate one-shot teardown, preserving later text, command/draft revisions.
- Inspect Save As targets in the background and require explicit replacement
  consent. Match the reviewed fingerprint inside the writer; cancel/retire never
  dispatches late writes. Keep replacement confirmation defaulted to Cancel.
- Seal typing groups at source Save admission, not late completion. Preserve
  CRLF and actual saved baselines during batch-close, including newer input;
  reject read-only replacement plans before any transaction staging.
- Remove Git refresh's UI-thread module walk. Refuse build/run/test/vet/coverage
  during pending save/document operations, without implying automatic saving.
- Give restore/save conflicts a terminal retry state instead of a stranded loading
  page; reject manual-open stale snapshots and retire an opener's own save guard.
- Synchronize design Save/Open/New/restore window titles at completion instead
  of waiting for the 500 ms poll. Reject nested noncomparable draft-version
  values without a checkpoint-comparison panic.
- Refresh the labeled ownership diagram and add unedited final-candidate native
  macOS/Windows captures, including default-Cancel replacement review.
- Pass exact frozen-source full/race suites: macOS 9,438 named passes /
  29 skips each; Linux 9,377 / 37 each; Windows
  9,226 / 43 each. Four strict real bundled-gopls checks pass on each.
  Add 1,480 focused race passes, native GL transfer/pure-Go rendering and script
  checks; independent source exports match all 2,803 frozen files / 2,060 Go files.
- Native macOS verifies late overwrite conflict/default Cancel and quit-save
  refusal/retry; Windows verifies read-only/sharing refusal, dirty quit Save All,
  Undo/Redo and retry. Disk/model/session readback and normal exit 0 pass.
  CI-scoped vet passes; full-repository vet retains 501 unchanged baseline
  diagnostics (no new). Preserve setup failures and an aborted initial Mac
  startup separately. This is a verified development increment, not a package
  release or whole-product/performance-completion claim.
  [Verification and limits](ASYNC_SAVE_PROGRESS_20261116.md).

## Unreleased — fail-safe document save foundation (2026-11-16)

- Add opt-in plain/gzip atomic TDoc saving without changing legacy direct-write
  APIs: same-directory staging, sticky write errors, flush/sync/close checks and
  platform snapshot replacement, never delete-then-rename.
- Preserve existing permissions; reject read-only/non-regular destinations and
  detect identity/content/permission changes during preparation before replacing.
- Add error-returning UI-only `GedScene.SaveTo`; adopt Save As identity and clean
  marker only after success. Reject draft-callback retarget/reentrant saves and
  keep committed drafts undoable. Editors and recovery writes use safe staging.
- This is a synchronous safety prerequisite, not completed asynchronous Save or
  Qt Creator parity. [Verification and remaining work](SAVE_SAFETY_QA_20261116.md).
- Pass frozen-source full/race gates on macOS, Linux and Windows, 12 strict real
  bundled-gopls checks and 450 focused race passes. Native macOS/Windows verify
  refused read-only saves, unchanged originals, successful retries, Undo/Redo,
  Unicode source/session readback and normal exit 0. Windows also verifies a
  write-sharing handle that denies replacement. Add two actual README captures;
  retain intermediate failures and existing vet warnings in the QA report.

## Unreleased — UI-admitted design hot reload (2026-11-16)

- Add SDK invalidation-only notifications while preserving legacy AST reload,
  serialized callbacks, bounded coalescing and permanent Close/physical Wait.
- Replace discarded worker AST plus duplicate UI reads with UI-owned admission,
  one bounded preparation reader shared with manual Open, and prepared TDoc apply.
- Coalesce watcher/UI bursts through revocable mailboxes; guard scene/Undo,
  drafts, focus/capture, lifetime, path and manual-open admission priority.
- Preserve own-save widget/Undo identity, exact designer metadata/handler bytes
  and hidden-editor input without recovery prompts or automatic workspace changes.
- Pass frozen-source full/race suites on all three platforms and 12 strict real
  bundled-gopls checks. Verify macOS/Windows clean external reload, own-save Undo,
  dirty-design protection, hidden-editor input, saved model/session and exit 0.
  Add actual native README captures; retain failures and limits in
  [reload QA](DESIGN_RELOAD_QA_20261116.md). Save/auto-save transactions and
  resource construction are not claimed nonblocking by this increment.

## Unreleased — owned design opening and bounded canvas (2026-11-16)

- Read/decompress/parse primary and autosave .silkui snapshots off the UI loop;
  keep one physical reader and one replaceable latest queued target. Cooperative
  timeout/cancel never starts replacement readers while OS I/O remains blocked.
- Keep the old scene editable and its session identity authoritative until real
  installation. Reject changed design/Undo, drafts, input/capture, workspace and
  rename/New/close ownership; recheck after recovery dialogs.
- Support explicit recovery of missing/damaged originals without creating the
  original file or installing an empty scene after rejection/read errors.
- Defer recovery choice installation past native modal cleanup/focus return;
  preserve explicit Yes/No choices across an unowned canvas focus gap without
  changing another field's focus or bypassing edit/ownership guards. Add
  UI-only SDK PostAfterNativeCleanup without polling or worker-thread joins.
- Add Cancel Design Open in File/palette and owned status feedback; add SDK
  GedView.IsClosed/SubscribeClosed with idempotent close and detached handles.
- Bound line/dot grid work and ruler allocation to visible spans with paint-only
  decade LOD; preserve authored sizes, snapping and exact normal Cairo raster.
- Check transformed device coordinates too: intermediate forms at high zoom
  must not evade the huge-path guard.
- Prevent huge page/background/clip/line coordinates from overflowing Cairo and
  silently dropping visible content; reject nonfinite geometry and zoom.
- Pass final-source full/race suites on macOS, Linux and Windows, plus 12 strict
  real bundled-gopls checks. Verify macOS/Windows recovery, exact saved drag,
  external reload, huge canvas, input preservation and normal exit 0; add actual
  README screenshots. See [manual design QA](MANUAL_DESIGN_OPEN_QA_20261116.md).
  Save, resource construction and other callback I/O remain separate work.

## Unreleased — asynchronous source opening and navigation (2026-11-15)

- Move interactive source reads off the UI loop. Use owned inert loading/error
  tabs, deduplicated requests, 128 admitted jobs, two physical readers and a
  32 MiB source limit. A soft deadline or Cancel never spawns replacement readers.
- Install only still-owned snapshots; preserve dirty buffers/Undo, newer user
  intent, another field's focus, requested positions and pending-session metadata.
  Release canceled queue entries immediately even behind stuck OS reads.
- Migrate quick-open/locations, LSP/search/bookmarks, reopen and generated-source
  entrypoints. Record MRU/history only after real readiness; sequentially skip
  unavailable history destinations rather than committing a pending request.
- Add File/palette Cancel File Opens, with bound-owner guards against stale
  commands. Retire pending old-path pages on rename and worker mailboxes on close.
- Add SDK PrepareNavigation / CanCommitNavigation / CommitNavigation, readonly
  CurrentPosition and CurrentFocusWidget. Reject stale/foreign/ABA/copied visits;
  keep legacy history allocation-free when the new API is unused.
- Add blocked/reordered/canceled/error/focus/owner regressions and migrate legacy
  completed-state fixtures through a test-only owned dispatcher, never a
  synchronous production fallback. Full platform/native evidence and retained
  limits are recorded in [interactive-open QA](MANUAL_OPEN_QA_20261115.md).
- Pass frozen-source full ordinary/race on macOS, Linux and Windows, 12 strict
  bundled-gopls probes, native macOS/Windows keyboard/navigation/error replays,
  independent saved-source/session readback and normal exit checks. Preserve
  corrected Windows staging/exit-observer harness failures, not false approvals.
- Add two unmodified native Windows README screenshots, SDK lifecycle guidance
  and the 2,511-runtime/build-asset publication identity audit.
- Manual design reads, Save, resources and macro/reload callback I/O remain
  separate work; this is not complete Qt Creator parity or a universal CPU fix.

## Unreleased — bounded restore and live-update lifecycles (2026-11-15)

- Build bounded asynchronous startup restoration with owned loading pages,
  source/scene identity guards, soft deadlines and pending-session persistence.
  Preserve source bytes, Undo, location, MRU and user-selected workspace modes.
- Add SDK tab-page replacement and captured file-state watcher registration;
  retire late watcher results and provide nonblocking Close / physical Wait.
- Correct reentrant page-selection and empty/legacy startup mode regressions.
- Serialize Reloader callbacks on one fixed worker; coalesce latest input,
  bound pending paths, expose overflow and retire closed/unwatched generations.
  Add nonblocking Close / bounded physical Wait and correct concurrent Stop joins.
- Defer late autosave prompts after user interaction. Explicit menu/palette review
  rereads current files and guards pending property drafts and changed owners.
- Correct initial keyboard focus after asynchronous restoration without stealing
  another input's focus; transfer it when a missing active source is skipped.
  Reject the first frozen native candidate and rerun final-source gates.
- Own and coalesce AlarmPanel database bindings. Rebinding retires the old
  source; queued old updates and old cancellation handles cannot overwrite or
  cancel a replacement. Add explicit nil detach without a timer/per-event worker.
- Release retired source/subscription/owner references, including held cancel
  handles. Preserve the last visible rows on cancellation and keep UI access on
  UI; an already admitted refresh may finish. Repeat alarm races 100 times.
- Verify the combined source with complete ordinary/race suites on macOS arm64,
  Linux arm64/Xvfb and Windows amd64, plus 12 required bundled-gopls probes.
  Replay Windows restore/save/reload/error controls and alarm lifecycle, and
  macOS restored input/save/design reload; independently verify normal exits,
  source/settings/state readback and all 2,012 publication build inputs.
  Refresh README with exact combined-source Windows captures.
  Previous startup-only failures/results and the corrected Windows strict staging
  harness remain retained evidence, not silently discarded or reused as gates.
  See [alarm ownership QA](SDK_ALARM_LIFECYCLE_QA_20261115.md).
- Keep native IME, remaining manual I/O, all-menu and full product-parity limits
  explicit in [verification and scope](STARTUP_IO_QA_20261115.md).

## Unreleased — source-backed process runtime inside SilkIDE (2026-11-15)

- Add an inert, embeddable `hmiapp.RuntimeView` with alternate live diagram/point
  views, shared background SQLite/acquisition ownership, explicit Start/Stop and
  off-loop Wait. Preserve standalone workbench APIs and generated applications.
- Run the exact current `.silkhmi` buffer from Tools, palette and F5/Run in an
  owned IDE dock. Isolate stores by source identity; keep source, dirty state,
  Undo and file persistence separate from runtime point values.
- Cancel stale loads and retire sessions on source edit/close/rename, tool hide
  and frame shutdown. Reopening a tool or Undo never silently restarts HTTP.
- Add opt-in widget-tree visibility notifications without broadcasting local
  OnShow/OnHide or adding polling. Hidden ancestors now retire runtime work.
- Fix new snapshots retaining the default template's camera scale; keep the
  existing HTTP-error backoff and wait for confirmed recovery in native replay.
- Document the SDK, lifecycle boundaries and actual Windows IDE workflow; final
  native/platform gates are recorded in the iteration QA report.
- Pass final-source full ordinary/race suites on all three platforms, 12 strict
  real bundled-gopls probes and corrected Windows IDE/SQLite replay. Audit all
  2,004 build/resource inputs and add actual point/diagram README captures.
  Keep the unresolved macOS-native startup observation explicit in
  [the QA report](IDE_PROCESS_RUNTIME_QA_20261115.md); automated passes are not
  native keyboard/IME approval.

## Unreleased — industrial live-point monitoring and guarded writes (2026-11-15)

- Add atomic tag sample/revision snapshots, memory-only runtime point views,
  quiet change subscriptions and opaque runtime-local optimistic write tokens.
- Add typed manual writes with HTTP-source ownership checks and durable-before-
  live publication. Reject stale/foreign/ABA baselines; do not retry ambiguous
  failures automatically. Direct `Tags` mutation remains outside this contract.
- Add optional bounded recent/range history APIs without changing the existing
  SQLite `StoredSample` type. Validate restored samples and custom-reader output.
- Add the inert designer-placeable `ProcessPointMonitor`: stable live selection,
  exact int64 sorting/display, quality markers, owned explicit write/history
  dialogs, bounded background jobs and close/rebind cancellation guards.
- Integrate the standalone workbench's Live points panel and event-driven
  `RuntimeChanged` hook. Hide inactive authoring controls in live mode so they
  cannot cover the diagram viewport. No new widget polling/acquisition timer.
- Verify native Windows confirm/cancel/invalid/conflict/read-only/history actions,
  HTTP body/transform/failure/recovery, normal exit and independent read-only
  SQLite persistence. Add actual Windows README images and SDK documentation.
- Pass final-source full ordinary/race suites on macOS, Linux and Windows, plus
  12 strict real bundled-gopls probes. Audit all 1,997 compiled/resource inputs.
- Keep full platform-suite results and macOS native-observation limitations in
  [the QA report](LIVE_POINT_MONITOR_QA_20261115.md). This is not an alarm engine,
  PLC write protocol, access-control system or complete Qt Creator parity.

## Unreleased — exact incremental local completion candidates (2026-11-15)

- Re-lex changed lexical chunks and neighbours instead of rebuilding the whole
  local candidate source on every revision. Preserve static precedence, lexical
  first-occurrence/dedup order, distant candidates, fuzzy ranking and providers.
- Bound the optional sidecar to 2 MiB source / 32,768 stored chunk words;
  larger inputs use the complete scanner, never a truncated candidate window.
  Handle repaired invalid UTF-8 seams and canceled/coalesced edits exactly.
- Own identifier bytes and reuse only private worker scratch. Keep deliveries
  independent; retire prior cache aliases, clear removed tails, release on
  shrink/language/close, and preserve UI generation/caret/open-intent guards.
- Matched Windows release controls reduce 45-second cumulative allocation
  371–372 → 175–176 MiB for body input and 349 → 155 MiB for comments. Body
  IDE CPU time falls 7–10% in both run orders; gopls CPU has no consistent gain.
- Add final-source differential fuzz (14,495 sequences), scanner parity,
  ownership/budget/cancellation and editor Undo/Redo/lifecycle regression.
  Update SDK docs, native README capture and explicit performance limits.
- Pass complete ordinary/race suites on macOS arm64, Linux arm64/Xvfb and
  Windows amd64, 12 strict real bundled-gopls probes and seven Windows native
  controls with independent source readback and normal process exits.

## Unreleased — exact bounded function-body symbol projections (2026-11-15)

- Reuse exact Go outline symbols for proven function-body edits, using Go's
  lexer rather than brace substring matching. Preserve physical positions and
  full tolerant-AST fallback for newline, signature, boundary or uncertain edits.
- Bound synchronous body scanning to 8 KiB and immutable span copying to
  16,384 entries. Share unchanged metadata and retire stale internal comment
  offsets; no new polling timer or longer completion delay is introduced.
- Prime projections through synchronous SDK queries as well as background
  indexing. Preserve ready rows, selection and expansion through body edits,
  Undo/Redo; cover provider/language invalidation and safe close ownership.
- Matched Windows body-string replay in both run orders reduces IDE typing CPU
  31–37% and 45-second cumulative allocation 1,177 → 372 MiB. The comment-only
  control and gopls CPU do not show a gain; do not generalize this to FPS/latency.
- Add 202,927 differential fuzz executions, exact rendering/navigation and
  metadata-budget regressions. Refresh the native Windows README image and
  document measured results, source identity and remaining acceptance limits.
- Pass full ordinary/race suites on macOS arm64, Linux arm64/Xvfb and Windows
  amd64; 12 strict real bundled-gopls probes; seven Windows native controls
  and macOS SDK mouse Edit/Undo/Save with independently saved state.

## Unreleased — negotiated incremental LSP synchronization (2026-11-15)

- Add SDK `LSPContentChange`, `ComputeLSPTextChange`, `SupportsIncrementalSync`
  and `DidChangeFrom`; keep the existing full-text `DidChange` wire contract.
- Enable deltas only for explicitly negotiated UTF-16 incremental sync. Preserve
  rune/CRLF boundaries and conservative full fallback for unsupported or unsafe
  inputs; no new idle polling or longer completion debounce.
- Keep the last successfully sent source across coalesced edits and explicit
  flushes. Failed sends do not advance it; close releases retained wire text.
  Full-only custom document transports remain compatible.
- Matched Windows release controls reduce cumulative allocation 462 → 349 MiB
  (comment input) and 1,367 → 1,175 MiB (function-body string input); do not
  interpret the small CPU differences as a confirmed general speedup.
- Add strict wire/capability, differential/fuzz, failed/in-flight-send lifecycle
  and real-gopls unsaved Unicode/CRLF regression; require four real probes in
  the bundled language-service test script. Document the additive SDK API.
- Verify full ordinary/race suites on all three platforms, 12 strict real
  bundled-gopls probes, 201,721 text-diff fuzz mutations and no-CGO external SDK
  consumers on macOS/Linux/Windows. Refresh the actual Windows gallery image.

## Unreleased — incremental symbol projections and outline snapshots (2026-11-15)

- Add nonblocking `CodeEditor.SymbolsSnapshotSince`, with editor-local content
  identity, owned changed snapshots and no copying when declarations are equal.
- Matched uninstrumented Windows release replay of EOF line-comment input:
  IDE typing CPU 19.42 →
  12.42 seconds; cumulative Go allocation over 45 seconds 1,941 → 483 MiB.
  These are workload-specific measurements; gopls CPU is not improved.
  Function-body/string controls in both run orders reduce allocations 31–33%
  but do not demonstrate a CPU speedup.
- Reuse exact Go symbol projections for proven line-comment payload edits.
  Keep the complete tolerant AST fallback for signatures, directives, newlines,
  invalid encoding and structural/uncertain edits; preserve physical positions.
- Publish immutable metadata so safe UI edits do not race a worker or flash an
  indexing placeholder. Retire private large-source snapshots on close/shrink.
- Reuse unchanged outline trees, selection and expansion. Gate retained pending
  rows from painting, hit testing and keyboard navigation; reset editor/provider
  identity correctly. Preallocate full-parser declaration/result storage.
- Add differential/fuzz, snapshot-ownership and lifecycle/race regression, and
  document the SDK consumption pattern and actual native performance evidence.
- Verify full ordinary/race suites on macOS arm64, Linux arm64/Xvfb and Windows
  amd64, nine strict real bundled-gopls probes, and 181,886 differential fuzz
  executions. Add the final native Windows screenshot and explicit skip counts.

## Unreleased — device-pixel editor overview cache (2026-11-15)

- Cache the minimap at native device resolution and repaint only changed
  sample bands; scrolling keeps its viewport indicator live without redrawing
  thousands of unchanged translucent token rectangles.
- Handle fractional dock/layout coordinates without rounding the UI or scaling
  a low-resolution bitmap. Retain original Cairo clipping/alpha rounding with
  guarded scratch pixels and vector-rendered fractional edges.
- Bound the two native raster surfaces to 8 MiB per editor; release them when
  the editor is hidden, reparented/detached or the minimap is disabled.
- Preserve uncached paths for custom/vector painters, unsupported transforms
  and partial parent clips. Track rectangular-clip eligibility across paths,
  transforms and save/restore without extending `Painter` or approximating
  complex masks by their bounds. Add independent exact-pixel regressions across
  both themes, five DPI scales, fractional origins, edits and syntax changes.

## Unreleased — pump-owned diagnostics and SDK cleanup (2026-11-15)

- Own UI stall diagnostics from actual main, modal and drag message pumps.
  Nested pumps share one worker; last-owner exit cancels polling, and later
  pumps get fresh state rather than inheriting a stale deadline.
- Use a process-relative monotonic heartbeat clock. Retain the five-second
  blocked-work detector, native-dialog wait scopes and recovery diagnostics;
  do not wait on arbitrary user logging callbacks from the UI thread.
- Fix false post-pump stall warnings reproduced in native baseline/candidate
  controls on Windows, macOS and Linux. Deliberate active UI blocking remains
  detectable; this is not a blanket CPU/FPS or startup-latency fix.
- Close the processlibrary example's framework resources/run marker on success,
  file failures, invalid arguments and help. Preserve operation/usage exit codes
  and no-overwrite semantics; add real child-process restart/error-path tests.

## Unreleased — reusable 2D equipment library (2026-11-14)

- Add immutable subgraph capture/placement SDK, default independent point
  prefixes and explicit compatible point reuse. Remap all node/property/pipe
  bindings together; reject partial, conflicting or out-of-bounds insertions.
- Add three schematic device templates and an owned searchable library with
  inert preview, selection capture, point mapping and one-step insert Undo.
- Add strict bounded `.silkhmilibrary` load/save on cancellable owned workers,
  no-replace publication and explicit overwrite confirmation. Reject stale UI
  completions; retire nested dialogs when the editor closes or becomes locked.
- Persist custom catalog configuration in ProcessEditor's designer property.
  Fix its missing concrete Go setter and verify concrete, scene-declarative and
  silkgen output in an independently compiled/running consumer module.
- Add a no-CGO assembly example and persisted project/catalog readback tests.
  Templates never clone acquisition sources or live history; placed content
  remains ordinary editable nodes, not linked prefab instances.
- Fix Table's internal scroll-area child painting a debug placeholder over
  rendered cells. Add a transparent-overlay raster regression, including debug
  and pure-Go rendering runs.
- Add extension-specific native open/save SDK dialogs on Windows, Linux and
  macOS. Device catalogs no longer inherit the form-only `.silkui` save filter
  and unwanted suffix; legacy form-dialog behavior remains unchanged.

## Unreleased — IDE process-document coordination (2026-11-14)

- Recognize `.silkhmi` as JSON source and add Tools / command-palette **Edit 2D
  Process...** with actual document-sensitive availability and an existing
  vector preview icon.
- Stage visual changes in an owned, resize-fitted modal editor. Nested point and
  binding tables stay inert; Cancel does not write or dirty the source.
- Apply changed drafts as one ordinary host buffer undo transaction, preserving
  source line endings and reusing Save All, dirty-close guards and recovery.
- Decode on an owned bounded worker; reject late results and stale commits after
  source identity/revision/read-only changes or conflicting file operations.
- Paint ProcessEditor's own theme background instead of inheriting a bare
  Widget's debug placeholder, keeping the toolbar/footer readable in light/dark
  themes and debug-enabled builds, including modal wrappers.

## Unreleased — precise 2D group authoring (2026-11-14)

- Add validated, non-mutating `hmi` editing operations for group movement,
  resizing, alignment, equal-gap distribution, layering, subgraph duplication,
  deletion and pipe-waypoint editing. Preserve exact scalar initial values.
- Add Shift/Ctrl/Cmd multi-selection, directional marquee, corner resizing,
  keyboard nudging/duplication/history and an owned Arrange menu. Preview a
  gesture transiently and commit one undo step; cancel restores widget geometry.
- Move internal pipe waypoints with a device group; select pipes and drag,
  insert or remove authored waypoints without unintentionally deleting a pipe.
- Separate live-bound geometry/visibility from editable authored geometry;
  separate runtime preview from temporary I/O read-only locking.
- Reflect Undo/Redo, selection, runtime and I/O availability in actual controls.
  Preserve string values, labels and HTTP body whitespace; reject misspelled or
  trailing advanced-pipeline JSON rather than silently ignoring it.

## Unreleased — industrial 2D authoring and typed point runtime (2026-11-14)

- Add ProcessDiagram / ProcessEditor and the opt-in `cmd/silkhmi` workbench:
  vector equipment/value/text nodes, snapped dragging, connected ports,
  orthogonal routes/waypoints, zoom/pan, bounded undo/redo and Go generation.
- Add staged visual tables for points, HTTP sources/mappings/headers, node
  bindings, conversions, color/state rules, dynamic properties and pipes.
  Preserve advanced pipelines and reject invalid references without mutation.
- Add portable inert `.silkhmi` documents, atomic save/validated load and
  designer drag/drop, persistence, preview and code-generation coverage.
- Add strict scalar conversion, a bounded non-executing expression language,
  shared cancellable HTTP sources and optional typed SQLite latest/history.
  Restore persisted points; provide SDK publishing, TTL and bounded retention.
- Keep I/O off the GUI thread, join workers on stop/close, coalesce Tag bursts
  against current state, and separate authored previews from acquired samples.
  Bad/Uncertain samples do not masquerade as valid zeros.
- Fix table header backgrounds erasing the first caption; clip column captions.
  Let table body clicks pass through the internal scroll layer while keeping
  scrollbar/editor input intact, including nested modal configuration tables.
  Correct equipment nozzle/pipe alignment and avoid parsing documents per drag.
- Document actual UI screenshots, workflow, tests and industrial limitations.

## Unreleased — PDF editor feedback and LSP reliability

- Fix Windows indentation tofu by displaying tabs at consistent tab stops across
  syntax runs, caret/selection and hit testing; preserve source tab bytes.
- Draw bounded multiline tooltips instead of rendering newline/tab glyphs.
- Enable system DPI awareness before Win32 windows and use full grayscale font
  hinting; mixed-DPI per-monitor transitions remain a limitation.
- Add persistent Tools / command-palette Appearance & Editor settings for
  English/Chinese (restart), fonts, light/dark theme, tab width and custom colors.
  Normalize IDE-owned dock captions to the selected language.
- Coalesce gopls updates in one ordered worker per document; cancel superseded
  completion requests, reject stale diagnostics/results and retire old sessions.
- Apply completion replacement/auto-import edits atomically with undo/redo caret
  restoration, prioritize provider candidates over local guesses, request
  plain-text completions, flush unsaved text before explicit commands and escape
  file URI reserved characters. Bound shutdown against a non-reading server.
- Keep long completion signatures inside their own column, including Windows
  CJK fallback fonts; use opaque, clipped popups and rune-safe elision.
- Prevent Win32 Ctrl+Space from inserting a space and dismissing completion;
  preserve AltGr input. Separate informational/hint diagnostics from errors.

## Unreleased — satin sprites, HMI and compile-time macros

- Embed a generated 16-icon satin atlas, calibrated object crops, bounded
  device-size caches and neutral disabled masks. Keep structural vector glyphs
  and a `SILK_ICON_STYLE=vector` IDE opt-out.
- Give enabled idle buttons a cached material gradient and inset border.
- Add designer-ready SpriteView and ProcessValue widgets and Gauge TagName;
  preserve properties across drag/drop, save/load, preview and Go generation.
- Fix Gauge's hard-coded white background under dark themes, use theme-aware
  ticks/readout, clip to widget bounds, reuse fonts and remove duplicate end
  labels. Give the HMI example labeled buttons and an embedded window icon.
- Coalesce visual Tag bursts and invalidate cancelled updates/animations instead
  of allowing an unbounded UI callback backlog or animation after unbind.
- Add explicit `.silkgo` compile-time textual macros, complete-Go validation,
  generated-file write guards, `silkgen -macro`, IDE Tools/palette actions and
  runnable examples. Go remains the compiler; macros are not hygienic/recursive.

## SilkIDE v0.1.5 / SDK 2.5.1 (2026-10-20)

- Include the race, scheduling, font and visual-quality fixes listed below.
- Fix Windows DLL packaging under minimal MSYS2 / inherited Git Bash paths:
  compare runtime and staged DLLs using the active environment's SHA-256 tool
  and stdin, rather than relying on missing or foreign-runtime `cmp`.
- Keep missing, foreign, corrupt and conflicting DLLs as hard packaging errors;
  add regressions with an unusable `cmp` and remove diffutils from fixture tests.
- v0.1.4's tag is retained for audit; its Windows packaging failed and no Release
  was published. v0.1.5 is the corrected release, not a rewritten v0.1.4 tag.
- Windows system-DPI virtualization remains a known limitation; see release notes.

## SilkIDE v0.1.4 / SDK 2.5.1 (2026-10-20)

### Race safety, rendering and UI scheduling

- Serialize stdout/stderr progress callbacks and join the elapsed-time reporter
  before completing a build; stale heartbeats cannot overwrite final results.
- Bound each GUI callback turn to 64 tasks / 4 ms (soft budget); preserve queued
  work, FIFO remainder order, panic isolation and nested-modal dispatch.
- Reserve Win32 input, paint and timer turns during callback bursts while keeping
  ordinary idle and modal message pumps blocking; preserve quit/error handling.
- Join in-flight GLFW native wakeups before backend termination.
- Cancel Windows task-runner process trees by PID without killing unrelated
  same-name tasks; bound inherited stdout/stderr reads after cancellation even
  when a parent has already exited. Retire the pipe-cancellation worker before
  returning, and do not report cancelled reads as success.
- Stop sharing mutable OpenType faces between independent pure-Go renderers.
  Share parsed font tables instead; pool only temporary glyph callback buffers.
- Fix pure-Go exact sRGB and premultiplication, path/current-point restoration,
  empty glyph-buffer clearing, dashed strokes, transformed stroke widths, and
  nested group/alpha masks for disabled icons. The optional backend still has
  documented clipping/compositing/shaping limitations.
- Add Windows native missing-glyph fallback for simple LTR text, keeping
  measurement, drawing and IME cursor geometry aligned. Native file dialogs no
  longer generate false UI-watchdog stall reports while awaiting user input.
- Select raster and custom icon resolutions from the device transform instead
  of logical size; preserve fractional layout dimensions without upscaling
  a low-resolution source when a suitable asset is available.
- Use grayscale Windows offscreen font antialiasing to avoid LCD color fringes.
  Fix double-alpha darkening in Cairo-to-OpenGL presentation; native readback
  now verifies final displayed pixels as well as texture uploads.
- Bump SilkIDE to 0.1.4; add native/race/pure-Go release gates and archive checksums.


## v2.5.0 (2026-07-27)

### IDE — Qt Creator-class depth

Engines and panels closing the gap to Qt Creator. Editor engines and models are
pure and GL-free; wiring them into the editor/designer/IDE hotspots is the next
step.

- **Editor engines** — multi-cursor with per-cursor selections, rectangular
  (column) blocks, sticky columns and bottom-up edit ordering; a visual-line
  wrap layout with lossless source↔visual mapping; a `go/scanner`-based fold
  scanner that no longer counts braces inside strings or comments and folds
  import and comment blocks; a find/replace model with regex, case, whole-word,
  in-selection, wrap-around and preserve-case replace
- **Shared documents** — a `Document` with revisions, dirty tracking and
  edit-remapped anchors, an editor workspace with real split groups over one
  buffer (previously split cloned the text), a versioned session (ordered tabs,
  cursors, folds, splits, unsaved buffers) and a bookmark store that re-anchors
  by line context
- **LSP** — the client now advertises capabilities and workspace folders and
  **answers server-to-client requests**; previously `workspace/configuration`
  and `workspace/applyEdit` were dropped, which silently broke organize-imports
  and every command-based refactor. Completion keeps `textEdit` /
  `additionalTextEdits` so auto-import works, workspace edits decode
  `documentChanges` with versions and resource operations, and call hierarchy,
  type hierarchy, implementation, prepare-rename, inlay hints, semantic tokens
  and code lens are added. Edits apply transactionally: preflight, in-memory
  computation, all-or-nothing write with rollback, and a no-touch preview
- **Panels** — call hierarchy, type hierarchy, code insights (semantic tokens /
  inlay hints / code lens), grouped and filterable Find References, an outline
  driven by semantic symbols, a Go analyzer panel and a hierarchical test
  explorer
- **Build & run** — kits (toolchain, GOOS/GOARCH, tags, race/coverage, output,
  local or SSH deploy) persisted per project, multiple named run/debug
  configurations, and a task runner executing a dependency graph with streaming
  events, history and process-group cancellation, replacing hardcoded and
  uncancellable `go build` / `test` / `vet`
- **Testing** — results now come from the `go test -json` event stream, so
  output is attributed to the owning test and subtests nest under their parent,
  fixing misattribution of interleaved multi-package console output
- **Debugger** — clear/amend/toggle breakpoints with conditions, hit counts and
  logpoints; goroutine- and frame-scoped locals **and arguments**; lazily
  expanded variable children; goroutine pagination; debuggee output capture; and
  a debug panel with a breakpoint table, explicit goroutine/frame selection
  (watches previously always evaluated in goroutine -1 / frame 0) and a console
- **Version control** — branches, remotes, fetch/pull/push, stash, rebase,
  cherry-pick and conflict status behind non-interactive typed APIs with pure
  parsers (NUL-delimited porcelain, C-style quoted paths); a workspace panel
  grouping conflict/staged/unstaged/untracked against the real index; a
  multi-file patch model with hunk staging that no longer drops the unchanged
  lines between hunks or everything past the first file; and a diff3 merge
  engine with a conflict editor
- **Terminal & search** — a PTY-backed shell session with a full ANSI/SGR/cursor
  state machine replacing the one-shot line-oriented command runner; project
  search exposes regex/case/whole-word/include-exclude and replaces
  transactionally with preview and rollback; the TODO scanner becomes a
  configurable multi-language incremental index that no longer matches inside Go
  string literals

### 组态 / SCADA runtime

- `scada.Services` — one shared `TagDB`, `AlarmDB`, historian, event log, recipe
  book and stats collector with `New`/`Start`/`Stop` and alarm→event/notify
  wiring; `BindScreen` walks a widget tree and connects industrial widgets and
  panels through their plain-data setters (`scada` imports `gui`, never the
  reverse)
- Code generation emits `Services`-backed applications (`BindServices`, one
  shared runtime, no private `TagDB` per app) and silkide runs a live preview
  against an in-memory Services with drivers off
- Operator panels — recipe, report, event log, trend playback, statistics and
  calc — plus a device point-list editor and an end-to-end HMI example
- Redundant driver and gateway expose failover/error callbacks

### Designer & UI fixes

- Property IDs accept unicode labels — every Chinese property id (液位, 协议,
  点表 …) was previously lowercased, rejected and **silently dropped** from the
  designer
- `Table`/`HeaderView` no longer dereference an uninitialised theme face (drawing
  either one panicked)
- `SearchBox`/`NumberInput` key and text handlers had signatures matching neither
  dispatched interface, so typing was discarded; `Button` gains Enter/Space
- Designed widget properties now round-trip through `Save`/`LoadDesign`
- silkide shows a live `PropertySheet` for the selected widget, and worker
  build/test/vet output is marshalled onto the UI thread

### Icons

- 18 new icons at 16/22/32/48 in the existing flat style: `silkide`, `sandbox`,
  `platform` (silkide had **no application icon** — it is looked up by executable
  name), `diagram`, `map`, `project`, and distinct icons for the call-hierarchy,
  type-hierarchy, test-explorer, bookmarks, outline, todo, references, packages,
  keymap, help, log and merge panels (13 panels previously shared `edit`, 9
  shared `tree-view`)

### Build

- Windows CI/release and the README move from **MSYS2 MINGW64 to UCRT64**
  (`mingw-w64-ucrt-x86_64-*`)
- `win32.HRESULT` widened to `uintptr` — `syscall.NewCallback` requires a
  uintptr-sized result, so the COM drag-drop vtables panicked at startup on
  Go 1.25 (`compileCallback: expected function with one uintptr-sized result`)
- SQLite switches to the go-gettable `mattn/go-sqlite3`; the vendored fork was
  gitignored, so a clean clone could not build

## v2.4.0 (2026-07-09)

### 组态 / SCADA & Industrial Automation
- Real-time tag database (`TagDB`) with value-driven bindings, animation and alarms
- Field-bus drivers: **Modbus TCP, Siemens S7, OPC-UA, MQTT** — all PLC data types, all four register/byte orders (ABCD/DCBA/BADC/CDAB), read-only / read-write
- Driver **redundancy** (primary/backup failover), protocol **gateway** (data forwarding), **simulator** driver (hardware-free testing)
- `DeviceComponent` — widget-form device configuration placeable in the designer
- Structured-config **device templates** (batch tag instantiation)
- **Historian** (SQLite tag history), **reports** (interval aggregation, CSV/HTML), **trend playback**
- **Recipe** management, **calc/formula** tags, **event log**, live tag **statistics**
- Alarm engine + notification/event bridge; industrial widgets (Tank, Gauge, Valve, Indicator, Pipe, …)
- Runtime **Go scripting** (yaegi); **user auth** + login sessions

### IDE (silkide) — Qt Creator parity
- LSP (gopls): completion / hover / definition / references / rename / format / code actions
- Delve debugger integration
- **Locator** (fuzzy quick-open), **find-in-files**, **snippets**, **build-issue** parsing

### Build
- Go-gettable SQLite (`mattn/go-sqlite3`) — the tree now builds from a clean clone
- CI covers the full SCADA + IDE package set

## v2.3.0 (2026-04-12)

### New Widgets (20 new)
- ToggleSwitch, SearchBox, NumberInput, DatePicker, ColorPicker, Rating
- DropdownButton, SwitchGroup, Avatar, Badge, Breadcrumb, Tag
- Card, Accordion, Link, LabelSeparator, Placeholder, Timeline
- NotificationPanel, ImageView

### Visual Designer
- Smart alignment guides during drag
- Ctrl+Scroll zoom, Space+drag pan
- Object inspector, undo history panel
- Tab order editor, widget locking
- Form size presets (device profiles)
- Welcome screen, custom template saving
- Mode selector (Design / Code)
- File explorer, multi-tab editor
- Build output panel with error navigation
- Toolbar with common actions
- Theme preview panel
- Keyboard shortcut reference

### Code Editor
- Text selection, undo/redo, find/replace
- Auto-completion, code snippets
- Cmd/Ctrl+Click cross-file navigation
- Symbol navigation, Go to line
- Bracket matching, minimap, status bar
- Bookmarks, rename refactoring
- Code formatting (gofmt), error markers
- Split editor view, word wrap

### Framework
- Layout engine: stretch weights, min/max sizes, alignment
- Animation system: 12 easing functions, groups
- Style system: 5 color schemes, widget presets
- Context menu system, right-click support
- Modern theme (Tailwind-inspired colors)
- Performance: pixmap caching, backbuffer reuse
- Menu popup positioning fix (macOS)

### Testing
- 280+ automated tests
- Widget factory, layout, animation, style, codegen, persistence tests
- Benchmarks for layout and widget creation

## v2.2.0 (2026-04-10)
- Initial open source release
- 40 widgets, visual designer, code generation
- Cairo 2D rendering, GLFW/Win32 windowing
- TDoc persistence, signal-slot events
- 6 project templates, dark/light theme
