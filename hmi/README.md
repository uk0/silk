# Portable industrial 2D documents and editing SDK

`hmi` is the pure-Go, GUI-independent model behind `gui.ProcessDiagram` and
`gui.ProcessEditor`. Parsing, validating and editing a document never starts
HTTP, SQL, timers or drivers. Runtime construction/start are explicit operations.
See the [workbench guide](../docs/HMI-WORKBENCH.md) for acquisition and rendering.

## Atomic authored edits

```go
p := hmi.Template()
next, err := hmi.MoveNodes(p, []string{"reactor", "exchanger"}, 40, 20)
if err != nil {
    // p is still untouched; do not publish a zero-value candidate.
    return err
}
return hmi.Save("plant.silkhmi", next)
```

All editing functions validate the input and complete candidate. They return
independent slices/maps; failed edits do not mutate the input. Initial point
values retain their exact scalar Go types, including full-range `int64`.
Documents must not be mutated concurrently by their owner; returned copies are
not a synchronization mechanism. GUI mutation belongs on the UI thread.

| Operation | Contract |
|---|---|
| `CloneProject` | Independent validated document; no JSON scalar round-trip loss |
| `SelectionBounds` | Axis-aligned bounds of selected devices |
| `TranslationBounds` | Device bounds plus wholly internal pipe waypoints |
| `ClampTranslation` | One finite delta clamped for the whole group |
| `MoveNodes` | Same delta for devices/internal waypoints; external bends stay fixed |
| `ResizeNode` | New rectangle, constrained by normal document validation |
| `AlignNodes` | At least two; left/right/top/bottom/horizontal-center/vertical-center |
| `DistributeNodes` | At least three; equal nonnegative gaps; end nodes stay fixed |
| `ReorderNodes` | Front/back with stable relative order within both groups |
| `DuplicateNodes` | Unique Unicode-safe IDs; copy selected nodes and internal pipes |
| `DeleteNodes` | Remove selected nodes and all incident pipes |
| `EditLinkBend` | Validated insert/replace/delete; nil point means deletion |

Unknown/duplicate selection IDs, non-finite coordinates, invalid bounds or
document budgets produce errors. An empty or undersized selection is not
silently interpreted as “all”. Duplication intentionally shares the existing
point bindings and does not clone HTTP writers or create new point definitions.

Alignment and distribution reposition devices; endpoint ports follow them, but
waypoints remain at their authored coordinates. Group translation is the
operation that moves a complete selected subgraph and its internal waypoints.
The document format remains version 1: transient selection, pointer gestures,
read-only state and undo snapshots are not serialized.

## Interaction and limits

`ProcessDiagram` previews movement/resize/waypoints separately from the saved
document, then records one edit on release. Escape/capture cancellation restores
the preview; undo/redo is bounded to 32 snapshots. `SetReadOnly` prevents author
commands without applying runtime property bindings; `SetEditing(false)` selects
runtime rendering behavior. Selection/action callbacks and GUI methods are
UI-thread-owned, not safe worker entry points.

`gui.ShowProcessDocumentEditor(owner, project, commit)` embeds the same authoring
workflow in an owned modal. Drafts are independent and inert; Cancel is a no-op,
Use changes submits only changed authored state, and a failing host commit keeps
the editor open. The host must guard its source revision, publish one undo
transaction and decide when to persist. SilkIDE's **Edit 2D Process...** command
demonstrates this against the normal JSON source buffer and Save All workflow.

Orthogonal routes are deterministic doglegs, not obstacle avoidance. This is a
schematic authoring foundation, not a certified control/safety system, alarm
lifecycle or PLC write driver. See [current product plan](../PRODUCT_ITERATION_PLAN.md).

```sh
go test -race -count=1 ./hmi ./hmiapp ./gui ./ged
```

The optional `hmiapp` adapter uses SQLite/CGO; `hmi` edits do not require CGO.
The exact platform/native QA scope is in [authoring QA](../HMI_AUTHORING_QA_20261114.md).
