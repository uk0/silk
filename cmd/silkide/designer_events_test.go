package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func designerDraftFixture(t *testing.T) (*workbench, *ged.CodePanel, *ged.FakeWidget) {
	t.Helper()
	w := testWorkbench(t)
	w.activate(modeDesign)
	fake, err := ged.NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}
	fake.SetWidgetName("button")
	fake.SetParent(w.canvas.GedScene())
	panel := ged.NewCodePanel()
	panel.BindGedView(w.canvas)
	panel.SetWidget(fake)
	t.Cleanup(func() { panel.DiscardPendingChanges(); panel.BindGedView(nil) })
	w.canvas.GedScene().SetFilename(filepath.Join(t.TempDir(), "design.silkui"))
	if !w.canvas.GedScene().Save() {
		t.Fatal("baseline save")
	}
	panel.HandlerEditor().ReplaceAllText("func onButtonClick(){println(42)}")
	return w, panel, fake
}

func TestDesignerDraftGuardCancelSaveDiscardAndFailedSave(t *testing.T) {
	for _, choice := range []gui.DialogResult{gui.DialogCancel, gui.DialogOK, gui.DialogNo} {
		t.Run(string(rune('A'+choice)), func(t *testing.T) {
			w, p, fake := designerDraftFixture(t)
			old := askDiscardDirtyDesign
			calls := 0
			askDiscardDirtyDesign = func(*ged.GedView) gui.DialogResult { calls++; return choice }
			t.Cleanup(func() { askDiscardDirtyDesign = old })
			allowed := confirmDiscardDirty(w.canvas)
			if calls != 1 || allowed != (choice != gui.DialogCancel) {
				t.Fatal("guard bypassed draft")
			}
			switch choice {
			case gui.DialogCancel:
				if !p.HasPendingChanges() || fake.GetCode() != "" {
					t.Fatal("cancel lost or committed draft")
				}
			case gui.DialogNo:
				if p.HasPendingChanges() || fake.GetCode() != "" {
					t.Fatal("discard leaked draft into model")
				}
			case gui.DialogOK:
				doc, err := core.LoadTDocFile(w.canvas.GedScene().Filename())
				if err != nil {
					t.Fatal(err)
				}
				check := ged.NewGedScene()
				_ = check.LoadDesign(doc)
				if check.Children()[0].(*ged.FakeWidget).GetCode() != p.HandlerEditor().Text() || !w.canvas.GedScene().IsClean() {
					t.Fatal("guard save persisted stale source")
				}
			}
		})
	}
	w, p, _ := designerDraftFixture(t)
	old := askDiscardDirtyDesign
	t.Cleanup(func() { askDiscardDirtyDesign = old })
	askDiscardDirtyDesign = func(*ged.GedView) gui.DialogResult { return gui.DialogOK }
	w.canvas.GedScene().SetFilename(filepath.Join(t.TempDir(), "missing", "design.silkui"))
	if confirmDiscardDirty(w.canvas) || w.canvas.GedScene().IsClean() || !strings.Contains(p.HandlerEditor().Text(), "42") {
		t.Fatal("failed write allowed transition or lost draft")
	}
}

func TestDesignerDraftSaveAllAndHotReloadGuard(t *testing.T) {
	w, p, fake := designerDraftFixture(t)
	s := w.canvas.GedScene()
	writeRecoveryDesign(t, s.Filename(), "External replacement", "external")
	if changed, err := reloadCleanDesign(s, s.Filename()); err != nil || changed || s.Children()[0] != fake {
		t.Fatal("hot reload replaced draft owner", err)
	}
	if !saveAllDocuments(w.tabs, w.canvas) || !s.IsClean() || p.HasPendingChanges() {
		t.Fatal("Save All skipped code-only edit")
	}
	bytes, err := os.ReadFile(s.Filename())
	if err != nil || !strings.Contains(string(bytes), "println(42)") {
		t.Fatal("Save All lost draft", err)
	}
}

func TestDesignerEventMenuRoutesIntoSingleEditablePanel(t *testing.T) {
	w := testWorkbench(t)
	w.activate(modeDesign)
	globalBottomDock = w.center.SplitNewDock(false, true).(*gui.Dock)
	fake, err := ged.NewFakeWidgetFromFactory("gui.SearchBox")
	if err != nil {
		t.Fatal(err)
	}
	fake.SetWidgetName("search")
	fake.SetParent(w.canvas.GedScene())
	w.canvas.Selection().Add(fake)
	wireDesignerContextPanels(w.canvas)
	m := buildWorkbenchSubMenu("Design", w.tabs, w.canvas)
	var eventMenu *gui.Menu
	for _, child := range m.Items() {
		if b, ok := child.(*gui.Button); ok {
			if sub, ok := b.SubPopup().(*gui.Menu); ok && menuCommandButton(sub, "event.handler.OnSearch") != nil {
				eventMenu = sub
			}
		}
	}
	if eventMenu == nil {
		t.Fatal("Design menu missing actual widget events")
	}
	for _, id := range []string{"event.handler.OnSearch", "event.handler.OnTextChanged", "event.handler.OnSearch"} {
		b := menuCommandButton(eventMenu, id)
		if b == nil || b.Icon() == nil || !b.Action().IsEnabled() {
			t.Fatal("event action missing icon or disabled")
		}
		b.Action().Trigger(b)
	}
	if len(fake.EventHandlers()) != 2 || strings.Count(fake.GetCode(), "func ") != 2 {
		t.Fatal("menu duplicated or dropped handlers")
	}
	// No duplicate code panel on repeated requests; it is the active bottom view.
	panel, ok := globalBottomDock.ActiveView().(*ged.CodePanel)
	if !ok {
		t.Fatal("event action did not activate editable panel")
	}
	defer panel.BindGedView(nil)
	if panel.HandlerEditor().CursorLine() != 0 {
		t.Fatal("repeat navigation not at first handler")
	}
	w.activate(modeEdit)
	for _, child := range buildWorkbenchSubMenu("Design", w.tabs, w.canvas).Items() {
		if b, ok := child.(*gui.Button); ok {
			if sub, ok := b.SubPopup().(*gui.Menu); ok && menuCommandButton(sub, "event.handler.OnSearch") != nil {
				t.Fatal("hidden design exposed event mutation")
			}
		}
	}
}

func TestDesignerSaveKeepsHandlerFocusAndBuildDiagnostics(t *testing.T) {
	w, p, _ := designerDraftFixture(t)
	oldOutput, oldProblems, oldBuild := globalBuildOutput, globalProblems, statusBarBuildLabel
	t.Cleanup(func() { globalBuildOutput, globalProblems, statusBarBuildLabel = oldOutput, oldProblems, oldBuild })
	globalBottomDock = w.center.SplitNewDock(false, true).(*gui.Dock)
	globalBuildOutput = ged.NewBuildOutput()
	globalBottomDock.AddView(globalBuildOutput)
	globalBottomDock.AddView(p)
	globalBottomDock.SetActiveIndex(globalBottomDock.IndexOfView(p))
	globalBuildOutput.SetOutput("main.go:3:1: existing build error")
	statusBarBuildLabel = gui.NewLabel("build: 1 errors")
	if !saveDesignDocument(w.canvas) {
		t.Fatal("save failed")
	}
	if globalBottomDock.ActiveView() != p || !globalBuildOutput.HasErrors() || statusBarBuildLabel.Text() != "build: 1 errors" {
		t.Fatal("generation stole panel focus or erased build result")
	}
}
