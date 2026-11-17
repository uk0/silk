# Bounded property inspector — development verification

Baseline `590c36a086d9fdc1219564a3d94e377f53b98d87`. The wider
SilkIDE/designer/SDK goal remains active; this is not whole-product completion,
a Qt Creator parity claim, or a new release/tag.

## Changes and preserved failures

- Long property sheets previously had scrollbar range 0 and absolutely
  positioned editors directly beside the fixed search control. A bounded body
  now clips both paint and input. Logical labels, category hits and actual editor
  geometry follow one scroll offset; filter/collapse/resize/Clear clamp it.
- SDK focus reveal is optional and respects independent native ownership,
  callback focus transfer, disable and reparenting. Optional visual child order
  preserves Tab order when a filter causes later editors to materialize first.
- Scroll only repositions existing controls. It does not read/write model values,
  rebuild bindings or split a continuous typing transaction. Moved inline
  drop-downs are dismissed without deactivating text editors. Paint traverses
  only the visible label/header interval; no timer or idle polling is added.
- Native macOS exposed a missed input boundary: `gui.Edit.OnMouseWheel` swallowed
  wheel input in single-line mode. Preserve its failing control, forward property
  text/numeric wheel input to the sheet and retain multiline text scrolling.
- Correct a designer test's direct-child assumption to inspect real descendants.
  Isolate modal-frame wiring tests from previous fixtures' queued layout work;
  production still yields after 64 UI tasks or 4ms. A test is not justification
  for weakening that input/rendering fairness budget.

## Candidate identity and acceptance status

Final candidate is `bin/qa-artifacts/property-scroll-20261117/proposal3/`:
2,853 files, 2,087 Go files, archive SHA-256
`9af5158d7c4f866dd5cc729b7f985e6bff8f7cf1925cd955676020d67d499a1f`.
Final-result docs/media are reviewed separately; product Go is unchanged after
freeze. Earlier candidates are not final acceptance: first had test-fixture/
unindexed-export failures; second missed native editor-wheel input. Obsolete
Windows coordinator trees were explicitly retired after recording ownership and
source supersession, not restarted or counted as passing tests.

| Platform | Full passes/skips | Race passes/skips | Strict real bundled-gopls |
| --- | ---: | ---: | --- |
| macOS arm64 | 9,625 /29 | 9,625 /29 | 4 passed, no skips |
| Linux arm64, Xvfb | 9,564 /37 | 9,564 /37 | 4 passed, no skips |
| Windows 10 amd64 | 9,422 /43 | 9,422 /43 | 4 passed, no skips |

Named counts include subtests; skips are not passes. All platform failures are
empty and all runners exit 0. The renderer-tag property/GUI subset has 158
passes, no skips. Scoped vet passes; default full vet retains exactly 501
unchanged baseline diagnostics, none new or removed (it is not clean).
Independent Linux/Windows exports and the local frozen source each match every
manifest hash, with no extra Go files. Native acceptance and post-native source
checks also pass; publication requires the exact index/media audit and matching
remote commit readback. Previous candidates do not substitute for final results.
One observer saw the final coordinator's exit-0 marker just before the scheduler
left Running; the same task was subsequently confirmed Ready/result 0, without
restarting it. Its full/race/strict results and post-test archive remain unchanged.

An isolated macOS M2 Max presentation-only benchmark (three 200ms samples) is
15.6–16.1 microseconds per changed offset for 100 fields and 157.6–160.7
microseconds for 1,000. It excludes drawing/native input and is **not** a FPS or
cross-platform guarantee. The 1,000-field case still allocates about 208KB /
4,996 allocations per scroll; positions remain O(n). This is an explicit
optimization baseline for later work, not a claim of zero-allocation scrolling.
Allocation profiling attributes almost all sampled allocation objects to
`PropertyItem.ItemHeight` / configuration key splitting and `Widget.Children`
snapshots. A later optimization should cache resolved row geometry and avoid
unnecessary child snapshots, while retaining callback/ownership safety.

## Native macOS

Final profile `/private/tmp/silk-property-scroll-p3-20261117`, payload SHA-256
`065d2e116c71f1d5b125fa8a9de8b379c895624113eaefc880f9bf9b10838fd9`.
Adjacent bundled gopls connects in the actual IDE.

- A compact 1300-point-wide window exposes a real long inspector: wheel over
  labels and single-line editors in both directions, scrollbar drag, fixed search
  and clipped partial rows. Tab reveals X and then the lower Object Name field.
- Filter by Title resets range. Save `Scrollable inspector verified 20261117`,
  toolbar Undo/Save and Redo/Save each have independent current-SDK document reads.
  Horizontal caret scrolling was checked with Home, not mistaken for stale text.
- Scrolled Behavior header collapses at its rendered location. New then native
  Open selects the exact current profile's AX file URL; the saved title and final
  session read back correctly. Kqueue observes PID 78153 normal exit 0, with no
  owned IDE/gopls left. README media are original unedited captures.
- The actual new IDE companion is copied unchanged into a separate consumer,
  whose current-SDK module identity and race-instrumented constructor/runtime
  test verify title, content ownership, child state/value and restored input.

## Native Windows 10 amd64

Final session-1 IDE PID 6132, payload SHA-256
`238d16f4fff5a98e72fbef5e2f2dffd1b78a9b409d20d2de111631ba8fade92b`.
A 1460x560 native window exposes the bottom-of-pane boundary rather than hiding
it in a tall window; the adjacent real bundled gopls is connected.

- Wheel over the actual Title editor reaches the bottom and returns to the top.
  Tab reaches X then the lower Object Name field. Search remains fixed and
  partially visible editors cannot paint/hit across its band or the scrollbar.
- Click the scrolled Behavior header, temporarily reveal it with a category
  filter, clear to restore collapse, then filter Title. Independent current-SDK
  reads confirm view-only operations leave the authored model unchanged.
- Save `Windows scroll verified`, toolbar Undo/Save and Redo/Save; each separate
  read confirms the correct title and preserved group state/event. New yields
  untitled, then the **current** file-tree document reopens. Clear search,
  expand Behavior and drag the real scrollbar; the final original screenshot
  shows the accessible bottom fields.
- A held process handle confirms normal exit 0. Final document/session agree;
  the owned IDE and both bundled gopls processes are gone. The post-native
  audit preserves all 2,853 source hashes and finds no extra Go files.
- The actual new generated companion is copied unchanged and executed with
  race against the exact current SDK. SHA-256
  `a715983f4217ce29d01bdfb02682f9ff94f28ec99d0c5be07b7ee120dbe68cdd`
  matches the original and consumer copy; constructor/state/input checks pass.
- Quiet small-fixture observation: 10.0094 seconds; IDE consumes 0.109375 CPU
  seconds (1.093% of one core); bundled gopls consume 0.046875 and 0 seconds.
  This is process CPU, not a FPS/GPU/startup/large-workspace or universal
  performance guarantee. No native test app, observer or worker is left running.

## Limits and next product work

Expanded rows still materialize editor controls: visible-range paint is **not**
full widget virtualization. Custom native popups may need their own lifecycle
contract; tests do not certify every third-party control. No claim of universal
FPS/CPU, per-monitor DPI/IME, production industrial control safety, completed
plant routing/alarms, or complete Qt Creator/editor/LSP parity is made.
