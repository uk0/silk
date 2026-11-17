# Obstacle-aware process pipes

```sh
go run ./examples/processrouting
```

The drawing is inert. Its pipe explicitly uses `routing: "avoid"`; old documents
without the field keep their original orthogonal doglegs. Newly connected pipes
use avoidance. **Pipes** edits the mode; **Bends** adds mandatory waypoints.

Drag the reactor and inspect its shared painted/pickable route. Escape cancels
the preview; Undo/Redo restores authored geometry. Waypoint insertion uses the
actual displayed route. Clearance includes conservative rotated equipment bounds
and labels, not just the icon center. Rotated nozzle positions remain exact;
their outward direction is snapped to the nearest cardinal axis.

**Run** is optional and explicit. **Live points → Write value...** can change
`DI_SHOW`, `AI_X` or `AI_ROT` to exercise transient visibility/position/rotation.
There are no HTTP sources or PLC commands. Stop restores the authored scene;
live samples and computed paths never enter saved JSON or generated source.

An amber pipe and fixed-viewport banner mean a blocked route or a bounded-work
fallback, **not successful avoidance**. A waypoint inside equipment, overlapping
port exits or an outward nozzle against the drawing edge can legitimately fail.
Pipe crossings, electrical connectivity, shortest-path optimality and P&ID/PLC
safety certification are not promised.

Save/Load/Generate target the OS temporary directory by default, not the SDK root:
`silk-process-routing.silkhmi`, `.silkhmi.go` and `silk-process-routing.db`.
Use `-project <file> -db <file>` for a separate application profile. Generate
refuses overwrite; keep generated main programs in their own Go module. Reused
point stores may retain compatible latest values across explicit Run actions.
