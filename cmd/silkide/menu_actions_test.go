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
	saves, posts := installTestSourceSaver(t, w)
	path := writeFile(t, t.TempDir(), "main.go", "package main\n")
	if !openTestEditorReady(t, w.tabs, path) {
		t.Fatal("open")
	}
	w.activate(modeEdit)
	activeTestEditorReady(t, w.tabs).ReplaceAllText("package main\n// menu save\n")
	b := menuCommandButton(buildHamburgerMenu(gui.NewButton(), w.tabs, w.canvas), "file.save")
	b.Action().Trigger(b)
	if saves.pending == nil {
		t.Fatal("file menu save not admitted")
	}
	pumpOwnedSaves(t, posts, func() bool { return saves.pending == nil })
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "// menu save") {
		t.Fatalf("menu saved wrong document: %s %v", data, err)
	}
}

func TestPaletteEditorActionCannotMutateHiddenDocument(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "main.go", "package main  \n")
	openTestEditorReady(t, w.tabs, path)
	ed := activeTestEditorReady(t, w.tabs)
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

func TestDesignExportMenuRoutesFormatAndProtectsConfirmedPath(t *testing.T) {
	w := testWorkbench(t)
	w.activate(modeDesign)
	oldChoose, oldToast := chooseDesignExportFile, silkideToast
	t.Cleanup(func() { chooseDesignExportFile, silkideToast = oldChoose, oldToast })
	root := t.TempDir()
	path, requested := "", ""
	changeDesign := false
	chooseDesignExportFile = func(extension string) string {
		requested = extension
		if changeDesign {
			w.canvas.GedScene().SetFormTitle("Changed during save panel")
			w.activate(modeWelcome)
		}
		return path
	}
	var level gui.ToastLevel
	silkideToast = func(_ string, l gui.ToastLevel) { level = l }

	for _, spec := range []struct {
		ext, magic string
	}{{"pdf", "%PDF-"}, {"svg", "<svg"}} {
		id := "file.export." + spec.ext
		button := menuCommandButton(buildFileMenu(w.tabs, w.canvas), id)
		if button == nil || !button.IsEnabled() {
			t.Fatalf("missing enabled command %s", id)
		}
		path = filepath.Join(root, "design."+spec.ext)
		button.Action().Trigger(button)
		body, err := os.ReadFile(path)
		if requested != spec.ext || err != nil || !strings.Contains(string(body), spec.magic) || level != gui.ToastSuccess {
			t.Fatalf("command %s requested %q, wrote wrong format: %v (toast %v)", id, requested, err, level)
		}
	}

	pdf := menuCommandButton(buildWorkbenchSubMenu("Design", w.tabs, w.canvas), "design.export.pdf")
	if pdf == nil || !pdf.IsEnabled() {
		t.Fatal("Design menu lacks PDF")
	}
	path = filepath.Join(root, "design.svg")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pdf.Action().Trigger(pdf)
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) || level != gui.ToastWarning {
		t.Fatal("wrong extension modified an existing file")
	}
	path = filepath.Join(root, "design")
	pdf.Action().Trigger(pdf)
	after, err = os.ReadFile(path + ".pdf")
	if err != nil || !strings.HasPrefix(string(after), "%PDF-") || level != gui.ToastWarning {
		t.Fatal("extensionless selection unexpectedly targeted an unconfirmed filename")
	}
	level = gui.ToastSuccess
	path = ""
	pdf.Action().Trigger(pdf)
	if level != gui.ToastSuccess {
		t.Fatal("cancellation emitted feedback")
	}

	changeDesign = true
	path = filepath.Join(root, "stale.pdf")
	pdf.Action().Trigger(pdf)
	if _, err := os.Stat(path); !os.IsNotExist(err) || level != gui.ToastWarning {
		t.Fatal("export crossed its confirmed design context")
	}
	disabled := menuCommandButton(buildFileMenu(w.tabs, w.canvas), "file.export.pdf")
	if disabled == nil || disabled.IsEnabled() || disabled.Action().IsEnabled() {
		t.Fatal("export available outside Design mode")
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
