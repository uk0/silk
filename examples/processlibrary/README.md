# Reusable industrial equipment

```sh
CGO_ENABLED=0 go run ./examples/processlibrary
# Or choose a fresh directory:
go run ./examples/processlibrary -out /path/to/new-demo
```

No GUI, database or driver is started. The program writes a `.silkhmi` project
with **13 nodes, 5 pipes and 8 independent points**, plus a `.silkhmilibrary`
containing the three built-in compositions and a captured pump subgraph.
It refuses to replace either existing output. The two files are published
independently, not as a multi-file transaction; a later failure can leave the
first successfully written file in place.

The CLI closes its framework run marker/resources on both success and failure,
including argument errors and help. Exit codes are `0` for success/help, `1` for
operation failure and `2` for invalid arguments. A failed no-overwrite run does
not make the next legitimate invocation look like an abnormal prior shutdown.

Open `plant.silkhmi` using `go run ./cmd/silkhmi -project <file>`, or open it in
SilkIDE and use **Tools → Edit 2D Process...**. **Device library** previews and
inserts equipment; **Open library** explicitly loads the companion catalog.

## SDK

```go
library := hmi.BuiltinDeviceLibrary()
candidate, instance, err := hmi.PlaceDeviceTemplate(project, library.Devices[0],
    hmi.DevicePlacement{ID: "pump1", X: 20, Y: 20})
// On success, candidate is independent of project and library.
// instance.Nodes is ready for multi-selection; instance.Points reports remaps.
```

- Default point names are `pump1.RUN`, `pump1.LEVEL`, `pump1.PRESSURE`.
  `PointPrefix: "Line1."` uses that exact prefix instead.
- `Points: map[string]string{"RUN": "DI11"}` explicitly shares an existing
  compatible point. Type, category and unit must match. Host initial values,
  stale timeout and acquisition ownership are unchanged. Unknown mappings,
  aliases, unintended collisions and out-of-bounds placements fail atomically.
- `hmi.CaptureDeviceTemplate(project, instance.Nodes, "pump", "Pump skid")`
  copies nodes, internal pipes/bends and referenced point definitions. It does
  **not** copy HTTP sources, retention policy, live values or database history.
  Captured IDs/point names are preserved; use explicit mappings to shorten
  names when placing an already-prefixed captured device.
- Placement rewrites node, pipe and property bindings together. It creates
  ordinary flat nodes, not a persistent prefab with linked future updates.
  Moving them together afterwards uses the ordinary multi-selection tools.

## Catalog I/O

`LoadDeviceLibrary(ctx, path)` and `SaveDeviceLibrary(ctx, path, library,
overwrite)` are synchronous worker APIs; never run them on the UI thread.
Catalog JSON is versioned, strict, limited to 64 templates / 4 MiB, and never
executes sources. Non-regular paths and symlinks are rejected at preflight.

Custom GUI hosts can call `gui.OpenFileDialogForExtension("silkhmilibrary")` /
`gui.SaveFileDialogForExtension("silkhmilibrary")` on the UI thread, then pass
the selected path to a worker. The legacy `OpenFileDialog` / `SaveFileDialog`
retain form-specific `.silkui` defaults; they are not generic asset choosers.
Extension-specific APIs reject non-alphanumeric / over-32-character extensions
before opening a native chooser. OS chooser cancellation remains platform-owned.

Saving uses a same-directory temporary file. With `overwrite=false`, an atomic
hard-link publication refuses competing writers; the destination filesystem
must support hard links (e.g. APFS, NTFS, ext4). Unsupported filesystems return
an error, not an unsafe overwrite fallback. `overwrite=true` permits replacement
after caller consent; it is not a compare-and-swap or crash-durability guarantee.
Cancellation is cooperative around local filesystem operations, not an
interruptible kernel-I/O or hostile-path-race guarantee.

The GUI library is session-local unless explicitly saved. `.silkhmi` contains
placed nodes/points, not the catalog. A ProcessEditor in `.silkui` persists its
**Device library JSON** property through loading, preview and both Go generators.

See [workbench workflow](../../docs/HMI-WORKBENCH.md) for runtime and safety limits.
