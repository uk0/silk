# Typed industrial points

`point` complements, rather than replaces, `core.TagDB` and the existing numeric
historian. `point/sqlite.Store` persists definitions plus exact typed latest/history samples;
`TagDB` is the live UI subscription bus. DI/AI/BI are configurable categories,
not inferred types. Supported scalar kinds: `bool`, `int` (int64), `float`
(finite float64), and `string`. Malformed numbers, fractional integers and
out-of-range conversions fail explicitly. String/JSON-to-int accepts exact integer
syntax only (no decimal point or exponent); request explicit float-then-int
steps if that rounding behavior is intended. SQLite storage requires CGO;
conversion/expression/HTTP APIs also work without CGO. GUI imports only the
lightweight point package; SQL/SQLite drivers stay in the optional adapter.

## Transform and compare

```go
pipeline, err := point.ParsePipeline(`[
  {"type":"float"},
  {"expr":"x * 0.1"},
  {"expr":"x >= 80 && x < 100"}
]`)
if err != nil { return err }
value, err := pipeline.Apply("928") // true
```

Expressions allow only `x`, scalar literals, arithmetic `+ - * /`, comparisons,
`&& || !`, and parentheses. No Go execution, function calls, loops, file/network
access, member/index access or allocation. Limits: 1024 bytes / 128 nodes per
expression, 16 transform steps. Division by zero/non-finite results are errors.
Boolean conversion accepts true/false and 0/1 only, never arbitrary truthiness.

## Persistent points and HTTP

Import `pointsql "github.com/uk0/silk/point/sqlite"` and open a Store explicitly, Define each point, then pass the store and your shared
TagDB to NewHTTPSource. Configuration never starts I/O. A source snapshots its
request/mapping configuration, parses each response once and maps up to 128
JSON Pointers. Each field is converted, committed (latest + history transaction),
then delivered to TagDB. Invalid fields retain the preceding value with Bad
quality. Valid sibling fields may update: a response is **not** an atomic PLC
scan, and persistence across different points is not a shared transaction.

```go
store, err := pointsql.Open("points.db")
if err != nil { return err }
defer store.Close() // after the source worker returns
if err := store.Define(ctx, point.Definition{
    Name:"AI101", Kind:point.Float, Category:"AI", Unit:"°C",
}); err != nil { return err }
source, err := point.NewHTTPSource(nil, httpx.Request{
    Method:"POST", URL:configuredURL,
    Header:http.Header{"Content-Type":{"application/json"}},
    Body:[]byte(`{"device":"R-001"}`),
}, []point.Mapping{{Name:"AI101", Pointer:"/data/temp", Kind:point.Float}}, tags, store)
if err != nil { return err }
// Run blocks: execute in an owned worker and cancel/join before closing store.
// true explicitly permits repeated POSTs; never enable for non-idempotent writes.
return source.Run(ctx, time.Second, true, reportError)
```

Do not persist credentials in designs: inject them into request headers at
runtime. `Poll` performs one request and no retries; `Run` polls with fixed delay
and bounded failure backoff (no overlapping requests). Cancellation interrupts
an outstanding request. Store failures return to the caller before that point's
live update; do not hide them behind an error callback that discards everything.

`Tag.Observe` updates acquisition time for unchanged samples without notifying
UI subscribers. It leaves the legacy Publish contract unchanged. Store records
observations including unchanged values. History reads are limited to 10,000
rows and 16 MiB of encoded payloads; invalid timestamp ranges are rejected. Poll failures mark samples Bad;
`hmi.Runtime` supplies a shared TTL/staleness watchdog when configured.

## Designer / dynamic display

ProcessValue properties:

- `tag`: shared point name, wired by `scada.BindScreen`.
- `转换规则 JSON`: ordered conversion pipeline, example above (normally ending
  with a numeric or boolean value for this numeric card).
- `颜色规则 JSON`: first matching condition wins, for example:

```json
[
  {"when":"x>=100","color":"#ef5350"},
  {"when":"x>=80","color":"#ffb300"},
  {"when":"true","color":"#26a269"}
]
```

Optional `text` overrides the number with a state label. Bad/Uncertain samples
and binding errors display grey `—`, preserving the last valid value internally.
The authored numeric preview is persisted separately from live values so save,
load and generated Go never double-apply conversions. Invalid configuration is
rejected and exposed through BindingError/TrySet*; prior accepted config remains.
The ProcessEditor workbench adds staged visual tables; standalone widget
properties retain their JSON representation for designer compatibility.
Only ProcessValue and ProcessEquipment consume full quality-bearing samples in
this increment; older widgets have their original binding behavior.

ProcessEquipment supplies schematic reactor/exchanger/separator/pump/valve/tank/indicator vector symbols,
scales/clips as one widget, and binds DI status via the same tag bus. It is not
a complete certified industrial symbol catalog or an auto-routing piping editor.

## Run / test

`go run ./examples/process2d` launches a **local synthetic** POST source and
SQLite database under SILK_DATA_DIR. Buttons simulate normal/warning/alarm,
HTTP 503, malformed data, and print explicit state snapshots. Three displays
share one source and one point; three equipment symbols share DI11.

`go test -race -count=1 ./point/... ./core ./gui ./ged ./scada ./decl`

Tests use an actual loopback HTTP server (not a stubbed client) to check method,
body/header snapshots, typed mapping, 503/malformed recovery and cancellation.
They establish client behavior, not a real vendor API/PLC interoperability result.
No changes to production endpoints or cloud services are needed.

See [the HMI workbench](../docs/HMI-WORKBENCH.md) for point/source/rule tables,
connected pipes, runtime publishing/restoration, TTL, bounded retention and
dynamic visual properties. Pending: alarm hysteresis/delay/ack, compaction,
obstacle-aware routing, certified symbols and real hardware interoperability.
