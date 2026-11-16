# Event-driven layout invalidation — verified development increment

Baseline: `055a84d66191d1944ba9fe31f1abf78401d63aa3`.
This is verified development source, not a new packaged release,
full Qt Creator parity or a global performance claim.
The full SilkIDE/designer/editor/industrial SDK objective remains active.

## Plan and implemented behavior

1. Preserve actual setter -> nested layout -> stale sibling/paint failures,
   rather than hiding them behind a manual `Layout()` after every mutation.
2. Add UI-only `Widget.RequestLayout()` and `InvalidateSizeHints()` without
   extending the required `IWidget` interface. Changed layout ancestors form
   an owner-local, revocable batch on the existing UI queue; no idle timer,
   background widget access or full-window widget registry is added.
3. Coalesce repeated edits into one pending batch per hierarchy. Only changed
   layout ancestors get entries; natural-size measurement remains separate.
   A pass yields after at most 64 entries or its soft 4 ms deadline. A custom
   `Layout` callback itself remains non-preemptible.
4. Preserve synchronous normal-path `InvalidateLayout` / `InvalidateParentLayout`.
   Higher ancestors update on a UI turn; reentrant invalidation defers rather
   than recursively entering the same widget. Native form/popup owner links
   are not treated as ordinary parent-layout dependencies.
5. Share dispatch with Resize so already-installed descendant geometry does
   not get a duplicate Layout just because its queue entry still exists.
   Changes made during a callback retain a separate subsequent admission.
6. Revoke pending/partial entries on hide/detach, releasing captured trees.
   Retain dirty flags for show/new-owner geometry. Zero-size construction and
   hidden mutation create no active queued layout work.
7. Connect Label text/font/wrap, shared layout setters and hierarchy/visibility
   changes. Keep fixed tracks, disabled/free-positioned layout and explicit
   bounds semantics. Do not auto-size a native window to preferred dimensions.
8. Reject stale box/flow/form snapshot ownership after earlier Resize callbacks;
   bound grid coordinates against the tracks actually computed for this pass.
   Moving/replacing FormLayout slots removes duplicate metadata/owned orphans;
   same-parent grid/form metadata changes request a new pass.
9. Add an actual SDK consumer, `examples/layoutupdates`, with text/font/spacing/
   visibility actions and no polling or action-owned manual Layout work.
10. Freeze inputs, run broad/race/compatibility/performance/platform gates, replay
    actual SDK and IDE UI, refresh original README captures, then audit exact
    staged/runtime identity before the authorized commit/push.

## Retained failures and current local results

Raw evidence: `bin/qa-artifacts/layout-invalidation-20261116/`.

- `negative`: on the published baseline, a changed wrapped label needs 176.4px
  but its nested container still gets 19.6px. Increasing internal spacing needs
  69.2px, while the outer sibling still begins at 39.2px. Both fail after real
  UI-queue drains; no manual Layout is supplied to conceal the bug.
- `foreign-negative`: VBox/HBox/FlexWrap snapshots overwrite a later child after
  an earlier Resize callback moved it to another owner and explicitly set its
  new bounds to (7,8,9,10). All three failures are retained.
- `metadata-negative`: FormLayout's saved `fieldActive` overwrites a field moved
  by its label's Resize callback. GridLayout panics at `colX[c.col]` when an earlier
  callback changes a later cell to coordinates outside this pass's old tracks.
- `form-negative`: re-registering one field duplicates it across rows, and
  replacing a slot leaves the old child independently painted by the form.
- `local3`: 63 named focused tests pass, no skips, exit 0; the example package
  compiles. Covers dynamic text/spacing/font/wrap/visibility, custom SDK hint
  notification, 1,000-edit coalescing, idle silence, reparent/cancel, nested pumps,
  synchronous compatibility, bounded turns/partial cancellation and the above
  ownership/metadata regressions. This is NOT full/platform/native acceptance.
- Earlier intermediate runs are preserved separately. The first prototype fixes
  the two baseline dynamic-layout failures; later failure replays exercise the
  still-unfixed snapshot/registration paths before those fixes.
- The first frozen proposal also passed macOS full tests, but an added nested
  event-pump replay exposed a flaw in that NEW prototype: a blocked successor
  self-posts, causing nine wakeups over eight idle nested drains. Preserve
  `blocked-negative` (exit 1). Do not call it a preexisting production CPU cause.
  The current source holds blocked work without polling and explicitly wakes
  the successor when the active pass completes. Its 20-repeat focused race
  regression passes; the new source requires its own frozen broad/native gates.
- `local5`: current no-polling successor dispatch passes 440 named race tests
  over 20 repetitions, no skips, exit 0. Includes a blocked successor arriving
  during a yielded partial pass and independent popup-owner isolation.
  Proposal 1 is superseded, even though its full/race/strict source gates pass;
  the newly demonstrated nested-pump invariant was absent from those suites.

## Frozen-source verification

Current frozen proposal 2: **2,819 files /2,068 Go files**, archive SHA-256
`54a3982574f45614fec88b469e1df17d5b4631d6865bd44924964e6a2072190c`.
All source gates completed with physical runner exits 0; independent post-source
exports match all files and contain no extra Go source.

| Platform | Full, named pass / skip | Race, named pass / skip | Strict actual bundled gopls |
| --- | --- | --- | --- |
| macOS arm64 | 9,479 /29 | 9,479 /29 | 4 pass /0 skip |
| Linux arm64 | 9,418 /37 | 9,418 /37 | 4 pass /0 skip |
| Windows amd64 | 9,267 /43 | 9,267 /43 | 4 pass /0 skip |

Additional gates: 440 focused race passes /20 repetitions, pure-Go renderer
234 named passes /one skip, opt-in native GL one pass /no skips, release-version
and platform-script checks. CI-scoped vet passes. Unfiltered `go vet ./...`
still exits 1 with **501 unchanged baseline diagnostics**, not a clean result.
Every published baseline Go/module file was matched to the previous frozen
vetted source before reusing its log; diagnostic path/message multiplicities
match after excluding only shifted line/column positions. No new diagnostics.

Raw final gates: `proposal2/three-platform-verified.json`; source/exit/log,
native, generated-consumer and benchmark evidence remain separately retained.

## Actual native SDK and IDE acceptance

The actual P2 macOS SDK consumer has passed both directions of every text/font/
visibility/spacing action, native width reduction (640 -> about 502 logical
pixels), button-flow wrapping and normal system Quit/exit 0. Captures are raw
window PNGs. One quiet 10.01-second interval reports 0.08 CPU seconds, about
0.80% of one core at `ps`'s 0.01s resolution. Preserve the stale-AX close-attempt error:
no close input was delivered, the fresh AX lacked that button, and actual Cmd+Q
closed normally. The app is closed; never re-query it in a way that relaunches it.

The actual Windows SDK consumer passes the same actions in both directions,
including keyboard activation, and native resizing from 680 to 500 window
pixels. Four buttons remain on the first row and the fifth wraps below.
A quiet 10.01-second sample records 0.03125 CPU seconds, about 0.31% of one core.
Normal Alt+F4 closes with held-process-handle exit 0. Three README images are
byte-identical copies of these macOS/Windows captures, not edited mockups.

Retain the Windows harness's misleading initial detail captures: its generic
`Reset-IME` helper injects Ctrl+Space into the previously focused Toggle font
button, changing geometry before the mouse click. Fresh pointer tests omit
that text-only helper and pass; `actual-*` and restored/spacing captures are
the accepted evidence. No production pointer-routing defect is inferred.

Both actual IDEs pass design drag from (20,25) to (30,30), File-menu read-only
Save refusal with old bytes/dirty state retained, toolbar retry, Undo/Redo,
Unicode source editing/save and the About menu/dialog/return path. Windows
also proves a reader can allow ordinary write-open while denying atomic replace;
refusal retains disk and buffer, retry succeeds after releasing the handle.
Both quit flows preserve dirty Unicode after refused Save All and Escape,
then successfully save the pending buffer through the actual default Save All
before native exit 0. Independent model/source/session readback and **zero
remaining owned-path gopls processes** pass. Original generated companions
from both IDE saves compile as isolated consumer packages, not SDK-root files;
this is package compilation, not generated-application runtime acceptance.

Actual IDE quiet samples: macOS About, 10.03 seconds, 0.10 CPU seconds (~1.00%
of one core); its exact owned gopls has zero delta. Windows small-fixture editor,
10.01 seconds, 0.265625 CPU seconds (~2.65%); both exact owned-path gopls
processes have zero delta. Samples exclude transient grandchildren/GPU and
are neither FPS, startup/typing latency nor a large-workspace/global guarantee.

## Bounded measurements and remaining product work

A test-only benchmark overlay leaves the frozen base untouched. On Apple M2
Max, three 200ms repetitions measure warm admission through 32 nested layouts:
VBox 186–188ns, HBox 243–245ns, **0B /0 allocations** per request. A burst of
64 setters plus the complete queue drain takes VBox 74.7–83.6us and HBox
587–589us, about 2,040B /48 allocations per burst. These are logic/queue costs,
not rendering FPS or a speedup comparison against the incomplete old layout.

- No all-tree atomic transaction or universally constant-time layout is claimed.
  Direct arbitrary recursive custom `Layout` calls and arbitrary mutations in
  getters remain outside the nonmutating measurement contract. Explicit bounds
  callbacks that themselves reenter/reparent require further dedicated review.
- Windows remains SYSTEM_AWARE, not PMv2. Multi-monitor coordinates/input/IME,
  modal overflow scrolling, broad editor/LSP and industrial SDK work continue.
- SDK captures expose GroupBox border-corner path/curl artifacts; disabled
  menu cancel actions still have generic block-like icons. Separate path/icon
  investigation is required; neither is fixed or root-caused by this increment.
- One immediate Windows read-only screenshot has incomplete leading source
  glyphs; later sharing/saved/cancel captures show the complete text. macOS
  similarly retains an immediately smaller post-modal capture followed by a
  stable full-sized one. Storage readback is exact. Capture/compositor versus
  first-frame damage has not been diagnosed; do not claim it permanently fixed.
- Prior published results and screenshots retain their original scope; do not
  relabel `055a84d` or its captured pixels as this new source's acceptance.
