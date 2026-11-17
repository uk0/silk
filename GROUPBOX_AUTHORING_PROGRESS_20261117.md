# Checkable groups, generated content ownership and native capture

Baseline: `3f884d584e3c9f373dc8cbe0c600173a5436dc68`.
The wider SilkIDE/designer/SDK goal remains active. This is one development-source
increment, not full Qt Creator parity, a new packaged release or industrial certification.
Final code, platform and native gates pass; source publication still requires the
exact index/media audit and matching remote commit readback.

## Changes

- Checkable GroupBox headers join automatic Tab traversal; plain frames do not.
  Explicit NoFocus/TabFocus policies remain authoritative. Space toggles once,
  ignores repeated key-downs, and Enter does not toggle. The keyboard cue is
  clipped to the fitted header; disabled nested content is skipped in both directions.
- Stable `Checkable` / `Checked` property IDs have English/Chinese labels and
  survive save/load, frozen code snapshots and typed generation. GroupBox offers
  `OnToggled(checked bool)` in the inspector and event-navigation menu.
- Typed generated handlers connect after every constructor, authored scalar value
  and layout. Initialization does not inadvertently call business handlers;
  applications should explicitly call startup logic. Runtime setters still emit.
- The ordinary IDE decl companion now registers a GroupBox child through
  `AddWidget`, not merely `SetParent`. This establishes actual content ownership,
  layout and inherited input gates. `decl.Node` does not embed Go handler source:
  hosts bind callbacks through `BuildWithIndex`; typed SDK output wires handlers.
- Windows tracks only the exact source/target during its own native capture
  mutation. A matching synchronous acknowledgement is not an external loss.
  Unmatched loss, WM_CANCELMODE and focus loss still cancel input. Shared
  cancellation retires uncaptured/legacy presses, unwinds nested captures inside
  out, rejects reentry, clears retained slots and preserves independent surfaces.
- No polling/render timer, worker-owned widget mutation or required IWidget
  method is added. Existing child-enabled flags survive the temporary parent gate.

## Final frozen-source gates

`proposal2`: **2,842 files / 2,081 Go files**; archive SHA-256:
`827782c1510a8d7ad19fdadb70f4045c9755647fa4ccd47ba66f644a6d7a0984`.

| Native architecture | Full named passes / skips | Race named passes / skips | Strict actual bundled-gopls |
| --- | ---: | ---: | ---: |
| macOS arm64 | 9,553 / 29 | 9,553 / 29 | 4, no skips |
| Linux arm64 / Xvfb | 9,492 / 37 | 9,492 / 37 | 4, no skips |
| Windows 10 amd64 | 9,350 / 43 | 9,350 / 43 | 4, no skips |

All exits are 0 with no failed tests. Full/race use `-count=1`; named passes include
subtests, not a deduplicated count of top-level test functions. Platform skips are
retained, not called passes. Strict gopls tests exercise unsaved Unicode completion,
formatting, rename, type/implementation navigation and incremental CRLF edits.
They run the actual architecture-matched bundled server, not a mock.

- All three independent post-test exports match every frozen manifest hash and
  contain no extra Go files. The working/indexed Go and module audit is required
  again immediately before publication; final result docs/media are reviewed exceptions.
- Pure renderer: 234 passes / 1 skip; renderer-tag GUI subset: 76 / 0; actual
  native GL transfer: 1 / 0. Native GLFW GUI still requires CGO; only renderer
  packages are claimed CGO-disabled. Platform/release scripts pass.
- CI-scoped vet and additional `./decl` vet pass. Full vet retains exactly
  **501 unchanged baseline diagnostics**, no new/removed diagnostics; it is not clean.
- Both an actual IDE decl companion and a typed machine/user pair generated from
  the native-authored document compile and execute with race instrumentation in
  isolated consumers. Checked child state/value 42, group ownership, disabled
  input and restoration survive. Consumer `go list -m` points explicitly at proposal2.

## Native acceptance

### macOS

- Source-built IDE PID 35996 edits Checkable=true / Checked=false in the actual
  inspector. Save; toolbar Undo/save independently reads true; Redo/save reads
  false. The context event action creates/navigates to one correctly typed
  `onOptionsToggled(checked bool)` method. Save Code and document Save persist it.
  New then actual native Open reloads the properties/binding. Model, embedded
  handler and saved session are independently read back. Normal system Quit is
  observed as exit 0 through kqueue, not inferred from process absence.
- SDK consumer PID 33775 builds the **unchanged actual IDE decl companion** with
  proposal2 and explicit post-build host callbacks. Its GroupBox owns the VBox;
  no SetContent workaround is used. Native Tab/Shift+Tab, Space, Enter-no-toggle,
  nested checkbox/SpinBox, unchecked Tab skipping and mouse rejection pass.
  Disabled clicks at checkbox `(98,333)` / spin-up `(1125,397)` produce no callbacks;
  enabled same-coordinate controls produce checkbox callback 2 and value 44.
  Both 900ms Space holds produce one observed activation; repeat=true is separately
  injected by unit tests. Normal system Quit exits 0. Authored enabled flags remain true.
- Exact event sequence: `startup, shown, group, check, spin, group, outside,
  group, check, spin`; group counts `0,0,1,1,1,2,2,3,3,3`. The actual new native
  IDE companion hash equals the consumer input. Original JPEG captures, not
  retouched mockups or mislabeled PNG conversions, accompany README.

### Windows 10

- Final-source first toolbar retry after read-only File Save succeeds with one
  click, independently reads button `(30,30)`, retains Undo/Redo and exits 0.
  Three earlier fresh proposal1 IDE controls also pass the first retry and exit 0.
- Actual decl SDK GUI consumer PID 10896 passes the same ten-event sequence,
  keyboard state transitions and disabled/enabled same-coordinate controls
  `(57,164)` / `(571,194)`. It ends with group/check/value callbacks `3/2/2`,
  value 44, explicit child flags intact, and a held-handle native exit 0.
- Final-source IDE PID 7964 edits the new inspector values, saves, and independently
  reads true after Undo/save and false after Redo/save. Owned context/submenu HWND
  checks precede creating/navigating to the correctly typed event handler.
  Save Code/document Save persist it; New and actual file-tree reopen retain it.
  Document, embedded handler and saved session are independently read back; a held
  process handle confirms normal exit 0. Exact owned IDE/SDK/bundled-gopls processes
  remaining after shutdown: zero. The actual new Windows companion is compiled
  and executed against the same proposal2 SDK, separately from the native UI.
- The broader proposal1 IDE replay also preserves Unicode source, read-only and
  sharing-denied failures, quit refusal/Escape and successful Save All exit.
  Those are retained preflight evidence, not a claim that every final-source menu,
  popup, native DPI/IME state or large workspace was exercised.

## Why earlier green candidates were not published

1. The published payload reproduces a lost first toolbar retry in two fresh
   profiles. Trace-only published source shows main/foreground HWND `19465306`,
   but retry down/up reach hidden menu HWND `38667076`. Internal same-HWND capture
   acknowledgements had recursively canceled Button capture and emptied the logical
   stack. This is lost event admission, not a successfully dispatched save losing bytes.
   Trace patches and the original failed run remain outside product source.
2. Proposal1 passes full/race/gopls on all platforms, but **executing**, rather
   than merely compiling, its actual IDE decl output reveals Content()==nil and
   enabled children under an unchecked group. The exact new regression fails
   with proposal1 attach.go via a Go overlay, then passes the AddWidget repair.
   Proposal2 repeats the full gates; proposal1 is not final acceptance.
3. Harness errors remain separate: unmanaged/parent-local graph bounds produced
   an invalid layout fixture; a draft used an unsupported enabled literal instead
   of persisted `可用`; another used the wrong SpinBox callback signature. One
   P2-labeled consumer still pointed to P1 in its go.mod and panicked. Final
   consumers record actual module resolution, not just an archive label.
4. A retired Windows SDK console-subsystem probe selected the console as its
   MainWindowHandle (caption was the executable path) and exits `0xC000013A` when
   that console is closed. It is not accepted. The final GUI-subsystem consumer
   uses `-H=windowsgui`, the reviewed GUI caption and a held-handle exit 0.
5. An independent Windows read helper initially exits `0xC0000135` with empty
   output under the clean scheduled-task PATH. Extending only that process's
   toolchain/UCRT PATH makes the identical binary/read succeed, 0. Original
   failure and one-variable control remain recorded; it is not a failed GUI save
   or a standalone-package/DLL-closure certification.
6. A menu left open across separate scheduled-task sessions is dismissed before
   a later click. The accepted replay opens and selects it in one interactive
   task and requires the actual owned popup/submenu HWND, not merely the PID.
   Earlier wrong-target selection screenshots are retained, not called event binding.
7. The local Windows collection observer reaches its own deadline while tests
   continue. Only collection resumes after inspecting the exact job; the full
   test/coordinator is never restarted. Final raw logs and post-export are collected.

## Boundaries and next work

- A proposal1 Mac reopen's first observation showed incomplete child ink; ordinary
  selection repaint completed it with the model intact. No first-frame/compositor
  cause or permanent damage fix is claimed. Cached offscreen scaling, property
  categories, arbitrary popup/IME transitions, Windows PMv2 and packaging remain work.
- The wider industrial bindings/alarm/runtime and editor/LSP requirements remain
  active in the product plan. File Export remains SVG/PDF, not a typed Go export
  action. No all-controls/all-menu coverage or full Qt Creator parity is implied.
- A final-source small-fixture quiet ten-second Windows IDE sample measures about
  **1.25% of one core**, with its two exact owned bundled-gopls processes at zero
  CPU delta. The retained proposal1 sample is about 1.56%. These are process-CPU
  samples, not FPS/GPU, startup or large-workspace performance certification.
  No new polling timer is introduced.
- A direct dirty-root `go test ./...` picked up ignored legacy `Package/dc`
  artifacts. They remain untouched; clean isolated exports supply full-suite proof.
  QA traces, consumers and logs remain separate from product packages.
- README captures are source-built original app captures. Commit/push requires the
  exact tested/indexed Go/module audit and independent remote HEAD readback.
  No release/tag or completed overall product goal is declared.

Raw evidence: `bin/qa-artifacts/groupbox-authoring-20261116/` (ignored), with final
`proposal2/{mac,linux,windows}-gates.json`, `final-export-audit.json`, native profiles,
consumer module identities and retained failures. Reproduce SDK behavior with
`go test -count=1 -race ./gui ./ged ./prop ./decl` in the clean candidate; full gates
use `go test [-race] -json -count=1 -p 2 -timeout=20m ./...`, plus strict
`scripts/test-bundled-gopls.sh` against the actual matching bundle.
