# GroupBox rendering and inherited input availability — verified source increment

Baseline: `ea4ff29e9f8b2b8a0c74112ccbc4d3f5f2939577`.
This is part of the continuing SilkIDE/designer/SDK product goal, not full
Qt Creator parity, a binary release or industrial-control certification.

## Reproduction and implementation

- The old counterclockwise border used positive arcs: four 270-degree sweeps.
  Retained raster replays at 1x/1.25x/2x/3x expose inward curls. Four bounded
  negative quarter arcs, half-pixel inset and allocation clipping repair this.
  A true caption gap replaces background erasure; empty titles retain a border.
- Constrained captions use middle elision without changing `Title()`. Natural
  metrics and one allocated-width fit have bounded caches, sensitive to theme
  and mutable font attributes. Value-comparability checks avoid panics for
  direct/nested noncomparable custom metric fonts. This does not make native
  Cairo drawing accept arbitrary external Font implementations.
- Explicitly restore the caller's font: legacy Cairo Save/Restore intentionally
  does not restore Go-side style fields. Global painter semantics are unchanged.
- A native macOS consumer of the published baseline delivered a mouse action
  inside an unchecked GroupBox. Direct content was disabled, but the nested
  button still reported enabled. Space was already blocked by ancestor checks;
  this was a real mouse/keyboard inconsistency, not a claimed keyboard bypass.
- Effective enabled-state inheritance and optional `IChildEnablement` gates now
  block nested native pointer/key/text/wheel input and Tab selection. GroupBox
  gates only its currently owned content, not its interactive header; children’s
  explicit flags survive check/uncheck, detach and replacement.
- Disabling or reparenting into an unavailable subtree retires old gestures,
  repeat actions, pending clipboard reads and transient editor hints, without
  fake release/click/commit events or document replacement. Cancellation-time
  reparenting cannot revive the original capture. Both native backends clear
  discarded capture backing slots on pop/replacement; restored independent
  capture is preserved without retargeting the canceled mouse event.
- A Windows full-suite allocation failure led to isolating the unchanged pure
  line-splice fixture in a bounded test-binary child. The same five runs and
  **<= 4 allocation** bound remain; automatic GC is disabled only during that
  child measurement, with an explicit collection and restored policy. No
  production editor operation or application GC policy changes. Corrected
  Windows ordinary/race GUI runs and 100 isolated checks per mode pass.

## SDK contracts and compatibility

- `Widget.IsEnabled()` reports effective availability, including derived checks
  such as Button Action state. Persist `IsExplicitlyEnabled()` for authored
  flags. `IWidget` has no new required methods.
- `IChildEnablement.IsChildEnabled(child)` is a pure UI-thread getter for direct
  children. Do not call `child.IsEnabled()` recursively. After changing a custom
  gate, call `child.NakedWidget().InvalidateEnabledState()` on each affected
  direct child; this needs no polling timer or worker.
- Older external controls overriding `IsEnabled()` locally cannot bypass native
  admission: the router separately validates the full input scope. They should
  still delegate to the embedded Widget for consistent painted state. This can
  rewalk ancestors; no universal asymptotic performance claim is made.
- Native `WtChild` inherits availability; independent `WtForm`/`WtPopup` parentage
  denotes ownership, not an input gate. Explicit programmatic Action triggering
  is not disabled by a container's UI gate.
- `HasFocus()` is false while ineligible, but `CurrentFocusWidget()` can retain
  the remembered identity so temporary disable does not redirect later input.
  External language-service hosts must still fence stale replies, for example
  using `LanguageHintRevision()`. Not every host-owned operation is canceled.

## Final frozen-source gates

Candidate **`final5`**: **2,832 files / 2,075 Go files**.
Archive SHA-256:
`13fa0a1be8534c60cc0a920e5662902db6f142484c976f25fc8cdfdf3ca13243`.

| Gate | Final result |
| --- | --- |
| macOS arm64 full / race | 9,538 named passes / 29 skips each; exit 0 |
| Linux arm64 full / race | 9,477 named passes / 37 skips each; exit 0 |
| Windows amd64 full / race | 9,326 named passes / 43 skips each; exit 0 |
| macOS focused race, 20 repetitions | 1,340 named passes, no skips; exit 0 |
| Strict actual bundled-gopls, all three platforms | Four integration tests per platform, no skips; exit 0 |
| Pure renderer / pure GUI subset | 234 passes / 1 skip; 67 passes / no skips; exit 0 |
| Native GL transfer / platform and release scripts | exit 0 |
| CI-scoped vet | exit 0 |
| Full vet | 501 unchanged baseline diagnostics; no new or removed diagnostics |

Named pass counts include named subtests; skips remain skips, not passes. Linux
has no native desktop acceptance in this increment. Full/race commands use
`-count=1`; actual scripts, command lines, stderr and JSON streams are retained.
The strict gopls runs supplement optional integration tests in the broad suite.
All three independently exported source trees match every frozen hash, with no
extra Go files. The previous vet baseline's 2,070 Go/module blobs match baseline
HEAD before comparing normalized `(file, message)` diagnostics.

## Native acceptance and readback

- **macOS SDK:** all six layout-example actions in both directions, keyboard
  font toggle and native resizing to 452 logical pixels. Long Unicode title,
  wrapped content, following row and reflowed actions stay separately laid out.
- **macOS and Windows nested-input probes:** journals record counts
  `0,0,0,1,2,2` for startup/show/check/mouse/Space/uncheck. Both actions are
  blocked before checking and after unchecking. Both probes exit normally, 0.
- **macOS IDE:** actual drag `(20,25)` to `(30,30)`, save and independent model
  reload; design/source Undo/Redo, exact Unicode source persistence and restored
  active-file session; actual Help/About; normal exit 0 with zero owned gopls.
- **Windows 10 IDE:** File Save refuses a read-only design without changing its
  hash; a later toolbar retry saves `(30,30)` and design Undo/Redo passes. Source
  read-only and replacement-denying sharing handles preserve original bytes;
  retry persists exact Unicode text and Undo/Redo retains the saved baseline.
  Actual Alt+F4/default Save All refuses read-only source without closing;
  Escape cancels, Undo restores clean state, writable retry saves and exits 0.
  Independent final disk/model/session readback passes.
- Both actual IDE-generated companions compile in isolated consumer modules;
  no generated `main` is placed in the SDK root. Compilation is not a generated
  application-runtime acceptance claim.
- One quiet Windows small-fixture interval lasts 10.013 seconds: IDE uses
  0.109375 CPU seconds (1.09% of one core); two exact bundled-gopls processes
  use 0.046875 and 0 seconds. This excludes transient grandchildren/GPU and is
  not an FPS, startup, large-workspace or universal performance benchmark.

### Failed or superseded attempts are not passes

- Early candidates found an unavailable-root keyboard fallback mismatch,
  PopCapture mouse-event retargeting, capture-stack backing references, the
  allocation-accounting failure and a nested metric-font comparison panic.
  They were fixed before final5; F1–F4 are not final acceptance. F4 was canceled
  through exact owned-process evidence, not labeled a product crash.
- Intermediate Windows repair scripts had argument-quoting/working-directory
  errors. Corrected runs are separate; original failure markers remain.
- The first final5 macOS probe did not confirm keyboard activation; later mouse
  diagnostics do not retroactively prove Space worked. A fresh isolated profile
  with the **same binary payload**, explicit AX Raise and separate mouse/key
  checkpoints passes. The original surface/focus cause remains unassigned.
- A macOS source-open attempt used stale dock coordinates and did not open the
  source file. Corrected geometry, actual source-tab state and independent
  persistence establish the accepted replay. A tiny transitional compositor
  capture is retained but not used in README; no product cause is asserted.
- The first Windows toolbar retry after read-only failure did not dispatch a
  successful save within the observation window. The app stayed alive/dirty,
  original bytes were intact and the file was writable. Fresh owned-window
  inspection followed by a separate toolbar retry in the **same process**
  succeeded. `native-design.exit=1` remains; `native-designretry.exit=0` records
  the accepted continuation. This does not prove first-click determinism or
  diagnose an input/save-state root cause. Further menu/capture timing replay
  remains work, not a falsely green original automation step.

## Evidence and publication scope

Evidence root: `bin/qa-artifacts/groupbox-paint-20261116/final5/` (ignored QA).
Key replay artifacts: `run-mac.sh`, `linux/run.sh`, `windows-run.ps1`,
`gate-counts.json`, `strict-lsp-verification.json`, `source-and-vet-audit.json`,
native journals, screenshots, generated-consumer build logs and final independent
readbacks. Windows uses the requested win10testcomputer interactive-session
workflow; SSH session 0 is not treated as desktop input evidence.

README includes three original, unedited final-source captures: the macOS narrow
SDK example and actual macOS/Windows designer saves. Source origins and SHA-256
values are retained in `readme-publication-captures.json`; QA-only input-probe
captures remain in the evidence tree. Reviewed result documents/media may differ
from the frozen manifest, but runtime Go/module source must not. The unchanged
`core/callback.go.txt` has existing worktree CRLF versus HEAD LF; normalized
content matches and it is not staged. Preserve unrelated user artifacts.
Commit/push is a source publication, not a release/tag. Remote HEAD must be read
back after push. The continuing product goal stays active.

## Remaining product work

GroupBox keyboard/inspector authoring, independently owned popup/action/modal
lifecycle, first-retry click timing, Windows PMv2/IME and large-workspace
scheduling/profiling remain open. This does not certify all controls, native
states, idle FPS or complete industrial HMI/Qt Creator capabilities.
See [product plan](PRODUCT_ITERATION_PLAN.md).
