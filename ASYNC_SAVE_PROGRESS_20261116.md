# Async document operations — implementation and verification progress

Status: **validated unreleased development-source increment**, based on
`882815032e318883383e751770965b120d56d379`.
The continuing designer/editor/industrial SDK product goal remains active. This
is not a claim of Qt Creator parity, complete platform readiness or a new binary release.

## Current implementation

- Explicit toolbar/File/shortcut/palette/source-tab Save captures UI text or a
  design receipt plus independent typed code inputs. Formatting, fingerprints,
  primary encoding, recovery cleanup and companion file work run off UI.
- The shared physical queue retains one active/one replaceable latest task.
  Save All sequences distinct documents, never silently dropping an unrelated
  target. Reservations last through physical completion **and** UI publication
  or explicit retirement. Cancel cannot free a blocked OS operation.
- Actual tab close, New, Open, native close and shortcut quit now use captured
  asynchronous continuations, not the old synchronous bool guards. Native close
  returns false while admitted work proceeds, then posts one owned close.
- A close/quit arriving during an existing Save or Save All waits for both
  physical work and UI/batch completion, including gaps between batch targets.
  It recognizes only legitimate own formatting/saved-position publication;
  newer input, model notifications, command/Undo ABA and draft revisions still
  revoke teardown. Waiters are cancellation-owned notifications, not polling.
- Save/Discard decisions are revalidated after modal cleanup and again before
  teardown. A real quit modal can dispatch Save and Close for a listed source;
  only its actually saved/observed clean identity may be removed from that list.
  A pending nested Save must finish physically and publish before final quit.
- Save As inspects unknown targets on the bounded worker. Existing content
  requires explicit replacement consent, defaulting to Cancel. The writer
  compares the fingerprint actually reviewed, so later external changes are
  refused. Opener/scope retirement severs late inspection/chooser/save results.
- Source Save seals its typing Undo group at admission, never on late completion.
  Batch-close preserves CRLF and publishes actual saved bytes/baselines even
  when newer text remains dirty. Multi-file transaction preflight rejects every
  replacement if a changed target is read-only; unchanged no-ops remain allowed.
- Git pane keys use explicit project/lexical design parents and cached startup
  cwd, not UI-thread go.mod walks. Build/Run/test/vet/coverage reject pending
  save/document operations. They still use saved files; automatic Save-before-
  Build and wholly asynchronous project discovery are not claimed.
- Ready source snapshots cannot install across a concurrent save/mutation.
  Session restore shows an explicit terminal retry state instead of an inert
  loading page. Manual reopen reads fresh bytes and replaces that page.

## Intermediate evidence (not substituted for final gates)

Raw results: `bin/qa-artifacts/design-save-async-20261116/transitions/`.

- `broad4`: race `gui`, `ged`, `cmd/silkide`, 6,443 named passes / 16 skips,
  exit 0; it predates the already-pending-save/review/batch/Git additions.
- `nested-save`: 20 race rounds, 100 named passes, exit 0, covering Save and
  Discard after a separately admitted nested save and newer-input preservation.
- `ownership`: 20 race rounds, 1,140 named passes, exit 0, covering initial
  pending saves/batches, checkpoints, batch CRLF/read-only and notification waits.
- `review2`: 20 race rounds, 1,020 named passes, exit 0, covering explicit
  replacement consent, subsequent conflict/edit/Undo, worker retirement, owned
  close/open transitions and stale open/restore delivery.
- `proposal4` is the preceding exact frozen seven-package increment: ordinary
  and race each 7,433 named passes / 20 skips, repeat 3,520 passes, hashes pass.
  Its SHA is `b5a80ba2eeb6477611b24ace8572f48dee2dc778b6fd049b2416a8484930e45f`.
  It is historical after the current runtime changes, not current full QA.

Retained negatives/failures are not counted as passing gates: the test-only
`typing-negative` overlay fails against proposal4 because typing before/after
Save merges across the saved position. Earlier broad interaction runs exposed
legacy tests expecting immediate synchronous close/quit; fixtures now pump actual
owned completion while retaining disk/page/focus assertions. `initial` contained
an undefined fixture plus an Undo test with no command; both fixtures corrected.
`review` assumed inspection refused read-only files before asking, but inspection
is read-only and refusal correctly happens in the writer; its corrected test
still asserts unchanged bytes, dirty state and unchanged Save As identity.

## Combined frozen-source gate and native replay

`proposal5` freezes 2,801 files / 2,059 Go inputs, archive SHA-256
`9a0cc3b43564050101ef7dd593728bbcfa79a8a43176fc12a2db8861eedfb750`.
Full and race each pass **9,435 / 29 skips** on macOS arm64 and **9,374 / 37
skips** on Linux arm64/Xvfb; strict real bundled-gopls gives four passes and no
skips on each. Mac focused repeat: 1,420 named passes. Pre/post checksums and
independent Linux source-export identity pass. Windows P5 has now terminated with full/race/strict/native-build/runner exits 0;
its raw named counts/source export still need collection, and it remains the
predecessor rather than current P6 acceptance.

Mac P5 native replay preserves read-only design/source originals, retains dirty
state after Save and Save-on-close/quit refusals, succeeds after permission repair,
reopens exact Unicode source and verifies typing `ab` -> Save -> `cd` -> Undo
returns clean `ab`, Redo returns dirty `abcd`. Saving the final Unicode buffer
then immediately quitting exits 0; actual disk/model/session readback and no
owned gopls remain. Both owned Mac PIDs are closed normally. The first test seed
used a Linux-style path and was not consumed; its empty-workspace quit and CUA
relaunch are recorded separately, not counted as startup-restore acceptance.

Native replay also exposed the title's 500 ms polling lag after design Save/Open.
A test-only overlay on exact P5 fails both immediate-completion title assertions.
Current source updates titles on Save/Open/New/restore completion; also reject
nested noncomparable draft token values without panic. These seven Go-file deltas
supersede P5 for final acceptance. Focused title/token race replays pass; exact
counts are in `transitions/current-summary.json`.

**Verified frozen source `proposal6`**: 2,803 files / 2,060 Go inputs,
archive SHA-256
`06b38e10109bae91654b960eae2cb28bd7b6acb6f33c625472d262115567975a`.

| Platform | Full named pass / skip | Race named pass / skip | Strict bundled-gopls pass / skip |
| --- | ---: | ---: | ---: |
| macos | 9,438 / 29 | 9,438 / 29 | 4 / 0 |
| linux | 9,377 / 37 | 9,377 / 37 | 4 / 0 |
| windows | 9,226 / 43 | 9,226 / 43 | 4 / 0 |

Every suite exits 0 with no failures. The 20-round Mac focused race run adds
1,480 passes and no skips. Independent Linux/Windows post-source exports match
every manifest entry with no extra Go inputs; Mac pre/post hashes match too.
Windows uses a single source job, never restarted after an observer timeout.
`three-platform-verified.json` retains independently parsed raw counts/exits.

The first P6 setup raced Docker bind-directory creation. Its Linux container
exit 2 and local Windows waiter import failure are retained; neither ran tests.
Separately named R2 harnesses completed setup before submission. This is harness
failure evidence, not a silently discarded runtime test.

P6 Mac native uses the identical payload SHA-256
`48710f9db6ef7405cb5439ee1be37b551dd2da23b58b27eaf83040cd2fa0f34c`.
It verifies actual restored state, immediate Save/New titles, read-only refusal,
Save As default-Cancel, preservation of an external edit made after overwrite
review, explicit retry adoption, source Unicode/refused quit Save All, durable
retry before exit, disk/model/session and normal exit 0 without owned gopls.
The first Desktop launch blocked in OS getcwd/open before a UI/log appeared;
its exact owned PID was terminated with SIGTERM and is **not a pass**. An
identical-payload temporary-directory run passed, and a later unchanged Desktop
run also restored/quit normally. Root cause remains unproved; do not relabel this
transient observation as a fixed framework bug or guaranteed startup performance.
Raw sample, aborted lifecycle, successful retry and temporary-profile acceptance
are retained under `proposal6/mac-native` and `mac-temp-evidence`.
Windows native verifies exact drag/save readback (30,30), read-only design
refusal, successful retry and retained Undo/Redo. Source refusal preserves exact
original bytes for both read-only attributes and a reader that permits direct
writes but denies replacement. Retry saves exact Unicode; Undo becomes dirty and
Redo clean without disk writes. A later native Alt+F4 / Save All refuses a dirty
read-only source and keeps the process/input alive; Undo restores the prior
saved text, then normal close exits 0. Independent final source/model/session
and zero owned gopls children are checked after physical exit. Screenshots are
unedited captures from this exact source-built payload.

## Additional exact-P6 checks

- Release-version validation and platform/branding scripts: exit 0.
- Pure-Go Cairo/paint parity: exit 0 with `CGO_ENABLED=0`, `silk_pure_go`.
- Opt-in native macOS GL transfer test: pass, not skipped; a 160 x 120 frame
  uploads 76,800 bytes initially and 2,688 bytes for the tested partial update.
  This is that transfer regression, not a whole-editor FPS/latency benchmark.
- Exact CI vet command: exit 0. Broader unfiltered `go vet ./...` remains exit 1
  with 501 Go diagnostics, identical to baseline by path/message (line shifts
  normalized), no new/resolved diagnostics. Disabling only `composites` for the
  whole repository still exposes four pre-existing test lock-copy warnings in
  `i18n`; that broader command is not a pass. Do not suppress them to manufacture
  a clean all-repository result. Apple file-dialog deprecation/compiler output
  is separately retained, not counted as Go diagnostic delta.
- README macOS/Windows source-refusal and macOS replacement-review images are
  exact unedited P6 native captures. The SVG is a labeled architecture diagram.

## Remaining publication/product requirements

The applicable frozen full/race/strict and native gates above are complete.
Exact staged runtime, independently exported platform sources and screenshot
bytes must match before publication; only specifically reviewed result docs/media
may differ from the frozen archive. Audit artifacts remain under `proposal6/`.
Publication is the user-authorized commit/push of this development increment,
not a new standalone package or a completion claim for the whole product.

Legacy standalone designer Save As and compatibility bool helpers remain
synchronous; legacy batch-close is a separately reserved transactional worker,
not migrated onto the shared save queue. Source workspace replacements and
Build/Run project discovery have their own I/O contracts. Further product
iterations must address these paths and broader editor/designer/SDK completeness,
not reinterpret this increment as completion of the whole product.
Native prompt punctuation/wrapping, long-path error toasts and Windows DPI
scaling still need UI polish; the acceptance screenshots do not conceal them.


---

# Previous snapshot-only increment (archived evidence)

Baseline: `882815032e318883383e751770965b120d56d379`. The production Save,
Save All, source saves, recovery and destructive-action guards are still
synchronous. This work is uncommitted; no new package/release or push is claimed.
The broader Qt Creator-style designer/editor/industrial SDK goal remains active.

## Implemented boundary

- UI-only `PrepareSave` commits drafts and retains an owned receipt; only its
  independent `DesignWriteSnapshot` goes to a worker. Completion separately
  validates path, intent, design revision, pending drafts and Undo epoch identity.
- `CodeSnapshot` copies properties/layout/tag declarations/bindings/source on UI,
  strips live widget/scene references and defers handler AST work. Rendering can
  run concurrently; physical split-file writes must be serialized. Existing
  machine ownership, user handler bodies/format/import rules remain in force.
- `PreparedSaveCurrent` checks the primary receipt after companion capture.
  `DocumentCopy` isolates companion TDoc mutation/indexing from primary encoding.
- The owned internal saver pairs primary and typed companion capture before
  admission. One physical slot includes primary write **and** companion work.
  Companion errors/panics are warnings, not a false failure of an already-saved
  primary. Retirement does not mean an active physical write did not commit.
- `WriteQueue` bounds side effects to one active and one replaceable queued
  task; neither timeout/cancel nor UI close replaces blocked OS work. It has no
  idle worker/polling. Close/Wait and physical results remain distinct.
- SDK history/saved-text publication supports newer editor input without
  destroying Undo/caret. An initial scene dirty/title nil-stack bug was fixed.

These primitives are **not yet wired to production commands**. In particular,
returning `admitted=true` is not permission to proceed with quit/open/New.

## Reproducible local evidence

Frozen source: `bin/qa-artifacts/design-save-async-20261116/proposal2/`

```text
Files:        2777 (2037 Go)
Archive SHA:  3e2b9ec779e4aa20ff2a321ebb8115d379d1ee2d56c1f9d236cfb63b493ed115
Platform:     macOS arm64
Packages:     ./async ./core ./graph ./gui ./ged ./cmd/silkide
ordinary:     7380 named pass, 20 skip, exit 0
race:         7380 named pass, 20 skip, exit 0
20 repeats:   2960 named pass, no skips/failures, exit 0
source:       pre/post checksums pass; live runtime input mismatch = []
```

These are affected-package checks, **not** full-repository, all-platform, strict
gopls, native UI or standalone-release acceptance. The documentation/diagram was
updated after freezing; runtime inputs match. Exact JSON counts, exits, manifests
and readback are retained in `verified-local-progress.json` and `run-local.sh`.

Additional evidence under `bin/qa-artifacts/design-save-async-20261116/codegen/`:

- Compare the preceding frozen generator against the current generator using
  the same test-only corpus overlay: 288 cases, 288 byte-identical records,
  including machine/user source, diagnostics and indexed constructor lines.
  Both output SHA-256 values are
  `e38260d474710ba2bf4b147474052b50780d3625af434f27187429733a383507`.
- Worker tests preserve captured primary/code despite corrupting the primary
  disk file and changing the live widgets after capture. Recursive value checks
  reject retained live references; concurrent rendering/live-UI edits pass race.
- Primary failures never run companions; companion warnings/panics do not erase
  primary success. A blocked companion retains its physical slot across logical
  retirement, with exactly one final current UI continuation.
- The first companion-slot test failed because its fixture incorrectly assumed
  an empty filename; `testWorkbench` assigns `Form.silkui`. Keep
  `companion-slot.log`/exit 1 and the corrected original-path comparison in
  `companion-slot2.jsonl`/20-pass exit 0. Do not present the harness failure as
  a demonstrated runtime bug.
- A test-only capture benchmark (20 buttons, three samples, no other QA runner)
  reports 11.2–12.7 microseconds for 27-byte handlers and 14.0–14.4 microseconds
  for 1,048,604-byte handlers; both use 31,944 B/op and 117 allocs/op. This only
  measures the UI capture stage, not document capture, disk, complete Save or
  Windows performance. Retain the earlier concurrently loaded run separately.
- The previous frozen production writer probe still fails as expected:
  `proposal1/production-negative/`. Do not substitute the owned API's early return
  for a corrected public Cmd+S path.

README now contains a labeled SVG architecture illustration, rasterized and
visually checked locally. It is **not** a fake native screenshot. The earlier
real macOS/Windows images retain their original synchronous-safety scope.

## Remaining integration / publication gate

1. Independent disk baselines and source formatting/save publication; reserve
   primary, companion and recovery targets against rename/search/batch close.
2. Route manual Save/Save All/source menus/Save As and recovery through coherent
   physical ordering; avoid a hidden synchronous companion/recovery cleanup.
3. Migrate owned close/open/New/native and shortcut quit continuations. Only
   current durable results may continue them; newer edits must remain dirty.
4. Replay the actual production public writer probe positively; freeze final
   runtime, run full/race/strict gopls on macOS/Linux/Windows and actual native
   readback/normal exit checks. Refresh real screenshots as applicable.
5. Audit exact staged/tested inputs and docs, then perform the user-authorized
   commit/push. Do not publish incomplete integration as a completed fix.
