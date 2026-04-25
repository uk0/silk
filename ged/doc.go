// Package ged implements the visual GUI editor (Silk Designer).
//
// It provides a drag-and-drop design canvas where users place widgets,
// edit their properties, write event handler code, and export the design
// as compilable Go source files.
//
// Key types:
//   - GedView: the main editor view (embeds graph.GraphView)
//   - GedScene: the design canvas containing FakeWidgets
//   - FakeWidget: a design-time representation of a GUI widget
//   - WidgetList: a palette of available widgets for drag-and-drop
//   - CodePanel: syntax-highlighted Go code editor for event handlers
//   - CodeGenOptions: controls Go code generation output
package ged
