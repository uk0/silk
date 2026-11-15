# Negotiated LSP text deltas — 2026-11-15

Baseline: `8f802d6248e69edc676f9010bf3824dbcf635db4`.
Prior Windows captures still show substantial local-completion and full-text
JSON allocations. This increment changes document synchronization, not the
language server's analysis policy or completion delay.

## Contract

- Negotiate only explicit `textDocumentSync: 2` or `{change: 2}` with omitted
  (default UTF-16) or explicit `positionEncoding: "utf-16"`. Decode once at
  initialize time; unknown/malformed capabilities cannot authorize a delta.
- Add SDK `SupportsIncrementalSync`, `DidChangeFrom`, `LSPContentChange` and
  pure `ComputeLSPTextChange`. Existing `DidChange` remains a full replacement;
  custom IDE transports implementing only the old interface still work.
- Keep exactly the last successful wire source/version, not a previous local
  edit. Coalescing, in-flight edits, failed sends and explicit-command flushes
  must preserve Open → Change → Request → Close ordering. Release wire text
  on document retirement, without UI-thread I/O or new idle polling.
- One containing edit handles disjoint changes. Never split UTF-8 runes,
  UTF-16 surrogate pairs or CRLF. Invalid UTF-8, standalone CR and small/mostly
  replaced documents use the compatible full-text fallback. A large unchanged
  source can send an empty range to advance its version cheaply.
- No dynamic registration, UTF-8/UTF-32 language-service coordinate support,
  persistent client-side document database, or longer debounce is claimed.

Protocol references: [official didChange semantics](https://raw.githubusercontent.com/microsoft/language-server-protocol/gh-pages/_specifications/lsp/3.17/textDocument/didChange.md),
[UTF-16 position negotiation](https://raw.githubusercontent.com/microsoft/language-server-protocol/gh-pages/_specifications/lsp/3.17/types/position.md),
[exclusive ranges and line endings](https://raw.githubusercontent.com/microsoft/language-server-protocol/gh-pages/_specifications/lsp/3.17/types/range.md).

## First evidence

The first diff prototype reduced allocation but was slower than full JSON
(523,593 vs 203,324 ns/op). Do not publish that as a CPU optimization. Its CPU
profile attributed about 60% to repeatedly decoding the unchanged prefix into
UTF-16 positions. Use ordinary safe chunk comparisons and count complete lines
before decoding only the final line; preserve the same differential checks.

Final focused serialization benchmark, Apple M2 Max, 5,000 generated functions
and one EOF-character change (not native paint, FPS or input latency):

| Path | ns/op | Allocated B/op | Allocs/op | Serialized update bytes |
|---|---:|---:|---:|---:|
| Full | 255,163 | 189,966 | 3 | 185,114 |
| Incremental | 130,426 | 280 | 4 | 169 |

Changing the request representation does not imply that gopls analysis CPU
falls by the same factor. Native process/child CPU must be measured separately.

- Differential tests apply edits through the separate strict `ide/lspedit`
  implementation and compare exact text; cover Unicode shared byte boundaries,
  CRLF edits, disjoint changes, malformed encodings and full/no-op fallbacks.
- Capability/wire-frame tests verify ranged versus legacy full notifications;
  tests distinguish absent JSON fields from fields retained in a reused decoder.
- IDE tests cover last-successful baseline, failed sends, concurrent local edits,
  close retirement and the old full-only transport interface.
- Real bundled gopls checks include unsaved Unicode definitions/symbols after
  coalesced CRLF edits, LF normalization, version-only change, empty replacement
  and restoration, with independent unchanged-disk readback.

Raw scripts/results: `bin/qa-artifacts/lsp-incremental-20261115/` (ignored local
QA outputs, not shipped product source). Final platform/native outcomes are
recorded only after terminal exits and readbacks are independently inspected.

## Frozen source and verified native release replay

Final source archive SHA-256:
`57f4c28b8893f3e472b6630d4f363d16edf203943b06495554d96edda590a73e`.
The 2,672-file manifest is checked before/after platform tests; the publication
index currently matches all 1,978 Go/native/module/embedded build inputs.

Windows 10 Session 1, the same 5,000-function fixture, verified bundled gopls,
DLL/tool bundle, APPDATA seed, 161 native characters at 125 ms intervals,
45-second capture and uninstrumented release flags:

| Workload / measurement | Baseline `8f802d6` | Candidate |
|---|---:|---:|
| EOF comment: typing interval, s | 29.09 | 29.22 |
| EOF comment: IDE CPU, s | 11.67 | 11.44 |
| EOF comment: gopls CPU, s | 25.39 | 24.50 |
| EOF comment: cumulative Go allocation, MiB | 462 | 349 |
| Function-body string: typing interval, s | 28.89 | 29.14 |
| Function-body string: IDE CPU, s | 19.13 | 17.84 |
| Function-body string: gopls CPU, s | 25.25 | 23.48 |
| Function-body string: cumulative Go allocation, MiB | 1,367 | 1,175 |

Allocation falls about 24% / 14% in these two matched controls. CPU differences
from a single pair do not establish a general or statistically reliable speedup.
Do not conflate cumulative allocation with retained heap, native wait samples
with OS CPU, paint wall time with FPS, or SendWait acknowledgement with
key-to-photon latency. Local completion and ordinary-body AST work remain.

All four processes are responsive and exit 0 via their exact owned windows.
Independently saved file hashes match within each pair:

- Comment: `ec798c639c8abfc910c01d8578e5bdb8ea54020d3d1a1345fcd13e548e8d526b`.
- Body: `b1d3552d19b33588d258ffbcf9ad71bc0a6a2ab073388d624442a72f8795c204`.
  A separate standard-Go AST reader checks 5,002 functions, the single target
  assignment and its exact decoded 161-byte value after Save formatting.
- Baseline executable: `51acb4d5e93b45915e0a12fe9828df1674e20b20fd89a1942a0129c8cb65708b`.
- Candidate executable: `63787b02c30498eb8fe0adeecaf3439d92912cf84d6e478a953004a37ae41981`.

## Final platform results

| Platform / full mode | Named passes | Named skips | Failures |
|---|---:|---:|---:|
| macOS arm64 ordinary | 8,786 | 29 | 0 |
| macOS arm64 race | 8,786 | 29 | 0 |
| Linux arm64/Xvfb ordinary | 8,725 | 37 | 0 |
| Linux arm64/Xvfb race | 8,725 | 37 | 0 |
| Windows amd64 ordinary | 8,576 | 42 | 0 |
| Windows amd64 race | 8,576 | 42 | 0 |

These are named tests/subtests/fuzz seeds, not separate user workflows. Skips
are not passes. All three platforms also pass four strict real bundled-gopls
probes: 12 passes, no skips. Changed-area pure-Go/race and no-CGO core tests pass. Independent no-CGO
SDK consumers build and execute on macOS arm64, Linux arm64 and Windows amd64;
all return the same UTF-16 range and round-trip exact Unicode replacement.
Final algorithm differential fuzzing completes 201,721 mutations without a
mismatch. Windows ordinary/race/strict gates all exit 0, independently of the native
replay gate. Linux pre/post checks verify all 2,672 manifest files; Windows
also verifies the same manifest before and after its full suites.

## Reproduction

```sh
go test -race -count=1 ./core ./cmd/silkide -run 'TestLSPTextChange|TestLSPIncrementalSyncCapability|TestLSPDocument'
go test ./core -run '^$' -fuzz '^FuzzLSPTextChangeRoundTrip$' -fuzztime=30s -parallel=2
go test ./core -run '^$' -bench '^BenchmarkLSPTextChangeJSON$' -benchmem
# A matching-platform verified tools bundle must already exist in staging:
bash scripts/test-bundled-gopls.sh STAGING_DIRECTORY
```

The strict script now requires four real gopls probes; missing tools are a
failure in that mode, not a passing skip. This increment is not full Qt Creator
parity, a complete incremental Go parser, or completion of the broader product.
