// Package graph provides a scene-graph editing framework.
//
// It implements a model for interactive graphical editing with items,
// scenes, tools, selections, undo/redo, and coordinate transformations.
//
// Key types:
//   - Item/IItem: base graph item with bounds, parent-child hierarchy
//   - SceneItem: root container for graph items
//   - GraphView: scrollable, zoomable view of a scene
//   - Selection: multi-item selection with decoration handles
//   - Tool/Part: input handling chain (select, move, resize, marquee)
//   - UndoStack: command-based undo/redo
package graph
