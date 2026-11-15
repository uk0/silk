# Exact function-body symbol projections — 2026-11-15

Baseline: `d11ffd5a57de8b531ce6aa120797f8e4bbf8dfe9`.
The prior native function-body control still spends substantial CPU/allocations
in whole-source outline parsing. Extend the exact symbol projection, not the
language grammar or compiler/type checker.

## Contract and implementation

- Record body interiors from the same scanner that locates bodies for tolerant
  AST outline recovery. Keep outer braces outside each editable span.
- Re-lex a candidate body using Go's scanner. It must close exactly at the outer
  delimiter, have no lexical errors and preserve physical line breaks. Strings,
  rune/raw literals, comments, nested literals and closures are tokens, not
  brace substring heuristics. Everything unproved keeps the complete AST path.
- Metadata is immutable once delivered. Rebase empty bodies in place; discard
  old internal comment positions after body edits rather than reuse stale token
  offsets. Private-worker state never shares mutable spans with the UI.
- Bound UI lexical work to 8 KiB and span copying to 16,384 entries (256 KiB on
  64-bit). Large files beyond these budgets use their existing bounded worker;
  small-file/explicit-command fallback remains synchronous as before. Equal-size
  edits and EOF comments share unchanged immutable ranges instead of copying
  every function's metadata. No idle worker/timer or longer debounce is added.
- Synchronous `ParseSymbols` and small-file queries publish the same metadata
  as the background path. Undo/Redo, provider/macro invalidation, language
  changes and close retain exact source/version and ownership boundaries.
- Unchanged body symbols stay ready without an indexing flash or tree rebuild;
  physical newline/signature/boundary edits cannot draw or navigate stale rows.

## Focused evidence before the release freeze

- GUI/designer ordinary logic and race regression pass. Tests cover synthetic
  package offsets, CRLF, empty-body growth, cross-edit comment openers, lexical
  boundaries, nested/generic code, span budgets and immutable sharing.
- SDK/editor Undo/Redo, provider/language retirement, active-worker close and
  actual outline drawing/navigation are exercised rather than only a parser
  helper. Macro/provider output disables base-only fast-path assumptions.
- One-edit AST differential fuzzing: 38,864 mutations without a mismatch.
- Repeated-edit private/UI differential fuzzing: 164,063 sequences without a
  mismatch. The separate complete parser is the field-by-field oracle.

Logic-only benchmark on Apple M2 Max, 5,000 uniquely named functions, TWO
alternating body changes per operation (not native paint/FPS/input latency):

| Path | ns/op (two changes) | Allocated B/op | Allocs/op |
|---|---:|---:|---:|
| Full AST symbols | 18,343,706 | 10,090,139 | 280,171 |
| Private worker proof | 483,262 | 536 | 16 |
| Immutable UI proof | 541,306 | 164,504 | 20 |

A faster helper is not evidence of a faster whole application. Native release,
platform/race and real-gopls results are reported only after their actual runner
exits, source identity and saved-file readbacks have been verified.

## Native Windows release replay

Windows 10 amd64, interactive desktop session 1; normal release flags, no
observability source overlay. Same 5,000-function fixture, copied warm settings,
bundled tools, foreground HWND checks, English input layout and 161 characters
sent with the same 125 ms pacing. Native input delivery is not input-to-photon
latency. Each process receives its own data directory and closes normally.

| Workload / run order | Baseline IDE CPU s | Candidate IDE CPU s | Baseline / candidate cumulative allocation MiB |
|---|---:|---:|---:|
| Body string, baseline first | 16.266 | 11.266 | 1,177 / 372 |
| Body string, candidate first | 17.172 | 10.828 | 1,177 / 372 |
| EOF comment, baseline first | 9.766 | 10.953 | 350 / 349 |

CPU is the OS-owned IDE process CPU delta between `typing-begin` and
`typing-end` (body windows 28.51–28.79 s). Allocation is Go's cumulative
`TotalAlloc` over the 45-second profile, **not retained heap**. Body IDE CPU
decreases 30.74% / 36.94%, and allocation decreases 68.39% in both orders.
Do not claim a comment-only CPU improvement: that control is 12% higher, with
essentially unchanged allocation. Body child-gopls CPU is 22.27 → 23.31 s and
22.13 → 23.41 s; language-server work is not optimized by this change.

Seven native controls have their own zero exit marker and a responding owned
process before normal close; all normal process exit codes are zero. The four
body outputs are independently parsed with `go/parser`: 5,002 functions, no
parser error and the exact 161-byte string assignment. Their complete saved
bytes are identical. The comment pair also has matching complete file hashes.
Screenshots show the candidate outline, device-resolution overview and connected
bundled gopls; the README gallery uses the final body-run capture.

Separate candidate idle control: unchanged source, 0.625 IDE CPU seconds over
41.29 seconds after shown, eight paint calls over the 45-second profile. The
last 15.42 seconds cost 0.250 IDE CPU seconds; gopls still uses 3.94 CPU seconds
there (including its background imports refresh). This is not a zero-CPU or
fixed-FPS guarantee. No baseline idle comparison is claimed.

```text
baseline release SHA256  63787b02c30498eb8fe0adeecaf3439d92912cf84d6e478a953004a37ae41981
candidate release SHA256 9a0b7df22cb67bce54687f46f58a82c1c7da4e3c487bf3a58d9e662f92ce7dd8
body saved SHA256        b1d3552d19b33588d258ffbcf9ad71bc0a6a2ab073388d624442a72f8795c204
comment saved SHA256     ec798c639c8abfc910c01d8578e5bdb8ea54020d3d1a1345fcd13e548e8d526b
idle saved SHA256        a04e05532e4cbf5d9340af1b0da5d1bccd4449c007e6c6817543a209f0fa775a
```

## Native macOS SDK consumer

Build an external SDK module against the same frozen source. Actual native
mouse clicks perform body Edit, Undo and Save. Callback snapshots remain ready
with 5,002 symbols and content identity 1 throughout; text versions advance
1 → 2 → 3. Independent saved-text readback restores the original `"a"` body;
the changed body and restored source have distinct recorded hashes. The app
returns from its main loop and prints `NORMAL_EXIT`, process status zero.

The post-edit Retina screenshot visibly shows `"native body"` and ready state.
ScreenCaptureKit later returns error -3812 during observation, although Undo
and Save are independently proven by the log and file. Do not infer additional
clicks succeeded from that failure. Redo/newline/macro branches are automated
regressions only, not native mouse acceptance here. This is not full-IDE,
keyboard/IME or full designer acceptance.

## Frozen source and platform gates

Archive SHA256 `47c7207281da65eeab384c61fbe742aa26178ef914911f36cb7bc0080dea9ccf`,
2,676 manifest files / 1,961 Go files. All three platforms' source checks match
every file; independent post-run reads contain no extra Go source. The 1,981
publication build inputs (Go/native/module files and three embedded assets)
match the tested snapshot byte-for-byte. Later documentation/image edits are
separately reviewed, not claimed to be part of the frozen archive.

| Platform | Ordinary named pass / skip | Race named pass / skip | Strict bundled real gopls |
|---|---:|---:|---:|
| macOS arm64 | 8,804 / 29 | 8,804 / 29 | 4 pass |
| Linux arm64, Xvfb | 8,743 / 37 | 8,743 / 37 | 4 pass |
| Windows amd64 | 8,594 / 42 | 8,594 / 42 | 4 pass |

All six full/race runs and all three strict-LSP runs exit zero. Windows native
build, bundle verification and final suite runner also exit zero. Strict probes
exercise unsaved Unicode completion/formatting, renamed-buffer definition,
type/implementation/back navigation and negotiated Unicode/CRLF deltas; none
may skip. Ordinary-suite skip counts remain explicit: they are not native
platform coverage. macOS/Linux each have 97 passing packages and 21 packages
without tests; Windows has 98 passing packages and 20 without tests.

## Replay

```sh
go test -race -count=1 ./gui ./ged
go test ./gui -run '^$' -fuzz '^FuzzGoSymbolIndexEditsMatchFullParser$' -fuzztime=30s -parallel=2
go test ./gui -run '^$' -fuzz '^FuzzGoBodyProjectionEditSequence$' -fuzztime=30s -parallel=2
go test ./gui -run '^$' -bench '^BenchmarkGoBodyProjection$' -benchmem
go test -count=1 -p 2 -timeout=20m ./...
go test -race -count=1 -p 2 -timeout=20m ./...
bash scripts/test-bundled-gopls.sh STAGING_DIRECTORY
```

Local raw artifacts/scripts: `bin/qa-artifacts/body-projections-20261115/`
(ignored outputs, not published source). Broader completion-index/server CPU,
macOS keyboard/IME, designer/runtime and Qt Creator parity work remain open.
