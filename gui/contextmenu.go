package gui

import (
	"sync"
)

// IContextMenuProvider — any widget can implement this interface to provide
// a context menu when right-clicked.
type IContextMenuProvider interface {
	BuildContextMenu(menu *Menu, x, y float64)
}

// contextMenuRegistry stores right-click menu builders for widgets
// that don't implement IContextMenuProvider directly.
var (
	contextMenuRegistry   = make(map[IWidget]func(*Menu, float64, float64))
	contextMenuRegistryMu sync.Mutex
)

// AttachContextMenu attaches a right-click context menu builder to any widget.
// The builder function is called when the widget receives a right-click event,
// and is responsible for populating the menu items.
//
// Usage:
//
//	gui.AttachContextMenu(myWidget, func(m *Menu, x, y float64) {
//	    m.AddButton1("Copy", nil).Action().BindFunc0(func() { ... })
//	    m.AddButton1("Paste", nil).Action().BindFunc0(func() { ... })
//	})
func AttachContextMenu(widget IWidget, builder func(menu *Menu, x, y float64)) {
	contextMenuRegistryMu.Lock()
	defer contextMenuRegistryMu.Unlock()
	if builder == nil {
		delete(contextMenuRegistry, widget)
	} else {
		contextMenuRegistry[widget] = builder
	}
}

// DetachContextMenu removes a previously attached context menu builder.
func DetachContextMenu(widget IWidget) {
	contextMenuRegistryMu.Lock()
	defer contextMenuRegistryMu.Unlock()
	delete(contextMenuRegistry, widget)
}

// TryContextMenu attempts to show a context menu for the given widget.
// It first checks for a registered builder, then checks if the widget
// implements IContextMenuProvider. Returns true if a menu was shown.
func TryContextMenu(widget IWidget, x, y float64) bool {
	contextMenuRegistryMu.Lock()
	builder, ok := contextMenuRegistry[widget]
	contextMenuRegistryMu.Unlock()

	if ok {
		ShowContextMenu(widget, x, y, func(menu *Menu) {
			builder(menu, x, y)
		})
		return true
	}

	if provider, ok := widget.(IContextMenuProvider); ok {
		ShowContextMenu(widget, x, y, func(menu *Menu) {
			provider.BuildContextMenu(menu, x, y)
		})
		return true
	}

	return false
}

// ShowContextMenu creates a popup menu at the given widget-local coordinates,
// calls the builder to populate it, then shows it as a popup.
func ShowContextMenu(widget IWidget, x, y float64, builder func(menu *Menu)) {
	menu := NewPopupMenu()
	builder(menu)

	// Only show if builder added items
	if len(menu.Items()) == 0 {
		return
	}

	gx, gy := widget.MapToGlobal(x, y)
	menu.ShowAsPopup(gx, gy, true)
}
