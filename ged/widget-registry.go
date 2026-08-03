package ged

// "Which widgets can a user place on a form" is one fact with two consumers:
// the palette decides what is draggable (categoryDefs, ged/widget-list.go) and
// codegen decides what can be emitted for it (factoryMap, ged/codegen.go).
// They were independent hand-maintained lists and drifted in both directions —
// the palette filtered a blocklist, so every factory registered after the
// blocklist was written appeared in it, including all 33 ged.* IDE dock panels.
// Dropping one emitted core.New("ged.TerminalPanel").(gui.IWidget); the
// generated program links gui and device but never ged, so core.New returned
// nil and the assertion panicked before the window opened.
//
// factoryMap is now the source of truth for *whether* a widget is placeable;
// categoryDefs only says *where* in the palette it appears. A widget reaches
// the palette exactly when codegen can emit it, and a new IDE panel never
// reaches it at all. ged/codegen_registry_test.go fails naming any factory
// present in one list and missing from the other.

// paletteOptOut names factoryMap entries the palette deliberately does not
// offer, with the reason. It is the explicit escape hatch the drift test
// honours: codegen can emit these, but dragging one onto a form is not
// something the designer supports.
var paletteOptOut = map[string]string{
	// The scrolling base other views embed (ListWidget/TreeView/Table). On its
	// own it has no content API — no AddWidget, no SetContent — so a placed one
	// would scroll nothing. Excluded from the palette since the blocklist days;
	// the mapping stays so a .silkui saved back then still generates.
	"gui.ScrollArea": "scrolling base class; nothing in the designer can fill it",
}

// isPlaceable reports whether a registered factory may appear in the widget
// palette and be dragged onto a form.
func isPlaceable(factoryName string) bool {
	if _, ok := factoryMap[factoryName]; !ok {
		return false
	}
	_, optedOut := paletteOptOut[factoryName]
	return !optedOut
}
