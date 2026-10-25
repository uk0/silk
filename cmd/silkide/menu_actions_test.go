package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func menuCommandButton(menu *gui.Menu, id string) *gui.Button {
	for _, child := range menu.Children() {
		if b, ok := child.(*gui.Button); ok {
			if b.Action().ObjName() == id {
				return b
			}
			if sub, ok := b.SubPopup().(*gui.Menu); ok {
				if found := menuCommandButton(sub, id); found != nil {
					return found
				}
			}
		}
	}
	return nil
}

func TestFileMenuInventoryAndEnabledRouting(t *testing.T) {
	old := globalPrefs
	globalPrefs = nil
	t.Cleanup(func() { globalPrefs = old })
	w := testWorkbench(t)
	w.activate(modeDesign)
	m := buildHamburgerMenu(gui.NewButton(), w.tabs, w.canvas)
	for _, id := range []string{"file.new", "file.open", "file.save", "file.saveAll", "file.export", "tools.a11y", "project.settings", "help.about"} {
		b := menuCommandButton(m, id)
		if b == nil || b.Icon() == nil || b.Text() == "" || !b.IsEnabled() {
			t.Fatalf("menu entry not usable: %s (%v)", id, b)
		}
	}
	w.activate(modeWelcome)
	if menuCommandButton(buildHamburgerMenu(gui.NewButton(), w.tabs, w.canvas), "file.save").IsEnabled() {
		t.Fatal("Save is enabled without an active document")
	}

	m = gui.NewPopupMenu()
	calls := 0
	b := appendFileMenuCommand(m, fileMenuCommand{"test", "Save", "save", "", func() { calls++ }, true})
	b.Action().Trigger(b)
	b.Action().SetEnabled(false)
	b.Action().Trigger(b)
	if calls != 1 {
		t.Fatalf("menu callback invoked %d times", calls)
	}
}

func TestFileMenuSaveWritesVisibleEditor(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "main.go", "package main\n")
	if !openFileInEditor(w.tabs, path) {
		t.Fatal("open")
	}
	w.activate(modeEdit)
	activeEditor(w.tabs).ReplaceAllText("package main\n// menu save\n")
	b := menuCommandButton(buildHamburgerMenu(gui.NewButton(), w.tabs, w.canvas), "file.save")
	b.Action().Trigger(b)
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "// menu save") {
		t.Fatalf("menu saved wrong document: %s %v", data, err)
	}
}

func TestPaletteEditorActionCannotMutateHiddenDocument(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "main.go", "package main  \n")
	openFileInEditor(w.tabs, path)
	ed := activeEditor(w.tabs)
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	registerPaletteCommands(w.tabs, w.canvas)
	var run func()
	for _, c := range paletteCommands {
		if c.Name == i18n.T("Trim Trailing Whitespace") {
			run = c.Fn
		}
	}
	if run == nil {
		t.Fatal("command missing")
	}
	w.activate(modeDesign)
	run()
	if !strings.Contains(ed.Text(), "main  \n") {
		t.Fatal("hidden editor modified from Design mode")
	}
	w.activate(modeEdit)
	run()
	if strings.Contains(ed.Text(), "main  \n") {
		t.Fatal("visible editor command failed")
	}
}

func TestExportFeedbackUsesActualWrittenPath(t *testing.T) {
	old := silkideToast
	t.Cleanup(func() { silkideToast = old })
	var text string
	var level gui.ToastLevel
	silkideToast = func(s string, l gui.ToastLevel) { text, level = s, l }
	canvas := ged.NewGedView()
	base := filepath.Join(t.TempDir(), "exported")
	if !performExportTo(base, canvas) {
		t.Fatal("export failed")
	}
	if _, err := os.Stat(base + ".svg"); err != nil || !strings.Contains(text, "exported.svg") || level != gui.ToastSuccess {
		t.Fatalf("wrong success feedback: %q %v", text, err)
	}
	if performExportTo(filepath.Join(base, "missing", "test.svg"), canvas) || level != gui.ToastError {
		t.Fatal("export error was hidden")
	}
	text = "unchanged"
	if performExportTo("", canvas) || text != "unchanged" {
		t.Fatal("cancelled export claimed an outcome")
	}
}

func TestSaveAllReportsEvenWhenAlreadyClean(t *testing.T) {
	old := silkideToast
	editors := openEditors
	openEditors = map[string]*gui.CodeEditor{}
	t.Cleanup(func() { silkideToast = old; openEditors = editors })
	var message string
	silkideToast = func(s string, _ gui.ToastLevel) { message = s }
	if !performSaveAll(nil, nil) || message != i18n.T("All documents are saved") {
		t.Fatal("clean Save All has no visible outcome")
	}
}

func TestProjectSettingsMenuUsesAndPersistsSelectedRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/selected\n\ngo 1.26.0\n")
	dlg, panel := buildProjectSettingsDialog(nil, dir)
	if panel.ProjectRoot() != dir || dlg.SizeHints().Width != 640 {
		t.Fatal("settings does not use selected project or requested width")
	}
	kit := panel.Kit()
	kit.Tags = []string{"menu_saved"}
	panel.SetKit(kit)
	if panel.SaveError() != nil {
		t.Fatal(panel.SaveError())
	}
	_, loaded := buildProjectSettingsDialog(nil, dir)
	if len(loaded.Kit().Tags) != 1 || loaded.Kit().Tags[0] != "menu_saved" {
		t.Fatal("menu settings changes were not persisted")
	}
}
