# Owned design opening and bounded canvas — verification

Baseline: `56fe3842caebc856c81d024ecd25214fdd686af3`.
This increment improves the designer and SDK; it is **not** complete Qt Creator
parity, a binary release, or a universal explanation/fix for Windows CPU spikes.

## Changes

- Manual `.silkui` primary/recovery stat/read, gzip expansion and TDoc parsing now
  run off the UI thread. Each workbench admits one physical reader and one
  replaceable latest queued target, with a 30-second cooperative deadline.
  Cancel/timeout do not free a blocked OS reader's slot or start replacement
  threads. Source-opening and startup pools are separate.
- Keep the current design editable and retain its original session/MRU identity
  until an owned result actually installs. Reject delivery after changes to the
  owner, view, scene/path, Undo metadata, pending drafts, focus/capture, mode,
  rename, New, close or newer intent. Normal model mutations must still emit
  their existing design-change notifications.
- Read limits: 32 MiB compressed/expanded and 100,000 TDoc nodes; validate the
  form root. Missing/damaged originals may explicitly recover a valid autosave.
  Rejection/read errors cannot install an empty design or write the input file.
- File/palette **Cancel Design Open** targets the observed controller. Rebind the
  watcher/autosaver only after actual installation, reusing captured file state
  rather than issuing a second UI-thread stat.
- SDK `GedView.IsClosed` / `SubscribeClosed` are UI-only, idempotent and sever
  canceled callback ownership. Closed views require fresh host recreation.
- SDK `gui.PostAfterNativeCleanup` schedules modal continuations after native
  destruction/focus-return attempts, then outside the cleanup batch. No timer,
  polling thread, worker join, input lock or forced focus is added.
- A recovery continuation accepts temporarily **empty** focus only after its
  own modal and only if the captured input owner was its canvas. A real foreign
  field, ordinary focus loss, pending edits or newer intent still reject it.
- Bound line grids/rulers to 1,024 entries per axis and dots to 64×64, using
  visible spans and origin-anchored paint-only decade LOD. Preserve document
  geometry, snapping, color/width and exact ordinary Cairo RGBA output.
- Clip huge page/background/overlay/line paths before Cairo conversion. Check
  transformed **device corners**, not just authored dimensions. Guard nonfinite
  and non-advancing arithmetic; NaN zoom falls back to 100%.

## Counterexamples and retained failures

Evidence root: `bin/qa-artifacts/design-open-20261116/` (ignored QA artifacts,
not a dependency or source package).

1. `baseline/`: a controlled regular-file reader proves public `openDocument`
   waits inside the baseline UI call. The corrected public flow returns before
   that reader is released and later installs exact Unicode contents via the
   owned UI dispatcher. This is not a slow-drive timing benchmark.
2. `grid-baseline/`, `ruler-baseline/`, `dot-baseline/`: real finite Cairo targets
   stop after 1,025 operations instead of attempting an enormous loop/allocation.
   Corrections bound actual path/tick work, not merely background parsing.
3. `device-baseline/`: finite 1e5 mm at 100× and 1e6 mm at 10×/100× exposed backend
   overflow that a logical-size threshold missed. All 15 public-view comparison
   cases now pass without relaxing ordinary pixel assertions.
4. `verified3/mac-native/` and `native-focus-probe/`: stock native recovery No
   left the origin scene. A separate read-only logging overlay found valid
   owner/scene/intent, no draft/capture, saved canvas focus, **current focus nil**.
   This disproved the earlier assumption that cleanup always restores identical
   focus. The narrow canvas-only empty-focus correction was then replayed in a
   separate stock app and again from the final frozen source.
5. `verified2/linux-initial-race-failure/`: the unchanged editor allocation test
   once observed 48 allocations against a zero requirement, without a data-race
   report. Its cause is **not proved**. Fifty isolated race repetitions, three
   entire GUI race runs and subsequent full frozen-source Linux runs pass the
   unchanged assertion. Do not relabel the original failure as a pass.

Earlier candidates and superseded Windows runner logs remain separate. An
accidental old Linux-container restart is recorded under `verified2/`; it is
not the final candidate. No old task/container was used for the final gates.

## Final frozen-source package gates

Candidate: `verified4/`, **2,748 files / 2,015 Go files**.
Archive SHA-256:
`6933dac3fd265870a1fa71cfc0b1ad8fbcc7c53459e937db3b2c4f0b2a6016b5`.

| Platform | Full ordinary pass / skip | Full race pass / skip | Strict bundled gopls |
| --- | ---: | ---: | ---: |
| macOS arm64 | 9,136 / 29 | 9,136 / 29 | 4 pass / 0 skip |
| Linux arm64 / Xvfb | 9,075 / 37 | 9,075 / 37 | 4 pass / 0 skip |
| Windows 10 amd64 | 8,925 / 42 | 8,925 / 42 | 4 pass / 0 skip |

Named-test counts include subtests. Full runs use:

```sh
go test -json -count=1 -p 2 -timeout=20m ./...
go test -race -json -count=1 -p 2 -timeout=20m ./...
```

Raw JSON, stderr and each own exit file are retained. Platform/environment skips remain visible, not counted as
coverage. The strict phase requires bundled `gopls` and permits no skips:

- `TestEditorRenameRealGoplsKeepsUnsavedDefinition`
- `TestGoplsUnsavedUnicodeCompletionAndFormatting`
- `TestRealGoplsIncrementalUnicodeCRLFEdits`
- `TestRealGoplsNavigationUnsavedTypeImplementationAndBack`

All final runner/ordinary/race/strict exits are zero. Pre/post manifests and
independent Linux/Windows readback match all 2,748 files with no extra Go source.
Generated fixture modules stay outside SDK source. Final indexed build/runtime
inputs are compared with this tested snapshot; only reviewed documentation and
actual screenshot updates follow freezing. The unchanged, noncompiled legacy
`core/callback.go.txt` CRLF/LF Git normalization is recorded separately.

## Final native acceptance

macOS: `verified4/mac-native/SilkDesignVerified.app`, fresh private HOME/data,
no instrumentation; bundled gopls resolves next to the executable with no
`SILKIDE_GOPLS` override and reaches connected state.

- Two No choices, **each starting from origin**, actually show `Original saved`;
  same-path repeats are not counted as proof. Yes shows `Recovered data` with
  dirty state, then Save and independent model readback confirm recovered bytes.
- Tree-open target, drag `submit` from (20,25) to (30,30) mm, Save; independent
  TDoc/model readback verifies exact coordinates, title and text.
- External target rewrite visibly reloads `Reload ready`, confirming the watcher
  follows the installed file rather than the previous design.
- Open a 1e12×1e12 mm form: visible grid and child remain. Opening invalid-root
  input retains that scene. Native Cmd+O opens the isolated origin fixture too.
- Origin/huge/invalid/source/go.mod hashes remain unchanged. Normal Cmd+Q exit
  independently reports **0**; final settings readback points at huge.silkui.

Windows 10 amd64: fresh private APPDATA in interactive Session 1, same frozen
source, no instrumentation, bundled gopls connected. Two No choices each from
origin, Yes->dirty recovery->Save, tree-open target, drag/save (30,30) mm,
external `Reload ready`, huge canvas and invalid-file scene preservation all
pass actual screenshot review plus independent saved-model readback. Normal
Alt+F4 exit is **0**, obtained with a process handle held before close and
`GetExitCodeProcess`; final session points at huge.silkui and original inputs
remain unchanged. Scoped process readback finds no remaining bundled gopls
child after normal close on either macOS or Windows.

A 10.084-second no-input sample on the huge canvas records 10 responsive samples,
0.1875 UI-process CPU seconds: **1.86% of one core**, **0.46% normalized across
four logical processors**. Working set changes from 86,786,048 to 86,790,144
bytes. This includes ordinary notification lifetime activity; it is one short
sample, not a steady-state FPS measurement, full process-tree measurement or
universal performance guarantee.

Two native harness failures are retained separately: a metadata PInvoke omitted
Unicode marshalling and read only the first caption character; a later
foreground-only modal lookup encountered the task console. Both failed before
clicking the recovery choice. Correct the harness with scoped Unicode caption
reads, exact-process visible-modal lookup, explicit focus and click-point owner
checks, then use new task/output names. No production changes or full-suite
restarts were hidden behind these corrections. Raw evidence and correction
notes are in `verified4/windows-native-evidence/` and the sibling JSON/logs.
The existing optional `property/ged.cml` default fallback warning is preserved,
not disguised by inventing an untracked resource.

README uses unmodified final-source macOS and Windows desktop captures:
`docs/images/design-open-drag-macos.png`,
`docs/images/design-open-large-canvas-macos.png`, and
`docs/images/design-open-drag-windows.png`. These are actual native
screenshots, not mockups, measured frame rates or whole-product benchmarks.

## Remaining product work / scope boundary

Widget/resource construction and design installation still run on the UI
thread. Explicit Save, resource loading, macro output and subsequent reload/
autosave callback I/O need their own bounded/asynchronous work. Safe closed-dock
recreation, broad native IME/DPI/menu coverage and arbitrary complex control
trees are separate increments. Unit ownership/race tests do not imply every
native interaction combination has been manually exercised.

Final source/index and whitespace checks pass; 2,740 build/runtime files match
this tested snapshot, with only reviewed documentation/captures and the recorded
unchanged legacy normalization outside that exact-byte check. The authorized
commit/push still requires remote identity verification. The wider product goal remains active;
there is no claim that all Qt Creator or industrial HMI work is finished.
