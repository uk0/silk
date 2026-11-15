# Exact incremental lexical completion candidates — 2026-11-15

Baseline: `8f49636c20695b6c682d349765832b75a5d3004c`.
The complete local scanner remains the independent candidate oracle. This is
not an AST/type checker, a reduced candidate window or a longer debounce.

## Establish the current bottleneck

A separate baseline source tree has one reviewed allocation-capture overlay in
`cmd/silkide/perf_capture.go`. It writes the standard Go `allocs` profile after
the existing bounded CPU capture; no network listener or GUI API is introduced.
The patch and source manifest are retained separately, never copied to production.

Repeat the actual Windows session-1 5,000-function body-string typing flow.
Normal process exit is zero; saved source independently parses into 5,002
functions with the exact 161-byte string assignment. Its SHA256 is
`b1d3552d19b33588d258ffbcf9ad71bc0a6a2ab073388d624442a72f8795c204`.
The captured `alloc_space` profile attributes 207.06 MiB, **54.16% of sampled
allocation**, to `scanCompletionItemsForLanguage.func1`. Sampling estimates
382.32 MiB while the process logs 372 MiB cumulative `TotalAlloc`; do not
present sampled attribution as exact byte accounting or retained memory.

## Implementation contract

- Partition the existing lexical identifier semantics into approximately 8 KiB
  chunks, cutting only after a non-identifier rune. Per-chunk deduplication plus
  source-order merging preserves static precedence and first-occurrence order.
- Compute exact byte differences between successive immutable source snapshots;
  re-lex the changed chunks and both neighbours. Neighbours matter for word
  joins and edits that repair invalid UTF-8 across a formerly broken boundary.
- Identifier strings own their bytes, rather than retaining slices into many
  historical document buffers. Reuse only worker-private candidate/dedup scratch;
  ranked deliveries remain independently owned. Clear truncated candidate tails.
- Bound the optional sidecar to 2 MiB input and 32,768 stored chunk words. Beyond
  a budget, use the original **complete** scanner; never truncate identifiers.
  After a word-budget overflow, retry indexing only after substantial shrink,
  avoiding a failed partial index before every complete scan. No source is held
  by that negative cache. Closing clears it as well.
- One existing worker owns mutation. A scan retires its prior candidate cache
  before scratch reuse; canceled merges cannot publish half-built candidates.
  Preserve the existing generation/revision/language/caret/open-intent checks,
  provider merge behavior, 90 ms debounce and two active-worker permits.
- Shrink to the synchronous small-file path, language change and explicit
  `CancelCompletion` close and release the worker/index. Close never waits on the
  UI thread; active work unwinds cooperatively and retires its private state.

## Differential, ownership and lifecycle evidence

Complete-scanner comparisons cover repeated edits, Unicode/invalid-byte seams,
duplicate ordering, language/static changes, budget fallback, cancellation and
storage ownership. Final-source differential fuzz completes **14,495** edit
sequences without a candidate mismatch. An earlier isolated prototype completed
16,088 sequences; these are not added to the final-source count.

Production integration additionally exercises actual editor Undo/Redo,
provider preservation, stale/closed deliveries and lifecycle retirement under
the race detector. Word backing-storage checks and truncated-buffer checks
exercise actual ownership rather than only text equality. The changed-area
GUI/designer race cases also pass with `-tags silk_pure_go`.

Final-source logic-only benchmark on Apple M2 Max, **two alternating body
changes per operation**, in a 5,000-function source:

| Path | ns/op | Allocated B/op | Allocs/op |
|---|---:|---:|---:|
| Complete scanner pair | 3,132,431 | 3,177,850 | 100 |
| Incremental chunk pair | 737,495 | 74,684 | 622 |

Small allocations increase; byte reduction is not a promise about native CPU,
retained memory, FPS or input-to-photon latency.

## Native Windows release comparison

Actual Windows 10 amd64 session 1, no instrumentation source overlay in either
release. Same 5,000-function fixture, warm-settings copy, bundled gopls, owned
PID/HWND foreground checks and English input layout. Send the same 161-byte
string with 125 ms pacing; save and independently parse the resulting file.

| Workload / order | Baseline IDE CPU s | Candidate IDE CPU s | Baseline / candidate cumulative allocation MiB |
|---|---:|---:|---:|
| Body string, baseline first | 11.313 | 10.125 | 371 / 176 |
| Body string, candidate first | 11.344 | 10.500 | 372 / 175 |
| EOF comment, baseline first | 10.578 | 10.031 | 349 / 155 |

IDE CPU is the OS process delta over each typing window (28.55–28.89 seconds),
not the profiler's Windows wait samples. Allocation is cumulative `TotalAlloc`
over the 45-second capture, **not retained heap**. Body CPU improves 10.50% and
7.44%; allocation improves 52.56% / 52.96%. The single comment pair improves
allocation 55.59%, with a small 5.17% CPU difference, not a broad speedup claim.

Child gopls CPU is 23.84 → 22.31 s in the first body pair but 23.17 → 23.98 s
in reverse order. This is not a consistent language-server CPU improvement.
Body SendWait p95 is 37.75 → 43.03 ms / 40.13 → 41.43 ms; it does **not** prove
improved input latency, and native injection time is not input-to-photon time.
Paint wall time is also not FPS. Candidate retained allocation at profile end
is 17,940–22,321 KiB across the typing controls, not a universal memory saving.

All seven owned native runs are responding before close and exit normally with
process code zero. Independent `go/parser` readback proves 5,002 functions,
zero parser errors and the exact 161-byte assignment in all four body outputs;
their saved bytes are identical. Comment-pair complete file hashes match too.
The README image is the final candidate's unedited native window capture.
An additional read-only gallery run navigates to the saved body, confirms the
owned window responds, captures the visible source and connected gopls, exits
normally with status zero and independently verifies unchanged body-source
bytes. It then restores the original fixture and verifies that hash too. This
is a separate visual smoke test, not another measured typing control.

Candidate idle control keeps the fixture unchanged: 0.359 IDE CPU seconds over
41.42 seconds after shown, eight paint calls over the 45-second capture. The
last 15.50 seconds use 0.125 IDE CPU seconds but 4.19 gopls CPU seconds (including
background imports work). No baseline idle comparison, zero-CPU or fixed-FPS
guarantee is claimed. Native macOS keyboard/IME/full-designer work remains open;
this increment adds Windows native acceptance, not an unperformed Mac UI test.

```text
baseline release SHA256  9a0b7df22cb67bce54687f46f58a82c1c7da4e3c487bf3a58d9e662f92ce7dd8
candidate release SHA256 2c477b03183c8e0f545a261087ec9188561e12822268a27989e4403cad73edb8
body saved SHA256        b1d3552d19b33588d258ffbcf9ad71bc0a6a2ab073388d624442a72f8795c204
comment saved SHA256     ec798c639c8abfc910c01d8578e5bdb8ea54020d3d1a1345fcd13e548e8d526b
idle saved SHA256        a04e05532e4cbf5d9340af1b0da5d1bccd4449c007e6c6817543a209f0fa775a
```

## Frozen-source platform gates

Archive SHA256 `f16523791400f4107b6c7da34c233c6c98eeb395a28103bebf6f9e196953ab20`:
2,680 manifest files, 1,964 Go files. All 2,680 pre/post checks on macOS and Linux
match, with no extra Go source. All 1,986 current publication build inputs
(Go/native/module files, loader `.inc`, Windows resource `.rc` source and three
embedded assets) match the tested snapshot. The final index check also accounts
for every frozen source file, not just guessed compiler filename extensions.
Later documentation/image edits are not claimed as part of that frozen archive.
The pre-existing, non-compiled `core/callback.go.txt` sample has CRLF in the
checkout and LF in its Git blob. Its normalized bytes are verified exactly;
the file is not rewritten. No Go, native, module or embedded build input has a
normalization exception.

| Platform | Ordinary named pass / skip | Race named pass / skip | Strict real bundled gopls |
|---|---:|---:|---:|
| macOS arm64 | 8,818 / 29 | 8,818 / 29 | 4 pass |
| Linux arm64/Xvfb | 8,757 / 37 | 8,757 / 37 | 4 pass |
| Windows amd64 | 8,608 / 42 | 8,608 / 42 | 4 pass |

All six ordinary/race runs and all three strict-LSP runners exit zero. macOS
and Linux each have 97 passing packages and 21 without tests; Windows has 98
passing packages and 20 without tests. Windows native build/bundle verification
and final suite runner also exit zero. Its independently retrieved post-run
manifest matches all 2,680 files with exactly 1,964 Go files and no extra Go.
Ordinary-suite skips are not counted as native coverage. All 12 strict gopls
probes pass without skips: unsaved Unicode completion/formatting, renamed-file
definition, type/implementation/back navigation, and negotiated Unicode/CRLF
delta synchronization.

```sh
go test -race -count=1 ./gui ./ged -run 'Test(Completion|PlainTextDoesNot|Outline)'
go test -tags silk_pure_go -race -count=1 ./gui ./ged -run 'Test(Completion|PlainTextDoesNot|Outline)'
go test ./gui -run '^$' -fuzz '^FuzzCompletionLexicalEdits$' -fuzztime=30s -parallel=2
go test ./gui -run '^$' -bench '^BenchmarkCompletionLexicalBodyEdits$' -benchmem
go test -count=1 -p 2 -timeout=20m ./...
go test -race -count=1 -p 2 -timeout=20m ./...
bash scripts/test-bundled-gopls.sh STAGING_DIRECTORY
```

Raw artifacts: `bin/qa-artifacts/completion-index-20261115/` (ignored).
