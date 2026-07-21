# examples/hmi — end-to-end 组态 / HMI on `scada.Services`

A minimal, runnable HMI that exercises the **entire** Silk SCADA runtime loop —
from a simulated field device all the way to live widgets, alarms, a trend chart
and a statistics table — with **no hand-written per-widget binding code**. It
mirrors what the `ged` code generator now emits: build a form of tag-named
widgets and backend-free panels, then hand the whole tree to `scada.BindScreen`.

## What it proves

```
sim.Sim (sine waveforms)
  -> driver.Poller               reads every 500ms
      -> services.Tags           core.TagDB — the one real-time point registry
          ├─ AlarmDB.Watch       -> alarm bridge -> event log + notify + AlarmPanel
          ├─ Historian.Record    -> SQLite sample store (TrendPanel range replay)
          ├─ Stats.Collector     -> StatsPanel (count / min / max / avg / last)
          └─ BindScreen tag bind:
                 Tank.SetTagName("level")           -> BindTagAnimated -> fill eases
                 DigitalDisplay.SetTagName("temp")  -> BindTagAnimated -> readout eases
              TrendPanel (TrendTag "temp")          -> owned LineChart scrolls live
```

The whole chain — `sim` → `Poller` → `services.Tags` → `BindScreen` →
widgets / `AlarmPanel` / `TrendPanel` / `StatsPanel` + alarms + historian — runs
from ~40 lines of `main`, and `main.go` contains **zero** `gui.BindTag` calls of
its own.

## The four wiring calls

| Call | What it installs |
| --- | --- |
| `scada.New(cfg)` | The runtime container: one `TagDB`, `AlarmDB`, historian, event log, recipe book and stats collector. `cfg.TagMeta` seeds each tag's engineering range **and** its alarm limits (`Hi` / `HiHi`). |
| `scada.BindScreen(services, form, opt)` | Walks the form once and auto-binds: every `TagName()` widget to its tag, the `AlarmPanel` to the live `AlarmDB`, the `TrendPanel` to `TrendTag`'s live + historical feed, the `StatsPanel` to `StatsTags`. |
| `services.Start()` | Turns the runtime on: alarm bridge (alarm → event + notification), a per-tag limit watch, historian recording and stat folding. |
| `driver.NewPoller(...).Start()` | Reads the two `sim` waveform points into `services.Tags` every 500ms — the stand-in for a Modbus / S7 field device. |

## Tags

| Tag | Range | Alarm limits | Sim waveform |
| --- | --- | --- | --- |
| `level` | 0–100 % | `Hi 80`, `HiHi 95` | `sine:min=0,max=100,period=20s` |
| `temp` | 0–120 ℃ | `Hi 90` | `sine:min=20,max=110,period=30s` |

Both `TagPoint`s set `Type: driver.TypeFloat64`. This is required: the zero-value
`DataType` is `TypeBool`, which would coerce the sine floats to booleans.

## Screen

- **Tank** — bound to `level` by its design-time tag name.
- **DigitalDisplay** (7-segment) — bound to `temp`; segments recolor at/above `Hi`.
  (`Gauge` is not used for the tag binding because it has no `SetTagName` seam —
  only the eight industrial widgets do; `BindScreen` binds through that seam.)
- **AlarmPanel** — live view of the `AlarmDB`; `level` trips `Hi`/`HiHi` and
  `temp` trips `Hi` each cycle, so rows appear and clear on their own. Click a
  row's `ACK` to acknowledge.
- **TrendPanel** — its owned `LineChart` scrolls `temp` live; the range buttons
  replay `temp`'s persisted history from the historian.
- **StatsPanel** — running count / min / max / avg / last for `level` and `temp`;
  click `清零` to reset a tag.

## Run

```sh
# needs a desktop session — Cairo + GLFW, cannot run headless
go run ./examples/hmi

# or build a binary
CGO_CFLAGS="$(pkg-config --cflags cairo)" go build -o hmi ./examples/hmi/
```

The historian, event-log and recipe stores are created under
`os.UserConfigDir()/silk-hmi` (falling back to the OS temp dir).

Watch it: the temperature readout eases toward each new sample, the trend line
scrolls, the statistics table folds every poll, and alarm rows raise and clear as
`level` and `temp` cross their limits — all driven by the simulator through the
shared `Services`, with the screen wired by a single `BindScreen` call.
