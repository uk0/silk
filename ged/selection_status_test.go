package ged

import (
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"strings"
	"testing"
)

type statusSizeCommand struct {
	item      graph.IItem
	old, next float64
}

func (c *statusSizeCommand) Text() string { return "Resize" }
func (c *statusSizeCommand) Redo()        { c.item.SetSize(c.next, 12) }
func (c *statusSizeCommand) Undo()        { c.item.SetSize(c.old, 12) }

func selectionStatusFixture(t *testing.T) (*gui.Frame, *GedView, *FakeWidget, *gui.StatusBar) {
	t.Helper()
	frame := gui.NewFrame()
	sb := gui.NewStatusBar()
	frame.SetStatusBar(sb)
	view := NewGedView()
	view.SetParent(frame)
	item, err := NewFakeWidgetFromFactory("gui.SpinBox")
	if err != nil {
		t.Fatal(err)
	}
	item.SetParent(view.GedScene())
	item.SetBounds(15, 15, 85, 12)
	view.Selection().Add(item)
	frame.OnIdle()
	if !strings.Contains(sb.Message(), "85") {
		t.Fatalf("selection did not reach status: %q", sb.Message())
	}
	return frame, view, item, sb
}
func TestSelectionStatusTracksPropertyCommands(t *testing.T) {
	_, view, item, sb := selectionStatusFixture(t)
	view.GedScene().PushCommand(&statusSizeCommand{item, 85, 60.5})
	if !strings.Contains(sb.Message(), "60.5") {
		t.Fatalf("property edit left stale status %q", sb.Message())
	}
	view.Undo()
	if !strings.Contains(sb.Message(), "85") {
		t.Fatalf("undo status %q", sb.Message())
	}
	view.Redo()
	if !strings.Contains(sb.Message(), "60.5") {
		t.Fatalf("redo status %q", sb.Message())
	}
}
func TestSelectionStatusTracksKeyboardPosition(t *testing.T) {
	_, view, _, sb := selectionStatusFixture(t)
	view.OnKeyDown(gui.KeyRight, false)
	if !strings.Contains(sb.Message(), "(16, 15)") {
		t.Fatalf("nudge status lacks current position: %q", sb.Message())
	}
}

func TestSelectionStatusFormatterAndLifecycle(t *testing.T) {
	frame, view, item, sb := selectionStatusFixture(t)
	format := func(items []graph.IItem) string {
		if len(items) == 0 {
			return "empty"
		}
		return "host " + FormatItemBounds(items[0])
	}
	view.SetSelectionStatusFormatter(format)
	view.OnIdle()
	if sb.Message() != "host (15, 15) 85×12" {
		t.Fatalf("SDK overwrote host formatter: %q", sb.Message())
	}
	view.GedScene().PushCommand(&statusSizeCommand{item, 85, 60.5})
	if sb.Message() != "host (15, 15) 60.5×12" {
		t.Fatal(sb.Message())
	}
	view.Hide()
	sb.ShowMessage("editor active")
	view.GedScene().PushCommand(&statusSizeCommand{item, 60.5, 70.25})
	if sb.Message() != "editor active" {
		t.Fatal("hidden designer stole status", sb.Message())
	}
	view.Show()
	if !strings.Contains(sb.Message(), "70.25") {
		t.Fatal("show left stale status", sb.Message())
	}
	old := view.GedScene()
	otherCalls := 0
	other := old.AddDesignChangedCallback(func() { otherCalls++ })
	fresh := NewGedScene()
	view.SetScene(fresh)
	if view.Selection().Count() != 0 || sb.Message() != "empty" {
		t.Fatal("old selection survived scene replacement", sb.Message())
	}
	old.NotifyDesignChanged()
	if otherCalls != 1 || sb.Message() != "empty" {
		t.Fatal("old scene affected current status")
	}
	old.RemoveDesignChangedCallback(other)
	if len(old.designChangedList) != 0 {
		t.Fatal("old status listener leaked")
	}
	for i := 0; i < 10; i++ {
		view.SetScene(fresh)
	}
	if len(fresh.designChangedList) != 1 {
		t.Fatal("duplicate scene subscriptions", len(fresh.designChangedList))
	}
	next := addFakeAt(t, fresh, "next", 20.125, 30.5, 70.25, 12)
	view.Selection().Add(next)
	frame.OnIdle()
	if !strings.Contains(sb.Message(), "(20.125, 30.5)") {
		t.Fatal("new scene selection missing", sb.Message())
	}
	view.Close()
	view.Close()
	sb.ShowMessage("closed")
	fresh.NotifyDesignChanged()
	view.RefreshSelectionStatus()
	if len(fresh.designChangedList) != 0 || sb.Message() != "closed" {
		t.Fatal("closed view still owns status")
	}
}

func TestSelectionStatusTracksCompletedDragAndSnap(t *testing.T) {
	frame, view, item, sb := selectionStatusFixture(t)
	view.SetSize(700, 500)
	view.Layout()
	view.GedScene().SetSize(200, 150)
	for _, gesture := range []struct {
		name           string
		fx, fy, tx, ty float64
	}{
		{"move", 20, 20, 37.3, 32.7},
	} {
		t.Run(gesture.name, func(t *testing.T) {
			before := FormatItemBounds(item)
			dragOnCanvas(view, gesture.fx, gesture.fy, gesture.tx, gesture.ty)
			frame.OnIdle()
			want := FormatItemBounds(item)
			if want == before {
				t.Fatal("gesture did not change geometry")
			}
			if !strings.Contains(sb.Message(), want) {
				t.Fatalf("drag/snap status=%q want bounds %q", sb.Message(), want)
			}
			view.Undo()
			if !strings.Contains(sb.Message(), FormatItemBounds(item)) {
				t.Fatal("drag undo stale", sb.Message())
			}
			view.Redo()
			if !strings.Contains(sb.Message(), FormatItemBounds(item)) {
				t.Fatal("drag redo stale", sb.Message())
			}
		})
	}
	x, y, w, h := item.Bounds()
	dragOnCanvas(view, x+w-.5, y+h-.5, x+w+12.3, y+h+7.8)
	frame.OnIdle()
	if nw, nh := item.Size(); nw == w && nh == h {
		t.Fatal("resize gesture did not resize")
	}
	if !strings.Contains(sb.Message(), FormatItemBounds(item)) {
		t.Fatal("resize/snap stale", sb.Message())
	}
}

type selectionStatusPaintProbe struct {
	gui.StatusBar
	updates int
}

func (p *selectionStatusPaintProbe) Update() { p.updates++ }
func TestSelectionStatusNoIdleRepaintAndTransientOwnership(t *testing.T) {
	frame, view, _, _ := selectionStatusFixture(t)
	probe := new(selectionStatusPaintProbe)
	probe.Init(probe)
	frame.SetStatusBar(&probe.StatusBar)
	view.RefreshSelectionStatus()
	probe.updates = 0
	for i := 0; i < 100; i++ {
		view.RefreshSelectionStatus()
		view.GedScene().NotifyDesignChanged()
		frame.OnIdle()
	}
	if probe.updates != 0 {
		t.Fatal("unchanged selection repaints", probe.updates)
	}
	probe.ShowMessage("build completed")
	probe.updates = 0
	view.RefreshSelectionStatus()
	view.GedScene().NotifyDesignChanged()
	if probe.Message() != "build completed" || probe.updates != 0 {
		t.Fatal("unchanged design stole transient status")
	}
	view.Hide()
	if probe.Message() != "build completed" {
		t.Fatal("hide cleared another tool message")
	}
	view.Show()
	if !strings.HasPrefix(probe.Message(), "Selected:") {
		t.Fatal("show did not restore selection")
	}
}

func TestSelectionStatusMultipleAndEmptySelection(t *testing.T) {
	frame, view, _, sb := selectionStatusFixture(t)
	other := addFakeAt(t, view.GedScene(), "second", 90, 90, 10, 10)
	view.Selection().Add(other)
	frame.OnIdle()
	if sb.Message() != "Selected: 2 items" {
		t.Fatal(sb.Message())
	}
	view.Selection().Clear()
	frame.OnIdle()
	if sb.Message() != "Ready" {
		t.Fatal(sb.Message())
	}
	view.SetSelectionStatusFormatter(func([]graph.IItem) string { return "custom" })
	view.SetSelectionStatusFormatter(nil)
	if sb.Message() != "Ready" {
		t.Fatal("nil did not restore default")
	}
}

func TestSelectionStatusHiddenAncestor(t *testing.T) {
	frame, view, item, sb := selectionStatusFixture(t)
	parent := new(gui.Widget)
	parent.Init(parent)
	parent.SetParent(frame)
	view.SetParent(parent)
	parent.Hide()
	sb.ShowMessage("another view")
	view.GedScene().PushCommand(&statusSizeCommand{item, 85, 60.5})
	if sb.Message() != "another view" {
		t.Fatal("hidden ancestor allows status write")
	}
	parent.Show()
	view.RefreshSelectionStatus()
	if !strings.Contains(sb.Message(), "60.5") {
		t.Fatal("explicit refresh did not restore actual geometry")
	}
}

func TestSelectionStatusWidgetIdentity(t *testing.T) {
	_, view, item, sb := selectionStatusFixture(t)
	if got := SelectionItemName(item); got != "SpinBox" {
		t.Fatal("adapter type exposed", got)
	}
	item.SetWidgetName(" counter ")
	view.GedScene().NotifyDesignChanged()
	if got := SelectionItemName(item); got != "counter [SpinBox]" {
		t.Fatal("name/type identity missing", got)
	}
	if !strings.Contains(sb.Message(), "counter [SpinBox]") {
		t.Fatal("rename did not refresh status", sb.Message())
	}
	item.SetWidgetName("other")
	view.GedScene().NotifyDesignChanged()
	if !strings.Contains(sb.Message(), "other [SpinBox]") {
		t.Fatal("rename left old identity", sb.Message())
	}
	if SelectionItemName(nil) != "(nil)" || FormatItemBounds(nil) != "" {
		t.Fatal("nil format")
	}
}
