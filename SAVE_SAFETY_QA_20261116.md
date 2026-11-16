# Fail-safe document saving — 2026-11-16

Status: frozen-source package gates and current native acceptance complete.
Approved for source commit/push after final index identity review. This is a
synchronous safety prerequisite, not async Save or a new binary release.

## Counterexample and change

On published `8665beb`, an unchanged SDK `Save()` with an extensionless destination
inside a missing parent returns false but changes `Filename()` to add `.silkui`.
A separate test-only overlay reproduces that failure against the frozen source.
The earlier controlled physical-writer probe still proves synchronous UI Save;
atomic staging does not correct it and is not a storage-latency measurement.
The same public `performSave` probe was also rerun against this frozen candidate
with only a default-preserving atomic-writer boundary overlay: it again reports
`manual_save_ui_return_before_physical_writer_released=false; saved=true` and
fails as expected. `candidate1/sync-writer-current/` retains this current-source
negative evidence; it is not mixed into passing package-gate counts.

New `TDoc.SaveFileAtomic` / `SaveFileAtomic1` stage in the existing parent,
retain short/write errors even if an encoder ignores them, complete encoding,
Sync and Close before platform `ReplaceSnapshotFile`. Failure before replacement
leaves the original bytes/inode intact; no remove-then-rename fallback. The
destination is checked before/after encoding using identity, mode, size, mtime
and streamed SHA-256. Read-only files, directories and leaf symlinks are refused.

`GedScene.SaveTo` preserves old path/title/dirty history on failure, commits UI
drafts before capture and only adopts a successful Save As path. Nested saves
and draft-callback retargets are rejected. Existing Save stays synchronous and
returns durable success, not queue admission. Editor Save and automatic recovery
use the same staging primitive. Legacy raw `TDoc.SaveFile` remains unchanged.

## Scope and evidence

- Artifacts: `bin/qa-artifacts/design-save-20261116/atomic/` (ignored).
- Old failure: `baseline-identity/probe.log` + `.exit`, original source hashes
  and a test-only overlay. Existing `../baseline/` retains the blocking probe.
- Focused cases: encode/create/chmod/short-write/write/sync/close/replace failures,
  ignored write error/panic, mode/content/identity changes, new/deleted targets,
  held snapshot reader, read-only/nonregular paths, plain/gzip roundtrip, failed
  first Save/Save As, pending draft Undo, retarget/reentry and recovery preservation.
- Editor failure retains exact unformatted Unicode buffer/revision/dirty state;
  retry after permission repair persists the formatted buffer and retains mode.
- Final focused race regression: 10 repetitions, **450 named passes**, no skips.

### Frozen platform gates

Source: **2,763 files / 2,025 Go inputs**, archive SHA-256
`d7b1f92e6214ebeb10d8c6175bfafa76723158a2d767651932930a119bbcdc99`.
Each platform verifies all hashes before/after testing; independent exported
Linux/Windows source copies also match, with no extra Go files. Final reviewed
documentation/captures are separately audited against the staged Git tree.

| Platform | Full named pass / skip | Race named pass / skip | Strict real bundled gopls |
|---|---:|---:|---:|
| macOS arm64 | 9,201 / 29 | 9,201 / 29 | 4 pass, 0 skip |
| Linux arm64 / Xvfb | 9,140 / 37 | 9,140 / 37 | 4 pass, 0 skip |
| Windows 10 amd64 | 8,989 / 43 | 8,989 / 43 | 4 pass, 0 skip |

All final runner/full/race/strict exits are 0. These are named test/subtest
counts, not a claim of zero skipped cases. Windows adds one explicit symlink
skip because the test account lacks that privilege; macOS/Linux exercise it.
The retained expected-negative overlays are not passing-suite cases.

### Current native acceptance

- Both exact source-built apps close normally with observed exit **0**; macOS
  uses a process kqueue and Windows holds a process handle before Alt+F4 and
  reads `GetExitCodeProcess`. No owned bundled-gopls process remains.
- Both refuse read-only design/source writes without changing original hashes;
  edited state stays dirty. Repairing the test file's permissions permits retry.
  The button saves at `(30,30)`, Undo shows `(20,25)` dirty and Redo is clean.
- Windows additionally holds a real `FileShare.ReadWrite` reader without delete
  sharing. A nontruncating direct write-open is allowed, but atomic replacement
  is rejected. Exact original bytes and dirty editor survive; disposing the
  handle permits retry. No `.silk-write-*` files remain after failure/success.
- Exact saved source: macOS `SOURCE safe save macOS 😀\n`; Windows
  `SOURCE safe save Windows 中文` without a trailing newline. Source Undo/Redo
  retains the saved baseline; final disk/session/model are independently read.
- Final sessions stay on the original design in Edit mode with one source tab;
  no Save As retarget or hidden-scene substitution. README PNGs are the original,
  unmodified captures of the rejected source Save, not mockups.
- The native apps use explicit toolchain/Cairo test environments. Linux is an
  Xvfb package gate, not a newly claimed manual Linux desktop/package acceptance.

Compact replay/evidence pointers under `candidate1/`:

```text
manifest.json, identity.json, three-platform-verified.json
mac-r2/{mac-full,mac-race}.jsonl, mac-r2/mac-lsp.log
linux-r2/{full,race}.jsonl, linux-r2/lsp.log
windows-results/{full,race}.jsonl, windows-results/lsp.log
mac-native/{acceptance,final-readback,normal-exit}.json
windows-native-evidence/native-session/{source-acceptance,final-readback,normal-exit}.json
sync-writer-current/{identity.json,overlay.json,probe.log,probe.exit}
vet-baseline-comparison.json, NEXT_ASYNC_SCOPE.md
```

## Retained intermediate failures and additional checks

- The first frozen Mac full run failed the existing source-audit guard because
  the fresh source directory lacked its own Git index. No assertion was relaxed:
  initialize/index the Go inputs as the Linux/Windows runners do, then run fresh
  full/race/strict R2 gates with unchanged source. Original logs remain.
- The first Linux full run reported one `os.ErrNotExist` in the unchanged
  concurrent settings-reader test with host-bind temporary files. Twenty
  unchanged-test repeats on each of bind and native temporary storage then
  passed; **root cause is not established**, not called a fixed product defect.
  A fresh R2 uses a native Linux tmpfs for build/test temporaries, host-bind
  compiler cache and the unchanged full/race/strict suite. No failed job restart.
- Docker preflight showed zero available filesystem bytes. The previous exact,
  terminated `silk-reload-candidate1-r3-20261116` test container was removed only
  after verifying its independently exported source hashes and successful raw
  full/race/strict logs. No broad prune or unrelated container/volume removal.
- Optional broad `go vet` reports 67 existing unkeyed-literal warnings on both
  published baseline and candidate. Exact warning/file comparison (line offsets
  normalized) shows no additions. Do not label this extra vet run a clean pass.
- Current macOS native Save preserves a refused read-only design, retry saves
  the selected button at `(30,30)`, Undo returns `(20,25)` and Redo restores it.
  A refused read-only source Save keeps `SOURCE unchanged\n` on disk and the
  Unicode edited buffer dirty; retry writes `SOURCE safe save macOS 😀\n`.
  Source/session/model readback and observed normal process exit are retained.

## Deliberate limitations and next work

- No idle polling or new dependencies, but **all current IDE Save paths are still
  synchronous**. Fingerprinting adds reads; no performance/latency win is claimed.
- Worker code must receive an independent snapshot, not a scene. Async manual/
  recovery ordering, bounded physical slots, late completion/Undo protection,
  quit/open/New/save-all continuations and companion preparation remain pending.
- Design Save failure currently logs a warning without an error toast; the dirty
  marker/disk preservation is verified, but explicit UI failure feedback is a
  follow-up, not claimed complete by the new error-returning SDK SaveTo API.
- Scope is SilkIDE Save/recovery and the new SDK API. The separate legacy root
  `design.go:onFileSaveAs` still calls raw `TDoc.SaveFile`; workspace replace has
  another direct writer. They are not silently claimed migrated by this change.
  SilkIDE does not gain a dedicated Save As menu merely from adding `SaveTo`.
- Optimistic checks cannot exclude another process in the final check/rename
  window. Parent symlinks are followed. Replacement breaks hard-link aliases;
  extended ACLs/ownership are not copied. No cross-process lock or blanket
  power-loss/network-filesystem atomicity claim is made.
- No new release/tag; broader industrial/IDE compatibility work remains active.
