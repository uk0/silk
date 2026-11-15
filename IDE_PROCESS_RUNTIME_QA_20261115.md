# Source-backed process runtime inside SilkIDE — 2026-11-15

Baseline: `087c8a2d8b1c747828d8c81021b5950033d49685`.
This is an embedded frozen-source runtime, not hot reload, an alarm engine, a
PLC write protocol, database migration UI or full Qt Creator feature parity.

## Ownership and UI contracts

- `hmiapp.RuntimeView` is an ordinary widget. Construction / SetSnapshot do not
  start acquisition or open a database. Explicit Start owns a background store
  and runtime; standalone Workbench uses the same worker lifecycle helper.
- Diagram and point modes use alternate full-width views in compact docks.
  Only the visible subview stays bound; switching does not restart acquisition.
- New source snapshots fit their own geometry, not the initial template camera.
- UI-thread Stop revokes borrowed Runtime access and point dialogs immediately;
  cancellation is not a rollback guarantee. Close cancels; Wait runs outside UI.
- Opt-in `IEventTreeVisibility` reports ancestor visibility/reparent events.
  Ordinary local OnShow/OnHide semantics remain unchanged. Logical hiding stops
  the new runtime view; showing it never starts I/O. No new idle timer or poll.

## IDE source authority

- Tools / palette / F5 read the exact tracked `.silkhmi` editor buffer, including
  unsaved JSON. Source identity, path and revision guard asynchronous decoding.
- A one-job decode executor and cancellation/generation guards retire old work.
  Source changes, close, rename preparation, tool hide and shutdown stop pending
  startup / active work. Closed editors are not retained as navigable owners.
- Point data uses a per-source canonical-path namespace under application data,
  not a source-side database. Operator writes and history never save source text.
- Dock-owned tools recover after hide/removal without creating independent native
  windows or silently starting acquisition. Go to source targets the same editor.
- F5 on an unavailable process document must not execute an unrelated Go project.
  Ordinary Go run/debug paths remain separate; generic Stop includes this runtime.

## Targeted evidence

SDK / IDE / GUI visibility race regressions pass for snapshot isolation, typed
HTTP-to-points, manual persistence, restart, late ready delivery, dispatch failure,
reentrant Stop, parent hiding, menu/palette/F5 source routing, stale decode,
close/rename retirement and retained tool recovery. Broad core/point/HMI/GUI/ged/
IDE races pass on the initial native candidate; the final source was independently
frozen and rerun through the complete gates below after the camera correction.

The final pure-Go rendering-tag focused race run also passes 13 named cases
across `hmiapp`, `cmd/silkide` and `gui`. This is additional coverage, not a
replacement for the complete native-backend suites below.

Real Windows Session 1 replay uses the IDE executable, native keyboard/mouse,
its ordinary file dialog, a separate loopback HTTP fixture and isolated data:

1. Open a disk project with BI11 initial=41. No acquisition/store exists.
2. Paste a dirty unsaved project with BI11 initial=42. F5 opens the embedded dock;
   the table shows 42 while the source file hash remains unchanged.
3. POST the exact configured JSON body, extract a nested field, scale to 82.6
   and verify manual bool/int writes, HTTP-owned read-only action and history.
4. Switch Diagram/Points without stopping; observe controlled HTTP 503 and
   recovery. Recovery capture must wait for successful mode-2 responses because
   the existing poller deliberately backs off after errors.
5. Edit source: immediate cancellation / no continued requests. Undo does not
   restart; explicit F5 restores persisted DI11=true and BI11=43.
6. Collapse/reveal the dock: stopped counters remain equal; reveal is inert.
7. Restart explicitly, Cancel a dirty-close prompt (runtime continues), then
   Discard/close source (runtime stops). The original disk file remains unchanged.
8. Close the IDE normally and reopen its SQLite database independently read-only:
   integrity=ok, exact typed manual history entries and recovered HTTP values.

The first actual diagram capture exposed an undersized template camera. A focused
regression fails before the SetSnapshot FitToView fix. That executable/evidence
is retained, not promoted as the corrected candidate's final UI acceptance.

The corrected executable was replayed from a fresh isolated Session 1 baseline.
All native phase markers and the normal IDE/fixture exits are zero. Recovery was
captured only after at least two successful recovery responses; the diagram
visibly shows **77.5 bar in green**. Independent read-only SQLite reopen reports
`integrity_check=ok`, three point definitions, latest DI11=`true`, BI11=`43` and
AI11=`77.5`/Good. There are exactly two manual history writes: DI11=true and
BI11=43. The disk source still contains initial BI11=41, not dirty-buffer 42.

README point/diagram PNGs are byte-identical to the corrected final session,
not mockups or captures of the first, undersized-camera candidate.

## Native macOS observation boundary

The current full macOS IDE was built successfully, but CUA app selection timed
out before a usable window appeared. A separate observation-only Go overlay
captured the main goroutine blocked in `os.ReadFile` from `loadFileInEditor`,
called by workspace-session restoration before the event loop. The referenced
file was regular, 3,772 bytes; a FIFO hypothesis was disproved. The focused probe
recorded its mode, size and path length, not its name/content. No production patch
or OS-permission change was made on that hypothesis, and personal preferences
were not modified.

`SILK_DATA_DIR` does not isolate Darwin `settings.Default`, which derives its
path from the user home. A second temporary bundle requested a private HOME and
data directory; CUA still timed out and a native sample again showed `open` on
the main thread. That request did not prove preference isolation or its exact
OS/file-provider cause. Only owned QA processes were terminated. Automated
macOS suite success is **not** claimed as native keyboard/IME or startup approval.
Startup file I/O and actual macOS-native acceptance remain follow-up work.

## Tested source and executable

- Baseline: `087c8a2d8b1c747828d8c81021b5950033d49685`.
- Frozen archive: 2,703 files / 1,982 Go files; SHA256
  `28fb6fc74c93e13710162e0bc6b43798ee47e58b4b75105ce85b9fb3d8d90a39`.
- Final Windows native executable SHA256:
  `80ae7bfea4f04a55664d3d5d48131f2ec09c95dca0d7f2aa6baf225f387870d0`.
- Independent unchanged source-file SHA256:
  `3c8bdefb2564ee84de35c99dfdc562e8cd1932ebcc86f166068e04145b2c5c57`.
- Native replay and final suites use the same 2,004 compiled/native/resource
  inputs. Only reports/documentation and two final gallery PNGs are updated
  after the freeze; those are not compiled inputs.

## Final full-platform verification

Each ordinary/race cell counts named Go test and subtest events, not assertions.
Every suite and runner exits zero; there are no failed test/package events.

| Platform | Ordinary pass / skip | Race pass / skip | Strict real bundled gopls |
| --- | ---: | ---: | ---: |
| macOS arm64 | 8,853 / 29 | 8,853 / 29 | 4 pass, 0 skip |
| Linux arm64 / Xvfb | 8,792 / 37 | 8,792 / 37 | 4 pass, 0 skip |
| Windows amd64 | 8,643 / 42 | 8,643 / 42 | 4 pass, 0 skip |

Commands: `go test -json -count=1 -p 2 -timeout=20m ./...`, repeated with
`-race`. Strict probes use the real matching-platform bundled executable,
require-bundled/require-gopls mode and verify unsaved Unicode formatting/completion,
rename definition, navigation/implementation/back and incremental UTF-16/CRLF.
Ordinary-suite optional/platform/display/permission skips remain skips; four
strict probes do not imply every optional native GPU, debugger or LSP test ran.

All 2,703 manifest files match after testing on each platform; no extra Go files
remain. Windows post-run hashes, phase markers, source/SQLite readback and the
gallery were pulled back and checked independently. Publication contains 2,705
tracked files: the tested set plus two verified PNGs. All 2,004 build/resource
inputs match exactly in both the index and working tree. Post-freeze report/docs
edits are whitelisted; the pre-existing noncompiled `core/callback.go.txt` differs
only by Git LF versus worktree CRLF normalization. Neither is a build exception.

This validates this increment, not full Qt Creator parity or industrial safety
certification. Native macOS startup/keyboard/IME, alarms, PLC commands, store/type
migration and the broader product roadmap remain explicitly unfinished.

Raw artifacts/replay scripts: `bin/qa-artifacts/ide-process-runtime-20261115/`
(ignored; generated/harness Go stays outside framework source packages).
