# Exact industrial integers and status words — 2026-11-17

This is a converged development-source increment after `c979cb3`, not completion
of the continuing Qt Creator/industrial product goal or a binary release.

## Preserved failures and implementation

The previous evaluator compared integral JSON tokens via float64:
`9007199254740993 == 9007199254740992` incorrectly returned true. Typed int64
comparisons above 2^53 instead failed as a lossy float conversion. The numeric
card rejected a valid MaxInt64 sample and displayed a grey dash.

The negative run is retained at
`bin/qa-artifacts/industrial-integer-20261117/negative.log` (exit 1).

- Integer literals are compiled/cached as signed int64, including MinInt64,
  parentheses, Go bases and underscores. Integer `+ - *` are overflow-checked.
  Comparisons preserve integer identity even against represented float boundaries.
- Add integer `% & | ^ &^ << >>` and unary `^`. They require integer operands;
  shifts are bounded to 0..63 and left-shift overflow fails. `/` remains floating
  point; mixed arithmetic retains strict integer-to-float conversion checks.
- Integer JSON-to-float casts no longer silently bypass the precision guard.
  Explicit string formatting preserves the original finite JSON number token.
- `ProcessValue` formats large integer digits exactly, evaluates condition colors
  and alarm thresholds without rounding, and preserves nearby integer range-bar
  differences. `IntegerValue()` is the exact current-value API; legacy `Value()`
  is deliberately only an approximate visual/compatibility projection.
- Existing staged mapping/conversion/color/property tables accept the operators
  and retain a wrapped help message after validation errors. Configuration and
  authored previews serialize; acquired samples do not.
- `examples/processstatus` is a runnable local mock workbench. Run is explicit;
  normal/overflow/recover request modes demonstrate counter conversion, bit
  extraction, independent field quality and recovery. This is not PLC control.

## Verification scope

- Rational-oracle comparisons cover float boundary cross-products and 5,000
  seeded finite random float/int pairs, including reverse order. BigInt-oracle
  arithmetic covers boundary cross-products and 1,000 seeded integer pairs.
- Dedicated controls cover range/overflow, malformed inputs, noninteger bit ops,
  short-circuit evaluation, immutable concurrent evaluation and exact JSON casts.
- A race-enabled actual loopback POST test checks exact body/header and JSON
  mapping, one response feeding multiple controls, counter digits/colors, bit
  extraction, overflow isolation, recovery, independent SQLite reads/history,
  unchanged authored JSON and no duplicate per-widget requests.
- Designer drop/save/load/preview and generated-Go compilation retain integer
  configuration while refusing to serialize acquired MaxInt64 state. The new
  example round-trips and generates an inert workbench document.
- Changed-package race, no-CGO point/HMI, and GUI production-dependency isolation
  from SQL pass. SQL is imported only by the new GUI integration test, not the
  GUI production package.

One integration-harness failure (`hmi: unknown point BI_COUNTER`) was corrected
by creating the declared tags before binding; the binding contract was not
weakened. The native helper's missing IsRunning method was a QA-helper compile
error, fixed outside SDK source. Both failed logs are retained.

## Final frozen source and platform gates

Archive SHA-256:
`0503873df49761504a0b391fbaecda95160ec82a4170c8abd5bba9f4fd5a8606`.
Manifest: 2,872 paths, including 2,098 Go and 2,104 total Go/module paths.
Full suites use isolated source trees; no generated main enters the SDK root.

| Platform | Full named passes / skips | Race named passes / skips | Strict bundled gopls |
| --- | ---: | ---: | ---: |
| macOS arm64 | 9,682 / 29 | 9,682 / 29 | 4 pass, 0 skip |
| Linux arm64 + Xvfb | 9,621 / 37 | 9,621 / 37 | 4 pass, 0 skip |
| Windows 10 amd64 | 9,479 / 43 | 9,479 / 43 | 4 pass, 0 skip |

All three full/race suites and strict actual correct-architecture gopls exit 0. Skips
remain explicit. Linux Xvfb is not physical Linux/hardware-GPU certification.
Default full vet retains 501 baseline diagnostics, with no added or removed
messages after mapping shifted diagnostics to unchanged source lines.
The renderer subset passes all 175 named cases, with no skips; scoped vet passes.
Independent post-test source exports on Linux and Windows match all 2,872
manifest paths and contain no extra Go files.

## Native boundary

macOS preflight window/PID was live and its UI-thread snapshot proved inert
(calls=0, no runtime). However ScreenCaptureKit returned -3811, then -3812 after
Raise and after resetting/rebinding the same process. No blind coordinate input,
app restart, personal preference or system-permission change was attempted.
The inert owned preflight was retired with SIGTERM, not counted as normal exit
or a native UI pass. macOS full/race/render tests do not substitute for that gap.

Windows 10 final-source race-built native workbench passes explicit Run, overflow,
recovery, Stop, Save, Generate Go and normal Alt+F4 exit 0. Before Run there are
no requests or runtime. Original captures show `9007199254740993.0 pcs` in red,
status nibble `10.0` in blue and the DI pump on; overflow greys the counter while
valid siblings continue. Recovery shows adjacent `9007199254740992.0 pcs` in
green. The loopback QA endpoint only changes fixture modes and observes state
through `gui.Post`; it does not invoke those native toolbar actions.

Native runtime point snapshots retain mapped integer `9007199254740995`, bad
quality with that last-good raw value, and recovered `9007199254740994`. The
observer's derived node-pipeline expectation is not an actual widget getter;
the original screenshots are the rendered-value/color evidence. After Stop the
request count remains 143. Independent read-only SQLite inspection after normal
close confirms all 429 history rows, bad/good counter transitions and exact latest
values. Authored JSON is identical across all modes and the actual saved file;
it contains initial zero and expressions, not acquired samples. The actual
native-generated Go file passes a separate macOS race-enabled consumer test for
embedded-document identity, exact pipelines and inert runtime construction.
That test does not run the generated main's native UI or its acquisition.
The byte-identical actual export also builds with race instrumentation on
Windows. Post-native readback rechecks all 2,872 source paths without mismatches
or extra Go, and confirms no owned native process remains.

The first Windows action task lacked the uploaded common helper and failed
before any input; calls remained zero. Upload/hash verification fixed the harness
without restarting the same app. A generated-consumer QA command selected the
older local Go launcher and another assumed nonexistent Runtime APIs; both failed
attempts were retained and corrected outside SDK source before its passing test.

## Replay and publication boundary

```sh
go test -race -count=1 ./point/... ./hmi ./hmiapp ./gui ./ged ./examples/processstatus
CGO_ENABLED=0 go test -count=1 ./point ./hmi
go run ./examples/processstatus
```

Final platform/export/native evidence lives in
`bin/qa-artifacts/industrial-integer-final1-20261117/`; preflight/negative evidence
is separate under `bin/qa-artifacts/industrial-integer-20261117/`.
Exact source/index/media verification is required before commit/push. Raw tests,
binaries, databases, mocks and profiles stay out of Git.

This increment does not add multi-point expressions, arbitrary Go execution,
alarm certification, PLC command protocols, database type migration or solve
all popup/DPI/IME/large-workspace requirements. User-requested convergence freezes
this implementation rather than expanding those separate tasks before publication.
