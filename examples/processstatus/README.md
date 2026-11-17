# Exact BI counters and status words

```sh
go run ./examples/processstatus
```

The mock listener binds loopback only. The workbench does not acquire or open
its point store until **Run**. No PLC/hardware commands are sent.

- `BI_COUNTER` is an exact signed integer. HTTP mapping adds 2, the display
  conversion subtracts 2, and `x > 9007199254740992` selects red/green without
  silently rounding adjacent counters together.
- `BI_WORD` extracts the upper nibble with `(x >> 4) & 0xf`.
- `DI_RUN` maps the status word's bit 0 using `(x & 1) != 0`.
- Stop, edit **HTTP sources → Request body**, change `mode` from `normal` to
  `overflow` or `recover`, Apply, then Run again. Overflow marks only the counter
  bad/grey, preserving its last stored integer; valid sibling fields keep working.
- **Live points → History** explicitly reads observations. Save/Load and Generate
  Go retain the pipeline text, not acquired samples. The generated application
  references this temporary mock URL: leave the mock running, or configure your
  own endpoint explicitly. Do not put generated `main` files in the SDK root.

`+ - *` are checked int64 arithmetic when both operands are integers. Remainder
and bit operations require integers; cast strings/floats explicitly first.
Shifts are limited to 0..63 and left-shift overflow fails rather than wrapping.
`/` keeps floating-point semantics. The SDK's strict integer-to-float conversion
limit still applies to mixed arithmetic. Comparisons do not make that lossy cast.
Integer literals support Go decimal/hex/binary/octal forms and underscores.

`ProcessValue.IntegerValue()` returns an exact current good-quality integer.
`Value()` is a legacy approximate float projection for visual geometry; never
use it to reconstruct or persist a large counter. Display precision continues
to apply, so the default workbench card shows a `.0` suffix; standalone SDK
hosts can use `SetPrecision(0)` for integer-only text. Integer digits, condition
colors and alarm comparisons remain exact.

Display colors are not an alarm lifecycle, safety interlock or certified SCADA.
