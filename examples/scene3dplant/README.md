# Device-template plant assembly

A pure-Go, headless example that loads one authored OBJ, glTF/GLB or `.silkscene`
model and builds a portable multi-device scene. It creates no window, driver,
network connection or TagDB subscription.

From the repository root:

```sh
go run ./examples/scene3dplant \
  -model examples/scene3d/assembly.silkscene \
  -count 25 -columns 5 -spacing 6 \
  -out /tmp/plant.silkscene
```

`-out` is required; an existing regular file at that explicit destination is
replaced only after validation and a successful temporary-file write. Choose a
writable output path appropriate to Windows/Linux/macOS. Ctrl+C cancels the
operation at context-check boundaries; file-system blocking calls still depend
on the OS. Invalid dimensions, input, budgets or cancellation do not truncate an
existing output. Spacing is in model units, **not** automatic collision avoidance.

Open the output in SilkIDE and use **Tools → Edit 3D Scene...**, or load it in a
`gui.SceneView` and call `FitScene`. A placement adds a meshless wrapper:

- `plant` is the scene group; `device-0001` … are independently movable wrappers.
- An original `model/node/2` becomes `device-0001/model/node/2`.
- `tank.level` becomes `device-0001.tank.level`; the exact same source geometry is
  shared while transforms, visibility, colors and binding destinations are not.
- Loaded bindings remain inert. The host must create its desired points, choose
  a TagDB and explicitly call `SetSceneBindings`. No simulated measurements are
  silently installed in the saved document.
- Original warnings are kept. Geometry instancing is not hardware-instanced GPU
  drawing, and unsupported source materials/animations do not become supported
  by assembling more copies.

For the repository's assembly fixture, 1000 devices / 40 columns produce 9001
nodes and 3000 bindings but still only **2 unique serialized meshes**. This is a
storage/sharing property, not a promised frame rate. Every visible mesh instance
still counts toward the SDK's snapshot budgets; the node limit includes wrappers.

The current IDE edits existing nodes and bindings; device-library drag/drop,
interactive copy/delete, gizmos and hardware instancing remain separate work.
