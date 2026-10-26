package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/i18n"
)

func wireSelectionStatus(canvas *ged.GedView) {
	if canvas == nil {
		return
	}
	canvas.SetSelectionStatusFormatter(func(items []graph.IItem) string {
		switch len(items) {
		case 0:
			return ""
		case 1:
			return i18n.Tf("Selected: %s", itemDisplayName(items[0])) + " " + itemBoundsLabel(items[0]) + " mm"
		default:
			return i18n.Tf("Selected: %d items", len(items))
		}
	})
}
