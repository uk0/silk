# Asynchronous source opening and navigation — 2026-11-15–16

Baseline: `bac5463ffd6dfb3a7d5665f8972145679da783c7`.
This increment addresses interactive **source** reads and their navigation callers,
not every designer/Save/resource/macro/reload I/O path or complete Qt Creator parity.

## Reproduced public flow and correction

The baseline File/tree -> openDocument -> loadFileInEditor -> os.ReadFile path
waited inside the UI call. A separate overlay redirected exactly that read through
an injected, controlled regular-file reader; its default remained os.ReadFile:

```text
baseline: public_open_accepted=true return_before_reader_released=false
corrected: public_open_accepted=true return_before_reader_released=true
```

The baseline probe deliberately fails; the corrected probe calls the real product
entrypoint, proves an inert focusable pending tab/no premature MRU, then releases
the reader and checks exact Unicode source, focus and MRU. This is a controlled
blocked-reader demonstration, not measured drive latency, a FIFO issue, or the
cause of every Windows CPU spike. Baseline and failed development runs remain in
`bin/qa-artifacts/manual-open-20261115/`; no baseline overlay edits entered source.

## Implemented ownership and bounds

- Each workbench owns an asynchronous source opener: two physical executor slots,
  128 admitted jobs (including logically canceled still-running reads), bounded
  32 MiB source snapshots, canonical path deduplication and a metadata-only queue.
  The separate startup-restoration pool is not included in that two-reader bound.
- Thirty-second cooperative deadlines and explicit File/palette **Cancel File
  Opens** never join the reader on the UI thread or free a physical slot early.
  Stuck OS calls can occupy both slots; the UI remains usable, but opening further
  sources waits/rejects rather than spawning replacement threads.
- Loading/error pages are inert and closeable. Missing/nonregular/reserved paths
  never create editable empty files. Oversized files retain visible errors;
  pending source identity/location survives session capture.
- Workers retain atomic mailboxes, immutable paths/read functions and result bytes,
  not page/callback/frame references. Frame retirement clears mailbox ownership;
  late results cannot publish into another workbench.
- Deduped requests share one read/page and retain latest location callbacks.
  Obsolete results may install a background source but cannot navigate, update
  MRU/history or steal focus. Existing dirty buffers and Undo are never reread.
  Page replacement checks reentrant focus changes before transferring focus.
- Canceled queued entries leave the queue immediately; 100 repeated open/close
  cycles behind two blocked physical reads keep queue length zero and two readers.
  Renaming retires pending old/destination-path pages without waiting for reads.
- Source tree/menu, quick-open/location, search/LSP, bookmarks, reopen and generated
  source routes use readiness, not an immediate activeEditor assumption. Legacy
  tests use explicit test-only dispatch pumping; product opens never wait for it.

## SDK navigation contract

`PrepareNavigation(live, forward)` snapshots at most 255 ordered candidates without
I/O, caret/history mutation or events. Check `CanCommitNavigation` before applying
the exact still-owned target location, then call `CommitNavigation` only after
actual navigation succeeds. Failed/canceled opens discard visits; the IDE tries
unavailable history destinations sequentially and preserves the live return.

Any real history mutation invalidates visits, including clear/rebuild ABA and path
mapping; copies/siblings cannot commit twice. Exact no-ops/subscription changes
remain silent. Nonzero-sized opaque generations retain no stack/observer/frame
back-reference. Legacy mutation adds no allocations when unused; a full candidate
snapshot allocates once after initialization. CurrentPosition returns a read-only
copy; CurrentFocusWidget queries focus without activating a window. All are
UI-thread interfaces, not permission to touch widgets from a worker.

## Regression evidence

- Corrected SDK prepared-navigation tests: 20 named cases × 100 race repetitions,
  2,000 named passes, no skips/failures. Each parity test repeats 2,000 deterministic
  scenarios against legacy traversal (not 200,000 unique scenarios).
- Integrated source opener/focus/queue tests: initial 50 race repetitions,
  canceled-queue correction 20 repetitions, then expanded ownership/error/session/
  reordered/history tests plus CurrentPosition: 190 named passes over 10 race
  repetitions, zero skips/failures. Earlier failed focus and fixture runs retained.
- Test fixtures await completed effects explicitly. New async tests call actual
  entrypoints/getters directly and assert nonblocking pending state before pumping.
- The SDK-only prerequisite snapshot passed complete GUI+IDE ordinary/race packages
  on macOS (4,057/14 skips), Linux (4,022/19) and Windows (3,853/17). Its archive
  `89d916e30101f20ac496e4556bb80455f707f1055c7bec2ab31c6b27619e4958` is not the
  integrated source and is not substituted for final gates below.

## Frozen integrated candidate and final gates

Archive SHA256:
`a97c838b7dfabcaba74478dddca3247ccea938673c8cbf9ae8965b65de441e47`.
2,725 manifest files, including 1,997 Go files. The source root is separate from
all generated helper modules, native applications, profiles and test output.
Final documentation/screenshots may change afterwards; compiled/resource inputs
must independently match the frozen source before commit.

| Full repository, `-count=1` | Ordinary pass / skip | Race pass / skip | Strict bundled gopls |
| --- | ---: | ---: | ---: |
| macOS arm64 | 8,955 / 29 | 8,955 / 29 | 4 pass / 0 skip |
| Linux arm64 / Xvfb | 8,894 / 37 | 8,894 / 37 | 4 pass / 0 skip |
| Windows amd64 | 8,745 / 42 | 8,745 / 42 | 4 pass / 0 skip |

All six full suites exit zero, including generated SDK/industrial cases. Ordinary
optional/platform-specific skips are retained, not counted as strict LSP passes.
Each platform passes four strict bundled-gopls probes with zero skips. All 2,725
source-file hashes match after testing on every platform; independently copied
Linux and Windows post-source inventories contain no extra Go files.

The initial Windows runner exits 1 **after full/race both exit 0**: the harness
called silktools -verify before copying its tools directory. The failure/logs
remain unchanged. A separate corrected task copies the prior verified Windows
bundle, verifies its receipt/hash, runs the four real probes, builds the IDE and
rechecks source, returning runner-r2=0. No product source or suite result was
overwritten to conceal the harness error.

macOS native CUA verifies real quick-open
`target.txt:2:3`, no-mouse input/save, back/forward, close/reopen, oversized inert
error/close, independent source/settings readback and normal kqueue-observed exit
0. CUA captures are retained in the conversation, not exported screenshot files.
The first Enter before quick-open results were ready had no effect; it was not
counted as a successful open.

Windows native replay uses a fresh private profile, the frozen-source executable
and an interactive Session 1 task. Quick-open `target.txt:2:3` displays **Ln 2,
Col 3** and exact Unicode source; keyboard-only edits stay dirty until Save.
Independent bytes equal `manual open saved 20261115`; origin bytes are unchanged.
Back/forward, close/reopen, a 33 MiB inert error page and closing that page are
verified by task assertions and inspected captures. Final settings contain only
the two real editors, active target and preserved location; oversized bytes are
unchanged. A retained process handle plus GetExitCodeProcess observes normal exit
0, not merely process disappearance. Eighteen Windows captures across both
replays were inspected; README uses two unmodified R2 captures.

The first native close observer wrote a blank ExitCode from a Get-Process wrapper
and rejected it. The process was absent, settings persisted and stderr contained
no panic, but its actual exit status was **unknown**, not assumed zero or a proved
nonzero product exit. Preserve that failure. Repeat the entire workflow from a
fresh profile with a held handle: every action and explicit exit-code/readback
check returns zero. Native fixture logs include default-icon and no-Go-module
diagnostics; this is not a clean release-package asset/Go-project deployment gate.
The plain-text restoration completion popup and generic error window title are
observed native polish items, not labeled fixed by this increment.

Final verification reads back all six raw suites, 12 strict probes, source
inventories, native exit/source/settings evidence and reviewed-image hashes.
All 2,511 non-Markdown/non-gallery runtime/build assets (including all platform
Go tests, C/headers, templates, manifests and assets) must match the frozen source.
Only documentation/gallery changes are allowed after freezing. No generated
helper Go or unrelated user artifacts are admitted into the SDK source tree.

Reproduction: run `go test -count=1 -p 2 -timeout=20m ./...` and the same command
with `-race`; Windows uses `-p 1`. Give each run an isolated SILK_DATA_DIR. Strict
probes use `scripts/test-bundled-gopls.sh` with a correctly prepared platform-matched
tools staging directory; its -verify intentionally does not install a missing
bundle. Raw scripts/logs, frozen manifests, native profiles and aggregate report
remain in `bin/qa-artifacts/manual-open-20261115/integrated/`.

## Remaining boundaries

Manual `.silkui` decode/recovery, Save, resource loading, macro output and reload
callback I/O remain on the continuing roadmap. This change does not certify
industrial control safety, OS DPI, IME composition, all menus, complete LSP
performance or every slow-device failure mode. Do not treat optional ordinary
suite skips as strict real-gopls passes or old native evidence as a current gate.
