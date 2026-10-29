# Atomic scene authoring SDK and IDE integration

> 阶段记录：以下保留本候选的实现、失败和测试边界；文中的待验收/未提交状态描述的是当时的候选。
> 最终整合版本使用独立的 2500 文件冻结清单重新验证，当前提交门禁与平台结果见
> [最终验收矩阵](SCENE_RELEASE_QA_20261028.md)。旧候选结果不替代最终版测试。

Candidate based on the frozen template, frustum and instancing iterations. No
release claim: final platform results and native UI evidence are recorded under
`bin/qa-artifacts/scene-authoring-v2-20261028/`. A build, skipped test, or older
candidate result is not this candidate's acceptance.

## Contract

- `scene3d.EditSession`: private inert document, atomic AddNode/AddMeshNode,
  UpdateNode, binding upsert/remove, DuplicateSubtree, RemoveSubtree and Reparent.
  No files, tag database, subscriptions, goroutines or GPU contexts are opened.
- Geometry is immutable and shared across instances, node edits and history.
  Name/binding-only edits retain identical snapshot identity when draw contents
  are unchanged, so CPU/GPU render caches need not rebuild or re-upload meshes.
- Undo/redo: at most 32 retained states and 32 MiB estimated history metadata.
  Current/initial state and shared geometry are separate from that metadata cap;
  document geometry limits still apply. New edits discard redo. Revisions remain
  monotonic through undo. Failed/cancelled/no-op edits do not consume history.
- Duplicate: local root offset, sibling placement, deterministic descendant IDs,
  no copied bindings by default. Explicit binding copies can remap tags; unchanged
  names intentionally refer to the same process points. No live drivers started.
- Delete: whole subtree plus its binding declarations, preserving other ordering
  and all mesh resources. Undo restores exact authored metadata and transforms.
- Reparent: keep local or preserve authored world transform. Hidden/group nodes
  supported. Cycles, missing parents, singular/unsafe inverses rejected atomically.
  Preserve-world uses 1e-9 relative/absolute matrix-element tolerance, not bitwise
  equality. Runtime binding values and inherited visibility are not frozen.

## GUI

The existing scene inspector now offers an owned **Node actions** popup (Add,
Duplicate, Change parent, Delete), built-in group/box/cylinder creation, and Undo /
Redo. Box/cylinder resources are reused instead of allocating per-node meshes.
All menu/toolbar icons use existing high-DPI procedural icon names; tests reject
`image-missing`, not just nil icons. Delete defaults/focuses Cancel. Every delayed
form checks its captured revision, including after undo back to the source.

Small windows wrap the toolbar; spacing is bounded even at 0x0/1x1 host sizes.
Preview framing and tree/pick selection stay synchronized. Internal edits remain
staged; **Use changes** makes one host text-buffer transaction. Cancel does not
write disk. Saving uses the existing editor file transaction and can be undone /
redone as one edit. No new timer or paint-loop validation is introduced.

## Regression scope

- SDK: aliases/ownership, mesh pointer reuse, snapshot identity, error rollback,
  deterministic mid-operation cancellation, duplicate/tag conflicts, resource-ID
  branch collisions, deep unordered 10,000-node hierarchy, real metadata budgets,
  history, exact undo, save/reload, and randomized affine transforms.
- Numerical: 500 deterministic shear/reflection/nonuniform transform cases,
  singular/nearly singular/nonfinite/projective rejection; inverse has zero heap
  allocations. No unsupported projective inverse / pseudoinverse.
- GUI: every menu action and history button, safe defaults, stale modal callbacks,
  empty/closed states, shared resources, framing, no-overlap layout and icon identity.
- IDE: actual CodeEditor buffer -> scene inspector public APIs -> one commit ->
  disk unchanged until Save -> Undo/Redo -> atomic save -> document reload equality.
- Fuzz: bounded edit sequences validate failed-edit atomicity, budgets and graph
  rebuild after each operation. Benchmarks measure CPU authoring only, not FPS.

Native interactive desktop tests and Windows-specific GPU/dispatch tests remain
mandatory before publication. Headless hooks supplement but never replace them.

## Native UI follow-up

The first native authoring run found Fit scene incorrectly enabled for a group-only
scene, producing an invalid-geometry error. The derived candidate disables Fit
when the cached snapshot contains no visible geometry. Regression covers group-only,
hidden descendants and undo restoration; the original failing test and earlier
source manifest remain preserved separately.
