package ged

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

// FormatItemBounds describes committed scene geometry, in scene units (mm).
// Ten significant digits keep fractional dimensions readable without showing
// binary arithmetic noise. This only formats text; it never rounds the model.
func FormatItemBounds(item graph.IItem) string {
	if item == nil {
		return ""
	}
	number := func(v float64) string {
		if v == 0 {
			return "0"
		}
		return strconv.FormatFloat(v, 'g', 10, 64)
	}
	x, y := item.Pos()
	w, h := item.Size()
	return "(" + number(x) + ", " + number(y) + ") " + number(w) + "×" + number(h)
}

// SelectionItemName identifies the designed widget, not its FakeWidget adapter.
// Named widgets include their factory type; SDK status bars and IDE hosts share
// the same identity. Whitespace is collapsed only in this single-line display.
func SelectionItemName(item graph.IItem) string {
	if item == nil {
		return "(nil)"
	}
	if f, ok := item.(*FakeWidget); ok {
		kind := f.WidgetFactoryName()
		if i := strings.LastIndex(kind, "."); i >= 0 {
			kind = kind[i+1:]
		}
		name := strings.Join(strings.Fields(f.WidgetName()), " ")
		if name != "" {
			return name + " [" + kind + "]"
		}
		return kind
	}
	if t, ok := item.(interface{ Title() string }); ok && t.Title() != "" {
		return t.Title()
	}
	return fmt.Sprintf("%T", item)
}

// SetSelectionStatusFormatter customizes the shared frame's selection message.
// nil restores the SDK default. The callback is UI-thread-only and must not
// mutate the selection; returning empty text hides the selection message.
// This avoids a host selection callback being overwritten by the SDK afterward.
func (v *GedView) SetSelectionStatusFormatter(fn func([]graph.IItem) string) {
	v.selectionStatusFormatter = fn
	v.selectionStatusBar = nil
	v.RefreshSelectionStatus()
}

func defaultSelectionStatus(items []graph.IItem) string {
	if len(items) == 0 {
		return "Ready"
	}
	if len(items) != 1 {
		return fmt.Sprintf("Selected: %d items", len(items))
	}
	item := items[0]
	return "Selected: " + SelectionItemName(item) + " " + FormatItemBounds(item) + " mm"
}

// RefreshSelectionStatus updates geometry after an explicit model mutation.
// Ordinary commands, selection changes and completed drag/snap operations call
// this automatically. Direct setters in SDK hosts may call it explicitly.
// There is no idle polling, worker or timer. Unchanged text does not repaint or
// overwrite another tool's transient message. Hidden/closed views stay silent.
func (v *GedView) RefreshSelectionStatus() {
	if v.selectionStatusClosed || v.Scene() == nil {
		return
	}
	for w := v.Self(); w != nil; w = w.Parent() {
		if !w.IsVisible() {
			return
		}
	}
	frame := gui.FindOwnerFrame(v)
	if frame == nil {
		return
	}
	sb := frame.StatusBar()
	if sb == nil {
		return
	}
	// A headless/unused designer needs no status subscription. Bind only once
	// an actual visible status destination exists, then retire on scene/close.
	if scene, ok := v.Scene().(*GedScene); ok && v.selectionStatusScene != scene {
		v.selectionStatusScene = scene
		v.selectionStatusListener = scene.AddDesignChangedCallback(func() {
			if v.Scene() == scene {
				v.RefreshSelectionStatus()
			}
		})
	}
	fn := v.selectionStatusFormatter
	if fn == nil {
		fn = defaultSelectionStatus
	}
	text := fn(v.Selection().ItemList())
	if v.selectionStatusBar == sb && v.selectionStatusText == text {
		return
	}
	v.selectionStatusBar = sb
	v.selectionStatusText = text
	// A new selection/status owns this message, including its lifetime; cancel
	// a prior transient timer even if its text happens to be identical.
	sb.ShowMessage(text)
}

func (v *GedView) clearSelectionStatus() {
	if sb := v.selectionStatusBar; sb != nil && sb.Message() == v.selectionStatusText && v.selectionStatusText != "" {
		sb.ShowMessage("")
	}
	v.selectionStatusBar = nil
	v.selectionStatusText = ""
}

func (v *GedView) OnShow() {
	v.selectionStatusBar = nil
	v.RefreshSelectionStatus()
	v.refreshPropertyValues()
}
func (v *GedView) OnHide() { v.clearSelectionStatus() }

func (v *GedView) stopSelectionStatus() {
	if v.selectionStatusScene != nil {
		v.selectionStatusScene.RemoveDesignChangedCallback(v.selectionStatusListener)
	}
	v.selectionStatusScene = nil
	v.selectionStatusListener = 0
	v.selectionStatusClosed = true
	v.clearSelectionStatus()
}

// SetScene retires the old status subscription and selection before rebinding.
// Other panels retain their own independent design/selection subscriptions.
func (v *GedView) SetScene(scene graph.IScene) {
	if scene == v.Scene() && !v.selectionStatusClosed {
		v.RefreshSelectionStatus()
		return
	}
	v.stopSelectionStatus()
	if v.Scene() != nil {
		v.Selection().Clear()
	}
	v.GraphView.SetScene(scene)
	v.selectionStatusClosed = false

	v.RefreshSelectionStatus()
}
