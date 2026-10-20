# Satin HMI dashboard

```sh
go run ./examples/satin-dashboard
```

Requires the normal platform GUI/Cairo dependencies. No PLC, printer, network
driver, simulator clock or idle animation is started. The four buttons toggle a
local valve Tag, set the pressure to 42.5 / 95, and switch the light/dark theme.

- `SpriteView`: theme-neutral disabled symbol or full-color active satin sprite;
  selectable `IconName` / `ActiveIconName`, with boolean `TagName` binding.
- `ProcessValue`: title, value, unit, precision, bounded range indicator and
  `AlarmAbove` visual threshold, with numeric `TagName` binding.
- `Gauge`: now exposes the same numeric `TagName` seam.

The visual threshold is not a lossless alarm recorder or a PLC safety interlock.
`scada.BindScreen` owns the binding lifecycle. High-frequency visual samples are
coalesced to the latest value; unbinding cancels queued updates and animation.

Both new widgets are available in the designer's SCADA palette and support
drag/drop, properties, save/load, preview and generated Go.
