# UI-admitted bounded design reload — verified increment

Baseline: `76978b3c31a25e07ec0fd33782a2b236ca4d52b2`.
Evidence root: `bin/qa-artifacts/design-reload-20261116/` (ignored QA artifacts).
This is a designer/SDK increment, not complete Qt Creator parity or a binary release.

## Reproduced public flow

The baseline watcher reads/parses an AST on its worker and discards the result.
Its posted UI callback then calls LoadTDocFile for equality and again via
openDesignDocument. A default-preserving file-reader overlay and controlled
regular-file reader prove the actual guarded UI callback waits:

```text
baseline: reload_ui_return_before_physical_reader_released=false
corrected: reload_ui_return_before_physical_reader_released=true
```

The baseline failure is intentional and retained, not measured slow-drive
latency or attribution of all Windows CPU issues. The corrected regression
exercises real production admission and owned UI delivery. Removing the file
after preparation but before delivery proves installation does not reopen it.

## Implementation and invariants

- SDK `NewInvalidations` preserves the existing AST-based `New` API, debounce,
  bounded paths, serialized callbacks/errors and Close/Wait, but reports only
  paths without redundant source read, decode or AST projection.
- A revocable atomic mailbox holds one latest relevant path and one queued UI
  wakeup. The watcher callback retains the mailbox rather than directly capturing
  a canvas; closing/retargeting revokes old queued UI authority.
- UI admission captures the exact scene/path, Undo metadata, drafts, capture,
  mode, focus and watcher identity before bounded background preparation.
  Automatic reload and manual Open share one physical reader plus one
  replaceable latest request. Cancel/timeout cannot replace blocked OS readers.
- The existing bounded snapshot primitive checks regular-file identity, size,
  gzip and form root: 32 MiB compressed/expanded and 100,000 nodes. All source
  stat/read/decode stays off UI; reload ignores recovery siblings and prompts.
- Only the still-owned captured TDoc may install. Preserve exact designer grid,
  fractional bounds and handler source without a lossy AST roundtrip.
- Identical own-save notifications preserve widget pointers and Undo. Reload
  does not reveal a hidden canvas, focus another widget, mutate MRU or reset
  autosaver/watch host state. Clean hidden designs may update without changing
  the active editor's source/caret owner.
- Explicit manual Open wins over automatic events, including its nested
  discard/Save admission stage. A failing regression exposed this boundary;
  a dedicated admission reservation fixes it without dropping later user intent.
- Normal model mutations still need existing design-change notifications.
  Arbitrary unreported plugin writes are not covered by the ownership contract.

## Final frozen-source gates

Candidate 1: **2,753 files / 2,018 Go files**; archive SHA-256:
`3cc36115bba348b0e8ec3ac3da98d1ce1c361b1b3a0c76f2935f54380ae6ce38`.

| Platform | Full ordinary pass / skip | Full race pass / skip | Strict bundled gopls |
| --- | ---: | ---: | ---: |
| macOS arm64 | 9,163 / 29 | 9,163 / 29 | 4 pass / 0 skip |
| Linux arm64 / Xvfb, fresh R3 | 9,102 / 37 | 9,102 / 37 | 4 pass / 0 skip |
| Windows 10 amd64 | 8,952 / 42 | 8,952 / 42 | 4 pass / 0 skip |

Each runner and its own ordinary/race/strict exit files report zero. Counts
include subtests; platform/environment skips are retained, not claimed as
coverage. Runs use `-count=1 -p 2 -timeout=20m ./...`, normally and with `-race`.
Ten focused race repetitions also pass 1,060 named tests with zero skips.

Strict real bundled-gopls checks, each passed without skips on all platforms:

- `TestEditorRenameRealGoplsKeepsUnsavedDefinition`
- `TestGoplsUnsavedUnicodeCompletionAndFormatting`
- `TestRealGoplsIncrementalUnicodeCRLFEdits`
- `TestRealGoplsNavigationUnsavedTypeImplementationAndBack`

Source pre/post manifests and independent Linux/Windows post-source copies
match all 2,753 files with no extra Go. Generated fixture modules stay outside
SDK source. Final indexed/runtime inputs are checked against this snapshot;
only reviewed documentation/captures follow freezing. The unchanged,
noncompiled `core/callback.go.txt` CRLF/LF Git normalization is separate.

## Current-source native acceptance

Both stock apps use fresh private profiles and bundled gopls, without
instrumentation. Screenshots, helper inputs and independent model/settings
readbacks remain under candidate1/mac-native and candidate1/windows-native-evidence.

- Open target and rewrite it externally: the visible button becomes `Reload ready`.
- Drag (20,25) to (30,30), save and let the own-file notification settle. Actual
  Cmd+Z/Ctrl+Z restores (20,25), selected-widget controls remain usable, and
  independent saved-model readback confirms both exact coordinate sets.
- Make local unsaved (30,30), externally write (20,25): the dirty canvas retains
  (30,30) while independent disk readback proves the external (20,25) exists.
- Save the local choice, open source.txt, type, externally update the hidden
  clean design, then append through the keyboard without clicking another
  editor. Saved text is exact: macOS `SOURCE hidden reload input + focus retained`;
  Windows `SOURCE hidden reload input focus_retained`. Edit stays visible.
- Returning to Design shows the external (20,25) model and clean state, proving
  the hidden reload happened rather than merely leaving the source untouched.
- Normal Cmd+Q/Alt+F4 exit independently reports **0**; final settings retain
  target.silkui, Design mode and the source tab. Windows exit is read using a
  process handle held before close and GetExitCodeProcess, not disappearance.
- Scoped post-close process readback finds no owned bundled gopls child.

README uses unmodified current-source native captures:
`docs/images/design-reload-focus-macos.png` and
`docs/images/design-reload-focus-windows.png`. They are not mockups, FPS or
whole-product performance measurements.

## Retained failures and environment recovery

- Original unused legacy decl import compile failure and the pre-reservation
  manual-admission regression remain separate from later passing outputs.
- The first Linux race attempt fails while compiling with `no space left on
  device`, not an application assertion or data-race report. Preserve its raw
  JSON/stderr and independently copied source before reclaiming only exact,
  terminated, project-owned test containers whose source/results are retained.
  Do not prune unrelated images/volumes/containers or user data.
- Recovery-harness optional metadata-key/path preparation failures and an
  exit-2 unstaged-runner container are recorded separately. No failed container
  was restarted or its original logs overwritten. A fresh R3 reruns the entire
  ordinary/race/strict sequence against unchanged source and assertions.
  Exact cleanup scope/configuration is recorded in candidate1/linux-disk-recovery*.json.
- The initial native-build harness addressed a nested module from the outer
  root; its failed command is retained. Corrected build2 runs inside frozen src
  and is verified before the only macOS native launch. Windows UI tasks use the
  already-corrected Unicode caption/owned-window harness, not guessed exit codes.
- The existing optional property/ged.cml default fallback warning is retained;
  no untracked replacement asset was invented to hide it.

## Remaining product work

UI model snapshot/equality and widget/resource construction remain UI work.
Save, autosave transactions, companion generation and external conflict/dirty
review require separate changes. Dirty-scene notifications are still ignored
rather than replacing user work; no conflict-resolution UI is claimed.

A separate default-preserving public Save writer probe now proves manual Save
still waits for physical writing (`bin/qa-artifacts/design-save-20261116/`). Its
next-step contract also covers atomic failure preservation, clean markers,
autosave ordering and owned close/open/quit continuations. It is not a fix or
an acceptance gate for this reload increment. The wider product goal remains
active. Final source/index/whitespace checks pass: 2,747 non-exempt frozen files,
including all runtime/build inputs, remain exact. Reviewed document/capture
updates and unchanged legacy normalization are recorded separately. Commit/push
still requires final remote identity verification.
