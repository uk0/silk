# Workspace startup I/O and live-update lifecycle — verified increment

Baseline: `f7a191707e748a5f9133ea2498f7ee7c9959b195`.
This report records the combined source's completed automated and scoped native
gates. It is not a claim of full Qt Creator parity or all-menu/IME acceptance.
Git publication is tracked separately from the test evidence below.

## Publication identity update

The combined source includes the AlarmPanel correction described in
[live-alarm lifecycle QA](SDK_ALARM_LIFECYCLE_QA_20261115.md). Its archive SHA256 is
`c9736e95165ea1a800659b0794acecd9277af7c60c3cf527407409b73b18d969`:
2,717 files / 1,990 Go files / 2,012 publication build inputs. Every startup-only
result below remains historical evidence for its exact source, not approval of
the new combined code. Transport loss is not a job verdict or reason to restart.

| Combined source | Ordinary pass / skip | Race pass / skip | Strict bundled gopls |
| --- | ---: | ---: | ---: |
| macOS arm64 | 8,915 / 29 | 8,915 / 29 | 4 pass, 0 skip |
| Linux arm64 / Xvfb | 8,854 / 37 | 8,854 / 37 | 4 pass, 0 skip |
| Windows amd64 | 8,705 / 42 | 8,705 / 42 | 4 pass, 0 skip |

The completed platform runners exit zero. Independently read-back source trees
match all 2,717 manifest hashes, with no extra Go. Windows has completed the
combined executable's keyboard/save/session, external design reload, oversized
F5 negative/runnable positive control and alarm UI replay; all three windows exit
normally with code zero. Eight workspace and five alarm captures were inspected
against source/settings/state readback. README images are exact combined-source
captures, not the earlier startup-only executable. The completed combined-source
ordinary/race, strict and source-verification runners all exit zero. The current
macOS native replay below passes the basic restore/input/save/reload/exit flow;
IME and all-menu coverage remain open. Counts are named test/subtest events, not
assertions; ordinary suites retain their platform/environment-dependent skips.

The first combined Windows strict runner fails before probe compilation: it
points `-verify` at `src/tools`, the tracked pinned-tool **source** directory,
where no gopls.exe is expected. A separate diagnostic reproduces the exact missing
file error. The corrected runner verifies the native staging directory's tools
and builds its probe beside them, outside SDK source enumeration, with the same
four required tests and four-minute timeout. All four pass without skips. Both
failed harness runs/logs remain retained; this is not a product-code correction.

Post-run Windows readback independently matches every manifest hash and contains
no extra Go. The IDE binary's pre/post SHA256 is
`b0fec979b6184c621829047e86ce91f21ac4d3130d4eae8ac5e718684717b1a7`.
Only explicitly audited documentation/screenshots change after the source freeze;
all 2,012 staged and working compiled/native/resource inputs match tested bytes.
The existing noncompiled `core/callback.go.txt` LF/CRLF difference is preserved,
not staged as a code change. Unrelated review/generated/user binary files stay out.

## Current combined-source macOS native replay

A fresh V3 app bundle runs the frozen payload with launcher-set private HOME,
data directory and cwd, Go 1.26.10 and the matching verified gopls bundle. This
time CUA successfully captures the real restored window. The earlier -3811
captures remain historical failures; no OS permissions or personal preferences
were changed, and no cause of the capture recovery is inferred.

- Before any mouse click, Super+End, Return and native typeText insert the exact
  `// macos saved after async restore` marker. The visible title becomes dirty;
  an independent SHA256 check confirms unchanged disk bytes before Super+S.
- Super+S removes the dirty title. An independent helper parses the saved source
  and reads its marker. This is real text input, not a save-only formatting test.
- Super+3 displays the restored design. An external file write changes the
  visible button from Restored to Reloaded; the saved Go source hash is unchanged.
  Super+2 returns to the editor.
- Super+Q ends the owned process. A separately registered kqueue NOTE_EXITSTATUS
  observer reports wait_status 0, exited 1, exit_code 0, no signal. A subsequent
  AX procNotFound error is post-exit, not the exit-status evidence. Fresh settings
  readback contains a.go then b.go, active b.go, line 29, scroll 25 and Edit mode.

The CUA screenshots are retained in the tool conversation, not exported as
repository PNGs. Local identity/readback/exit evidence is in
`verified3/mac-native-v3/`. This closes this basic keyboard replay gap, not
Chinese IME composition, source-runtime behavior or every menu/native workflow.

## Historical initial runtime evidence

The previous main-thread dump showed `restoreWorkspaceSession` ->
`loadFileInEditor` -> `os.ReadFile` before the event loop. The input was a regular
3,772-byte file, not FIFO. Neither a FIFO fix nor an exact OS/provider diagnosis
was justified by that sample.

This iteration launches the unchanged macOS baseline through a separate tiny
native QA launcher which sets HOME, data directory and cwd before exec. This
actually isolates process settings; Info.plist LSEnvironment alone had not proved
that. Its payload hash equals the previous final macOS executable:
`cf23fd7a86d73de340d1b6dc325ffbe1c3b6e29d2149986a7cce959653ea737f`.
The own-process sample is in GLFW/AppKit's idle event wait, not `open`. CUA reports
`ScreenCaptureKit.SCStreamErrorDomain -3811` (audio/video capture failure).
Thus capture failure and restoration I/O are separate observed problems. The
launcher inherited Go 1.26.5 under GOTOOLCHAIN=local, producing a go-list warning;
that QA PATH needs correction before candidate editor/LSP acceptance. No personal
preferences or OS permissions were changed; only the owned QA process was ended.

## Implemented changes

- Restore metadata is filtered/canonicalized without UI-thread stat/read calls.
  Loading pages preserve order and saved cursor/scroll metadata on first paint.
  Two worker slots read immutable file/optional recovery snapshots, decode TDoc
  in the worker and deliver only into their exact still-owned page/scene.
- A closed/loading source cannot become an editable empty file. Missing/directory
  inputs are skipped after background inspection; other failures remain visible.
  Manual opens, edits, Welcome switches and source replacement beat late results.
- A 30-second soft deadline reports stalled I/O. Cancel or close revokes UI
  delivery; it does not falsely claim an OS syscall was forcibly interrupted.
  Blocked slots remain occupied until the physical call returns, with no
  replacement-thread loop. Frame teardown does not join them on the UI thread.
- Pending source metadata is captured before frame teardown and persists through
  the normal preference writer. Runtime point data and authored source stay separate.
- Restore reads and expanded gzip designs are bounded to 32 MiB; decoded designs
  are capped at 100,000 TDoc nodes. CRC and source identity/change checks remain.
- `TabWidget.ReplaceTab` preserves order/selection and reports committed page
  identity, not an intermediate strip/stack state. The old page is detached,
  never destroyed or silently saved. Ancestors/foreign-owned pages are rejected.
- New captured-state watcher APIs avoid a second synchronous metadata read when
  registering a restored design. `fswatch.Close` revokes delivery without joining;
  `Wait(ctx)` distinguishes physical polling completion from cancellation.
  Late AddPath/tick snapshots cannot resurrect removed or closed registrations.

This does **not** make every manual Open, save, design-widget resource load or
reload callback nonblocking. No full UI FPS/latency improvement is claimed.

## Reproduced and corrected during development

- Existing synchronous session tests initially failed immediately after the API
  became asynchronous. They now drive the real completion dispatcher and retain
  the original source/order/active/caret/MRU assertions.
- A focused SDK replacement test failed when the old page's OnHide selected a
  different tab: strip and stack diverged. Replacement now honors the final
  lifecycle selection before notifying, and the regression passes.
- Empty/legacy sessions initially chose a blank Edit workspace. Empty launch now
  stays Welcome, and legacy restore uses the last document's workspace mode.

## Deterministic regression coverage

- Tests cover blocked reads with other sources completing, exact order/location,
  dirty manual buffers, close/cancel/timeout, bounded physical slots, Welcome,
  pending quit persistence via fresh preference readback, nested recovery-modal
  edits, compressed/oversized input, reentrant replacement and watcher retirement.
- Watcher/reloader race packages also pass 12 repeated runs. Focused corrected-
  source suites pass; the final frozen platform results are listed below rather
  than combining counts from different revisions.

## Reproduced SDK issue and correction

A standalone nested-module probe uses the real public Reloader: it enters an
actual reload callback, blocks it, calls Stop and observes whether Stop returned
before that callback could finish. The current result is:

```json
{"stop_returned_before_callback_finished":true}
```

The old `Reloader.Stop` joined only its event-reader loop. Debounce callbacks ran
through untracked `time.AfterFunc` goroutines, contradicting the serialized-
callback/shutdown contract. The exact unchanged public probe now reports:

```json
{"stop_returned_before_callback_finished":false}
```

The replacement has one collector/timer and one serialized worker, latest-token
coalescing, watch-intent/entry identity guards and configurable `MaxPending`
(default 1,024 distinct active/queued paths). Overflow is observable through
`Stats.Rejected` and a coalesced `ErrPendingLimit` error hook, not an unbounded
thread/queue. Close revokes future admissions, while an already admitted callback
may start/finish; Wait proves physical callback/poller completion. Callback Close
is safe; callback Stop/Wait would self-join and must not be used. IDE teardown
closes before joining off-loop and separately retires already-posted UI authority.

Focused race tests cover 32 concurrent Stop callers, blocked physical reads,
15,000 coalesced/rejected events, serialized/latest delivery, root replacement,
registration/unwatch races and callback/error-hook panic containment. They also
cover pending property drafts without implicit commit and TDoc prefix node limits.
After user editing/mode interaction, automatic recovery is deferred without
retaining old document payloads. File/palette Review Session Recovery is bound to
its exact controller and performs a fresh bounded read before asking consent.

## First frozen candidate rejected by native keyboard acceptance

The first 2,714-file candidate passed macOS and Linux full ordinary/race and
four strict real bundled-gopls probes each. Those results are retained, but are
**not** the final gates for the corrected source below. Windows restored two
sources in order with the saved Unicode location/scroll and correct view, yet
no-mouse keyboard typing and paste did not reach the editor. A save-only chord
formatted the original text; it did not prove input acceptance. No resulting
screenshot is promoted as the final gallery.

A focused regression reproduces the missing initial loading-page focus before
the fix. Loading pages now own initial tab focus in Edit mode, and replacement
transfers it only if the same page/workspace still owns focus. A different field
must keep its focus. Skipping a missing active source passes focus to the next
loading/ready page. Focused races pass for both transfer and non-stealing; all
final platform/native gates are refrozen and rerun, not inherited from candidate 1.

Native QA setup errors (a C# helper named Main with a non-entry signature and
PowerShell treating a benign helper stderr warning as terminating) were confined
to automation scripts. They are retained separately; the IDE source was not
patched to hide those warnings. The macOS candidate again reaches AppKit's event
wait with the correct isolated Go toolchain, but CUA capture still fails -3811.

## Historical startup-only corrected-source verification

The second frozen archive is SHA256
`8b1edc2ff3a730f255665005626dd760cd23a4e04a4115f812c0fdcfc4efa36c`:
2,714 files / 1,990 Go files. All 2,012 compiled/native/resource inputs match
both the index and working tree. Full ordinary/race and strict runners exit zero:

| Corrected source | Ordinary pass / skip | Race pass / skip | Strict bundled gopls |
| --- | ---: | ---: | ---: |
| macOS arm64 | 8,907 / 29 | 8,907 / 29 | 4 pass, 0 skip |
| Linux arm64 / Xvfb | 8,846 / 37 | 8,846 / 37 | 4 pass, 0 skip |
| Windows amd64 | failed partial run: Git cases and codegen timeout | not run | not run |

Counts are named test/subtest events, not assertions. Post-run all-file hashes
match on both completed platforms, with no extra Go. A corrected macOS-native
build with truly private process settings reaches the ordinary AppKit event wait;
CUA still fails ScreenCaptureKit -3811. This is not keyboard/IME or UI-exit approval.

The corrected Windows delta upload and subsequent 2,714-file checksum readback
completed successfully. SSH later recovered: explicit task/marker inspection
proved the earlier final-build submission had not registered a task or produced
an executable. Only then was the final build submitted; it passed after checking
every manifest hash. No OS/network setting was changed to bypass the outage.

The subsequent ordinary Windows run is **not a pass**. A captured live log has
failed `TestGitDiffUsesFileRepositoryNotProcessWorkingDirectory`,
`TestGitDiffContextStatesAndLiteralPaths` and the flood subcase of
`TestGitCommandBoundsOutputAndReapsCanceledChild`. The diff feedback reports
`context deadline exceeded`; the flood helper never reached its output limit
before its deadline. The ged package hits its 20-minute budget while
`TestCodeGenCodeEditorWithEventCompiles` is waiting for a child `go vet` process.
The same captured log already includes passing fswatch/hotreload packages and
6,777 named passes, but those cannot override any failure. Keep the original
task until its actual terminal marker; compare the failing flow against the
published baseline in the same environment before attributing or changing code.
Race, strict gopls, post-source and publication gates remain closed.

### Same-host isolation controls after the failed run

The original task reaches actual `full=1`, `runner=1` and Ready; it is not
restarted on a polling timeout. Two native GUI test binaries use the same source
tree/toolchain/DLL environment; the baseline overlays exactly the published GUI
sources and disables only the newly added replacement test. Four focused Git
runs in baseline/candidate/candidate/baseline order all exit zero in 5.2–7.1 s,
with the original command deadlines unchanged. The host reports four processors;
one standalone Git version query takes 440 ms.

Both entire GUI suites then pass separately from the GUI package directory:
baseline 48.5 s, candidate 49.7 s. An initial diagnostic used the repository root
as cwd and failed relative-source checks; that harness error is preserved and
not attributed to the SDK. The corrected controls use the normal package cwd.
This rules out a deterministic standalone GUI failure in these controls, but
does **not** prove the original whole-suite failure was harmless or its exact
cause. Next isolate the complete ged/code-generation flow before changing code
or retrying a final full run. No test is skipped or deadline lengthened to pass.

The complete unchanged ged package subsequently passes alone in 436.0 s. The
original failed log is archived independently and its source identity is audited
before a new full attempt. The next Windows profile uses `-p 1`: real native
tests share one interactive desktop and clipboard, and generated-program tests
launch nested Go commands on this four-processor host. All packages, internal
concurrent cases, race detection and the 20-minute package limit remain enabled.
This is a serialized native-test profile, not a claim to have established the
exact contention/cache cause or fixed all performance-under-load behavior.

After connectivity recovers, the retained serial startup-only task is observed
Ready with actual full/race/runner exit files all zero. Both suites contain
8,697 named passes / 42 skips and no failed events. These results supersede
neither the retained failed parallel run nor the new combined-source gates.
Its strict probe was not run after that source became obsolete. The combined
source is extracted separately; no old task/source tree is overwritten.

## Corrected Windows native acceptance

The interactive Session 1 replay uses the final executable and private settings,
source/project and data directories. The UI was not modified for automation.

- Two restored source tabs retain order, active source, Unicode and saved scroll/
  caret location. Without any mouse focus click, keyboard input appends the exact
  marker and makes the editor dirty; an independent disk hash stays unchanged.
- Explicit Ctrl+S saves the marker. A separately launched helper parses the saved
  Go source successfully. After normal Alt+F4, native GetExitCodeProcess returns
  zero and a fresh settings reader confirms tab order, active b.go, updated caret,
  retained scroll and Edit mode. A null PowerShell ExitCode is not used as proof.
- Ctrl+3 shows the restored design. An external file write changes its button
  from Restored to Reloaded through the real watcher/reloader. Both captures were
  inspected; the Go source hash is unchanged. Ctrl+2 returns to the saved editor.
- A separate 32 MiB + 1 byte process source remains an owned, read-only error
  page. F5 leaves its bytes unchanged and does not run the neighbouring, buildable
  Go sentinel project during the observation window. Ctrl+W closes that page and
  selects a.go; after normal exit zero, fresh settings contain only that source.
  The failed source is still exactly 33,554,433 bytes. After completing this
  negative check, explicitly running the built sentinel writes its expected
  marker, independently proving that the private fallback project was runnable.

The native stderr independently records bundled gopls v0.24.0 reaching ready,
loading this private module with Go 1.26.10, then shutting down normally. The
visible popup shows local suggestions; that alone does not verify all semantic
operations. The strict matching-bundle probes below are a separate gate. Blocked-reader cancel,
timeout and nested recovery-dialog races use deterministic dispatcher tests;
they are not claimed as timing-dependent native mouse tests. These observations
do not certify startup latency, all menus or the full product roadmap.

## Completed gates and remaining scope

- The public Stop counterexample is corrected; repeated ownership/retirement
  races, frozen full three-platform suites and 12 strict bundled probes pass.
- Windows native persistence/control and macOS basic restoration/reload/exit pass
  for the combined source, separately from deterministic blocked-reader and
  nested-recovery tests. README captures and publication build inputs are audited.
- Continue manual Open/Save/resource/reload-callback I/O, native IME and remaining
  source-runtime/menu coverage. Do not infer blanket latency/FPS improvements,
  a typed alarm engine or complete product readiness from this increment.

## Replay commands

Use the matching platform Go/C toolchain and a private `SILK_DATA_DIR`; graphical
tests require the platform's real desktop or Linux Xvfb. Run from an isolated
checkout with all Go sources indexed, never with generated harness Go at its root.

```sh
go test -race -count=12 ./fswatch ./hotreload
go test -race -count=1 ./gui ./cmd/silkide
go test -json -count=1 -p 2 -timeout=20m ./...
go test -race -json -count=1 -p 2 -timeout=20m ./...
STAGE=/tmp/silk-verified-tools
go run ./cmd/silktools -out "$STAGE/tools"
bash scripts/test-bundled-gopls.sh "$STAGE"
```

The strict integration runner is built beside its verified `tools` directory;
the repository's `tools/` directory contains pinned source, not the runtime
binary. Windows uses the same staged layout and serial full suites (`-p 1`),
without dropping packages, internal concurrency or the 20-minute package limit.
Set `SILK_TEST_REQUIRE_GOPLS=1` and `SILK_TEST_REQUIRE_BUNDLED_GOPLS=1`, then select
`TestGoplsUnsavedUnicodeCompletionAndFormatting`,
`TestEditorRenameRealGoplsKeepsUnsavedDefinition`,
`TestRealGoplsNavigationUnsavedTypeImplementationAndBack` and
`TestRealGoplsIncrementalUnicodeCRLFEdits`. Requiring the bundle must yield four
passes, not optional skips. Keep its test executable outside source enumeration
and remove it before the final extra-Go/source-identity check.

Ignored raw evidence and replay code:
`bin/qa-artifacts/startup-io-20261115/`.
