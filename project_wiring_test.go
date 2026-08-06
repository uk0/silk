package main

import (
	"strings"
	"testing"
)

// TestFileTreeOpensDesignsOnCanvas: double-clicking a .silkui in the project
// tree must open it as a design document, not as text. The tree's callback
// used to hand every path to the code editor, so opening a design from the
// project showed the user its raw TDoc instead of the form.
//
// createPanels needs a live frame and docks, so there is no way to drive the
// callback from a headless test; this reads the wiring the way run_test.go
// reads onRun's.
func TestFileTreeOpensDesignsOnCanvas(t *testing.T) {
	body := funcBody(t, "design.go", "func createPanels(mainFrame *gui.Frame) {")

	hook := strings.Index(body, "fileExplorer.SigFileOpen(")
	if hook < 0 {
		t.Fatal("createPanels no longer wires fileExplorer.SigFileOpen; update this test to match")
	}
	callback := body[hook:]

	design := strings.Index(callback, "ged.IsDesignPath(path)")
	if design < 0 {
		t.Fatal("the file tree does not recognise designs; every path still goes to the text editor")
	}
	open := strings.Index(callback, "openDesignFile(path)")
	if open < 0 {
		t.Fatal("a design opened from the file tree is not routed to openDesignFile")
	}
	editor := strings.Index(callback, "editorTabs.OpenFile(path)")
	if editor >= 0 && editor < design {
		t.Error("the text editor claims the path before the design check runs")
	}

	// OnDesignMode activates the first GedView in the dock, so a mode switch
	// after the open lands on the wrong document.
	if mode := strings.Index(callback, "ged.ModeDesign)"); mode < 0 || mode > open {
		t.Error("the mode switch does not precede the open; the clicked design would not stay active")
	}
}

// TestFileMenuOffersProjectCommands: 打开项目... and 全部生成 are the project's
// two entry points and both belong in the 文件 menu.
func TestFileMenuOffersProjectCommands(t *testing.T) {
	body := funcBody(t, "design.go", "func createMenuBar(mainFrame *gui.Frame) {")

	fileMenu := strings.Index(body, `AddSubMenu("文件"`)
	editMenu := strings.Index(body, `AddSubMenu("编辑"`)
	if fileMenu < 0 || editMenu < 0 || editMenu < fileMenu {
		t.Fatal("createMenuBar no longer builds 文件 then 编辑; update this test to match")
	}
	section := body[fileMenu:editMenu]

	for _, want := range []string{"打开项目...", "onOpenProject", "全部生成", "onGenerateAll"} {
		if !strings.Contains(section, want) {
			t.Errorf("文件 menu is missing %q", want)
		}
	}
}
