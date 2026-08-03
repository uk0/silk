package main

import (
	"testing"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

// buildChrome constructs the designer's menu bar and toolbar against a frame
// with no window, which is as much of the chrome as a headless process can
// have. selectionCommands is package state that createMenuBar/createToolBar
// append to, so it is reset first or a second build would assert against the
// previous one's actions too.
func buildChrome(t *testing.T) *gui.Frame {
	t.Helper()
	selectionCommands = nil
	f := gui.NewFrame()
	gui.SetDefaultFrame(f)
	createMenuBar(f)
	createToolBar(f)
	return f
}

// menuButtons returns the buttons of a menu, skipping its separators.
func menuButtons(m *gui.Menu) []*gui.Button {
	var ret []*gui.Button
	for _, it := range m.Items() {
		if b, ok := it.(*gui.Button); ok {
			ret = append(ret, b)
		}
	}
	return ret
}

// submenuButtons returns the items of the named top-level submenu.
func submenuButtons(t *testing.T, mainMenu *gui.Menu, title string) []*gui.Button {
	t.Helper()
	for _, b := range menuButtons(mainMenu) {
		if b.Text() != title {
			continue
		}
		sub, ok := b.SubPopup().(*gui.Menu)
		if !ok {
			t.Fatalf("%q has no submenu", title)
		}
		return menuButtons(sub)
	}
	t.Fatalf("no %q menu", title)
	return nil
}

func toolBarButtons(t *testing.T, f *gui.Frame) []*gui.Button {
	t.Helper()
	tb := f.ToolBar()
	if tb == nil {
		t.Fatal("frame has no toolbar")
	}
	var ret []*gui.Button
	for _, it := range tb.Items() {
		if b, ok := it.(*gui.Button); ok {
			ret = append(ret, b)
		}
	}
	return ret
}

// TestToolBarButtonsSayWhatTheyDo pins the fix for a toolbar of ten bare
// icons: every button was added with an empty label and no hover text, so
// there was no way — short of clicking one — to find out which icon saved and
// which one ran the design.
func TestToolBarButtonsSayWhatTheyDo(t *testing.T) {
	f := buildChrome(t)
	btns := toolBarButtons(t, f)
	if len(btns) == 0 {
		t.Fatal("toolbar has no buttons")
	}
	for i, b := range btns {
		if b.Text() == "" && gui.GetToolTip(b) == "" {
			t.Errorf("toolbar button %d has neither a label nor a tooltip", i)
		}
	}
}

// TestSelectionCommandsFollowSelectionSize covers the commands that quietly did
// nothing: 左对齐 and friends return immediately unless two items are selected,
// 水平分布 unless three, 打破布局 unless exactly one — and all of them stayed
// lit regardless, so clicking one on an empty canvas looked like a broken
// button rather than an unavailable command.
func TestSelectionCommandsFollowSelectionSize(t *testing.T) {
	newStatusBarLabels()
	f := buildChrome(t)

	gv := ged.NewGedView()
	dropWidget(t, gv, "gui.Button")
	dropWidget(t, gv, "gui.Label")
	dropWidget(t, gv, "gui.Edit")
	items := gv.Scene().Children()
	if len(items) != 3 {
		t.Fatalf("scene has %d items, want 3", len(items))
	}

	// menu label -> the selection sizes the command applies to
	applies := map[string]func(n int) bool{
		"左对齐    Alt+L":  func(n int) bool { return n >= 2 },
		"右对齐    Alt+R":  func(n int) bool { return n >= 2 },
		"顶对齐    Alt+T":  func(n int) bool { return n >= 2 },
		"底对齐    Alt+B":  func(n int) bool { return n >= 2 },
		"水平居中    Alt+C": func(n int) bool { return n >= 2 },
		"垂直居中    Alt+M": func(n int) bool { return n >= 2 },
		"水平分布    Alt+H": func(n int) bool { return n >= 3 },
		"垂直分布    Alt+V": func(n int) bool { return n >= 3 },
		"应用水平布局 (HBox)": func(n int) bool { return n >= 2 },
		"应用垂直布局 (VBox)": func(n int) bool { return n >= 2 },
		"应用网格布局 (Grid)": func(n int) bool { return n >= 2 },
		"打破布局":          func(n int) bool { return n == 1 },
	}
	menuItems := append(submenuButtons(t, f.MainMenu(), "排列"),
		submenuButtons(t, f.MainMenu(), "布局")...)
	if len(menuItems) != len(applies) {
		t.Fatalf("排列+布局 hold %d commands, want %d", len(menuItems), len(applies))
	}

	// The three alignment buttons sit at the end of the toolbar. They carry no
	// label, so position is the only way to name them from a test.
	tbAlign := toolBarButtons(t, f)
	tbAlign = tbAlign[len(tbAlign)-3:]

	for n := 0; n <= 3; n++ {
		gv.Selection().Clear()
		for _, it := range items[:n] {
			gv.Selection().Add(it)
		}
		updateStatusBarInfoFor(gv)

		for _, b := range menuItems {
			want, ok := applies[b.Text()]
			if !ok {
				t.Fatalf("unexpected command %q in 排列/布局", b.Text())
			}
			if got := b.IsEnabled(); got != want(n) {
				t.Errorf("%d selected: %q enabled=%v, want %v", n, b.Text(), got, want(n))
			}
		}
		for i, b := range tbAlign {
			if got := b.IsEnabled(); got != (n >= 2) {
				t.Errorf("%d selected: toolbar align button %d enabled=%v, want %v", n, i, got, n >= 2)
			}
		}
	}
}

// TestSelectionCommandsDisabledWithoutACanvas covers the other half: with the
// center dock showing the code editor there is no canvas at all, and every one
// of these commands returns on the nil view.
func TestSelectionCommandsDisabledWithoutACanvas(t *testing.T) {
	newStatusBarLabels()
	f := buildChrome(t)

	gv := ged.NewGedView()
	dropWidget(t, gv, "gui.Button")
	dropWidget(t, gv, "gui.Label")
	for _, it := range gv.Scene().Children() {
		gv.Selection().Add(it)
	}
	updateStatusBarInfoFor(gv)

	updateActionStates(nil)
	for _, b := range submenuButtons(t, f.MainMenu(), "排列") {
		if b.IsEnabled() {
			t.Errorf("%q is still enabled with no canvas", b.Text())
		}
	}
}

// TestStatusBarSelectionCellClearsWithoutACanvas guards the odd cell out: the
// zoom and widget-count cells both reset when there is no canvas to describe,
// and the selection cell did not, so a switch to a non-canvas view left
// "2 selected" sitting next to "0 widgets".
func TestStatusBarSelectionCellClearsWithoutACanvas(t *testing.T) {
	newStatusBarLabels()
	gv := ged.NewGedView()
	dropWidget(t, gv, "gui.Button")
	dropWidget(t, gv, "gui.Label")
	for _, it := range gv.Scene().Children() {
		gv.Selection().Add(it)
	}

	updateStatusBarInfoFor(gv)
	if got := statusInfoLabel.Text(); got != "2 selected" {
		t.Fatalf("selection cell = %q, want %q", got, "2 selected")
	}

	updateStatusBarInfoFor(nil)
	if got := statusInfoLabel.Text(); got != "" {
		t.Errorf("selection cell = %q with no canvas, want it cleared", got)
	}
}

// TestRecentMenuPlaceholderIsNotAcommand covers the "(无)" entry the 最近文件
// submenu shows when the list is empty: it is a label wearing a menu item's
// clothes, and clicking it did nothing at all.
func TestRecentMenuPlaceholderIsNotAcommand(t *testing.T) {
	saved := recentFiles
	defer func() { recentFiles = saved }()

	sub := gui.NewPopupMenu()

	recentFiles = nil
	populateRecentMenu(sub)
	btns := menuButtons(sub)
	if len(btns) != 1 {
		t.Fatalf("empty recent list produced %d entries, want 1", len(btns))
	}
	if btns[0].IsEnabled() {
		t.Errorf("placeholder %q is clickable", btns[0].Text())
	}

	recentFiles = []string{"/tmp/a.ui", "/tmp/b.ui"}
	populateRecentMenu(sub)
	btns = menuButtons(sub)
	if len(btns) != 2 {
		t.Fatalf("two recent files produced %d entries, want 2", len(btns))
	}
	for _, b := range btns {
		if !b.IsEnabled() {
			t.Errorf("recent file entry %q is disabled", b.Text())
		}
	}
}
