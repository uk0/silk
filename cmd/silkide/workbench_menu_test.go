package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"testing"
)

func TestWorkbenchMenuGroupsAndGuards(t *testing.T) {
	w := testWorkbench(t)
	w.activate(modeWelcome)
	buildWorkbenchMenu(w.frame, w.tabs, w.canvas)
	groups := []string{"File", "Edit", "View", "Design", "Build", "Tools", "Help"}
	if len(w.frame.MainMenu().Items()) != len(groups) {
		t.Fatal("missing menu groups")
	}
	for i, group := range groups {
		header := w.frame.MainMenu().Items()[i].(*gui.Button)
		if header.Action().ObjName() != "menu."+group || header.SubPopup() == nil {
			t.Fatalf("bad header %s", group)
		}
		menu := buildWorkbenchSubMenu(group, w.tabs, w.canvas)
		if len(menu.Items()) == 0 {
			t.Fatalf("empty menu: %s", group)
		}
		for _, item := range menu.Items() {
			if b, ok := item.(*gui.Button); ok && b.SubPopup() == nil && (b.Icon() == nil || b.Text() == "") {
				t.Fatalf("incomplete icon/label: %s", b.Action().ObjName())
			}
		}
	}
	file := buildFileMenu(w.tabs, w.canvas)
	for _, id := range []string{"file.save", "file.export", "file.print"} {
		if menuCommandButton(file, id).IsEnabled() {
			t.Fatalf("hidden design command enabled: %s", id)
		}
	}
	if menuCommandButton(file, "file.save").ShortcutText() == "" {
		t.Fatal("Save shortcut is tooltip-only")
	}
	w.activate(modeDesign)
	if !menuCommandButton(buildFileMenu(w.tabs, w.canvas), "file.export").IsEnabled() {
		t.Fatal("active design export disabled")
	}
	if menuCommandButton(buildWorkbenchSubMenu("Edit", w.tabs, w.canvas), "edit.undo").IsEnabled() {
		t.Fatal("undo enabled with empty history")
	}
}

func TestWorkbenchMenuViewModeRoutes(t *testing.T) {
	w := testWorkbench(t)
	w.activate(modeDesign)
	menu := buildWorkbenchSubMenu("View", w.tabs, w.canvas)
	for _, mode := range []workspaceMode{modeWelcome, modeEdit, modeDesign} {
		menuCommandButton(menu, "view."+string(mode)).Action().Trigger(nil)
		if documentMode(w.tabs, w.canvas) != mode {
			t.Fatalf("View menu failed to activate %s", mode)
		}
	}
}

func TestWorkbenchDesignMenuCannotMutateHiddenDocument(t *testing.T) {
	w := testWorkbench(t)
	w.activate(modeDesign)
	for range 2 {
		child, err := ged.NewFakeWidgetFromFactory("gui.Button")
		if err != nil {
			t.Fatal(err)
		}
		child.SetParent(w.canvas.GedScene())
		w.canvas.Selection().Add(child)
	}
	visible := buildWorkbenchSubMenu("Design", w.tabs, w.canvas)
	w.activate(modeWelcome)
	hidden := buildWorkbenchSubMenu("Design", w.tabs, w.canvas)
	for _, item := range hidden.Items() {
		if button, ok := item.(*gui.Button); ok && button.IsEnabled() {
			t.Fatal("hidden design menu still has enabled actions")
		}
	}
	// A command built before a mode switch also checks current mode at dispatch.
	for _, item := range visible.Items() {
		if button, ok := item.(*gui.Button); ok {
			if button.Action().ObjName() == "design.layout-hbox" {
				button.Action().Trigger(nil)
			}
		}
	}
	if len(w.canvas.GedScene().Children()) != 2 || w.canvas.GedScene().UndoStack().CanUndo() {
		t.Fatal("stale menu mutated hidden design")
	}
}

func TestWidgetPaletteSearchAndContainedBounds(t *testing.T) {
	// Build a real palette: the filter must keep factory data for drag/drop.
	list := ged.NewWidgetList()
	p := buildWidgetPalette(list)
	p.SetSize(220, 400)
	p.search.SetText("gui.GanttChart")
	if list.Count() != 1 || list.Item(0).Data != "gui.GanttChart" {
		t.Fatal("search lost matching factory")
	}
	p.search.Clear()
	if list.Count() < 20 {
		t.Fatal("clear did not restore palette")
	}
	for _, size := range [][2]float64{{220, 400}, {180, 80}, {20, 8}, {0, 0}} {
		p.SetSize(size[0], size[1])
		if list.Y() < p.search.Y()+p.search.Height() && list.Height() > 0 {
			t.Fatal("search overlaps drag rows")
		}
		if list.Height() < 0 || list.Y()+list.Height() > p.Height() {
			t.Fatal("palette list floats outside parent")
		}
	}
}

func TestReleaseWorkbenchMenuDetachesOwnedTree(t *testing.T) {
	m := gui.NewPopupMenu()
	sub, header := m.AddSubMenu("Recent", nil, nil)
	sub.SetParent(header)
	m.SetParent(gui.NewButton())
	releaseWorkbenchMenu(m)
	if m.Parent() != nil || sub.Parent() != nil || m.Window() != nil || sub.Window() != nil {
		t.Fatal("replaced menu tree retained windows/ownership")
	}
}
