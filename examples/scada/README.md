# Silk SCADA / 组态 HMI — end-to-end demo

A minimal but complete water-tank HMI that proves the whole Silk 组态 stack runs
end to end: a real-time **tag** drives an **industrial widget**, which **eases
via animation**, while a **limit alarm** recolors the screen and a **rolling
trend** scrolls live.

```
examples/scada/main.go   — the demo (package main)
go run ./examples/scada   — build + run
```

## What it demonstrates

The full loop, off a background driver goroutine and back onto the UI thread:

```
sim goroutine (every 500 ms)
  └─ core.Tag.SetValue(x)                     goroutine-safe real-time point
       ├─ core.Tag fan-out ───────────────────► gui bridge (marshals via gui.Post)
       │     ├─ BindTagAnimated  → Tank.SetLevel / Gauge.SetValue   (eased)
       │     ├─ BindTag          → DigitalDisplay.SetValue / Valve.SetState
       │     ├─ ThresholdColorBinding → Tank.SetColor  (blue → red in HiHi)
       │     └─ Tag.Subscribe    → LineChart.AddSample (live trend, sample time)
       └─ core.AlarmDB.Watch(tag)              limit engine (LoLo/Lo/Hi/HiHi)
             └─ AlarmDB.Subscribe → gui.Post → Indicator turns red + alarm banner
```

Run it and you see:

- **Tank** fills / drains **smoothly** (level eases to each new setpoint, it does
  not jump), and its liquid turns **red** once the level enters the HiHi band.
- **Gauge** needle sweeps to the new temperature; the **7-segment display** shows
  the same value and goes red at/above the Hi limit.
- **Trend chart** scrolls left in real time over a 30 s window.
- **Pump lamp** is green while the feed pump runs and flips **red** the instant
  the level trips HiHi; the **alarm banner** lists every active alarm.
- **Feed valve** opens/closes with the pump tag.

## The tags

| Tag     | Type   | Range / Unit | Alarm limits              |
|---------|--------|--------------|---------------------------|
| `level` | float  | 0–100 %      | LoLo 5, Lo 20, Hi 80, HiHi 95 |
| `temp`  | float  | 0–120 ℃      | Hi 90                     |
| `pump`  | bool   | —            | (drives lamp + valve)     |

A background goroutine feeds new simulated values every 500 ms (a slow sine so
the level breathes through every alarm band; the pump trips off at HiHi). It only
calls `SetValue` — every widget/animation mutation is marshaled onto the UI
thread by the tag bridge / `gui.Post`, so nothing touches widget state off-thread.

## Silk APIs exercised

- **`core/tag.go`** — `NewTagDB()`, `db.GetOrCreate(name, core.Meta{...})`,
  `tag.SetValue(x)`, `tag.Subscribe(func(core.Value))`, `tag.Value()`.
- **`core/alarm.go`** — `NewAlarmDB()`, `db.Watch(tag)`, `db.Subscribe(...)`,
  `db.Active()`, `core.HighHigh`.
- **`gui/tagbridge.go` / `gui/tagbinding.go`** — `BindTagAnimated(tag, setFloat, dur)`,
  `BindTag(tag, setter)`, `WrapTag`, `ThresholdColorBinding(tag, ranges, setColor)`,
  `TagFloat` / `TagBool`.
- **`gui/industrial.go`** — `Tank.SetLevel` (0–1 fraction) / `Tank.SetColor`,
  `Indicator.SetOn` / `Indicator.SetColor`, `DigitalDisplay.SetValue` /
  `SetUnit` / `SetLimits`, `Valve.SetState`.
- **`gui/gauge.go`** — `Gauge.SetValue` / `SetRange` / `SetUnit` / `AddZone`.
- **`gui/chart_line.go`** — `LineChart.EnableRolling` / `AddSample` / `SetTimeWindow`.
- **`gui/frame.go` / `gui/uiqueue.go`** — `Form.AttachWindow(WtForm)` + `Window()` +
  `Show()`, `gui.Post`, `core.EventLoop()` (drains `gui.Post` and ticks the
  animation engine each frame).

## How to run

```sh
go run ./examples/scada
# or build a binary:
CGO_CFLAGS="$(pkg-config --cflags cairo)" go build -o scada ./examples/scada/ && ./scada
```

Requires the same toolchain as the rest of Silk: Go 1.21+, CGO enabled, the Cairo
system library (`pkg-config cairo`), and a display (it opens a GLFW window).

### Note on `-race`

The demo is written race-safe by construction: the sim goroutine only calls the
mutex-guarded `core.Tag` / `core.AlarmDB`, and every widget mutation runs inside a
`gui.Post` closure on the UI thread. `go build -race` is *not* a useful smoke here
because the framework's GLFW + Cairo cgo paint layer (`gui/window_glfw.go`) trips
the race detector / `checkptr` on its own — the shipped `examples/dashboard` fails
the same way. Smoke it like the other GLFW examples: plain `go build` + launch.
