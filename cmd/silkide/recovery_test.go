package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func writeRecoveryDesign(t *testing.T, path, title, buttonText string) {
	t.Helper()
	scene := ged.NewGedScene()
	scene.SetFormTitle(title)
	button, err := ged.NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}
	button.SetWidgetName("submit")
	button.SetBounds(10, 15, 35, 10)
	button.Widget().(*gui.Button).SetText(buttonText)
	button.SetParent(scene)
	if err := scene.SaveDesign().SaveFile(path); err != nil {
		t.Fatal(err)
	}
}

func recoveryButtonText(t *testing.T, scene *ged.GedScene) string {
	t.Helper()
	if len(scene.Children()) != 1 {
		t.Fatalf("recovery lost widgets: %d", len(scene.Children()))
	}
	return scene.Children()[0].(*ged.FakeWidget).Widget().(*gui.Button).Text()
}

func TestRecoveryKeepsOriginalIdentityDirtyStateAndSaveDestination(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "design.silkui")
	recovery := original + ".autosave"
	writeRecoveryDesign(t, original, "Saved Form", "Saved")
	writeRecoveryDesign(t, recovery, "Recovered Form", "Recovered")
	originalBytes := diskText(t, original)
	recoveryBytes := diskText(t, recovery)

	canvas := ged.NewGedView()
	scene := canvas.GedScene()
	notifications := 0
	scene.AddDesignChangedCallback(func() {
		notifications++
		if scene.Filename() != original || scene.IsClean() {
			t.Error("listeners observed recovery-source identity or a falsely clean design")
		}
	})
	if err := openDesignDocument(scene, original, recovery); err != nil {
		t.Fatal(err)
	}
	if scene.Filename() != original || scene.Title() != "design.silkui *" {
		t.Fatalf("recovery identity drift: file=%q title=%q", scene.Filename(), scene.Title())
	}
	if scene.FormTitle() != "Recovered Form" || recoveryButtonText(t, scene) != "Recovered" {
		t.Fatal("original content loaded instead of recovered form and widgets")
	}
	stack := scene.UndoStack()
	if scene.IsClean() || stack.Count() != 0 || stack.CanUndo() || stack.CanRedo() {
		t.Fatal("recovery must be dirty without inventing an undo history")
	}
	stack.Undo()
	stack.Redo()
	if scene.IsClean() || recoveryButtonText(t, scene) != "Recovered" {
		t.Fatal("no-op Undo/Redo silently marked recovered work saved")
	}
	if notifications != 1 {
		t.Fatalf("design changed notifications = %d", notifications)
	}
	if state := captureWorkspaceSession(nil, canvas); state.Design != original {
		t.Fatalf("session persisted recovery source: %q", state.Design)
	}
	if diskText(t, original) != originalBytes || diskText(t, recovery) != recoveryBytes {
		t.Fatal("opening recovery rewrote source files before an explicit Save")
	}

	if !scene.Save() {
		t.Fatal("save recovered design failed")
	}
	if !scene.IsClean() || scene.Title() != "design.silkui" {
		t.Fatal("successful save did not clear dirty title")
	}
	if diskText(t, original) != recoveryBytes {
		t.Fatal("save did not write recovered content back to original .silkui")
	}
	if diskText(t, recovery) != recoveryBytes {
		t.Fatal("save modified/deleted the user's recovery copy")
	}
	if _, err := os.Stat(recovery + ".autosave"); !os.IsNotExist(err) {
		t.Fatal("unexpected nested autosave identity")
	}
}

func TestRecoveryCanRestoreAMissingOriginalWithoutCreatingItUntilSave(t *testing.T) {
	original := filepath.Join(t.TempDir(), "missing.silkui")
	recovery := original + ".autosave"
	writeRecoveryDesign(t, recovery, "Recovered", "Keep me")
	scene := ged.NewGedScene()
	if err := openDesignDocument(scene, original, recovery); err != nil {
		t.Fatal(err)
	}
	if scene.Filename() != original || scene.IsClean() {
		t.Fatal("missing-original recovery lost identity or pending changes")
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatal("recovery created original before explicit Save")
	}
	if !scene.Save() || !strings.Contains(diskText(t, original), "Keep me") {
		t.Fatal("recovered original was not recreated by Save")
	}
}

func TestNormalDesignOpenResetsOldHistoryWithoutRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "normal.silkui")
	writeRecoveryDesign(t, path, "Normal", "Saved")
	scene := ged.NewGedScene()
	scene.PushCommand(gui.NewCompoundCommand())
	if err := openDesignDocument(scene, path, ""); err != nil {
		t.Fatal(err)
	}
	if !scene.IsClean() || scene.UndoStack().Count() != 0 || scene.Title() != "normal.silkui" {
		t.Fatal("normal open retained dirty state/history from the old document")
	}
	if recoveryButtonText(t, scene) != "Saved" {
		t.Fatal("normal open changed saved content")
	}
}

func TestRecoveryReadFailureLeavesCurrentDesignAndHistoryUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.silkui")
	writeRecoveryDesign(t, path, "Current", "Current")
	scene := ged.NewGedScene()
	if err := openDesignDocument(scene, path, ""); err != nil {
		t.Fatal(err)
	}
	scene.PushCommand(gui.NewCompoundCommand())
	beforeTitle := scene.Title()
	if err := openDesignDocument(scene, filepath.Join(t.TempDir(), "next.silkui"), "missing-recovery-source"); err == nil {
		t.Fatal("missing recovery source was accepted")
	}
	if scene.Filename() != path || scene.Title() != beforeTitle || scene.IsClean() || scene.UndoStack().Count() != 1 || recoveryButtonText(t, scene) != "Current" {
		t.Fatal("failed recovery changed the currently open design")
	}
	if err := openDesignDocument(nil, path, ""); err == nil {
		t.Fatal("nil scene accepted")
	}
	if err := openDesignDocument(scene, "", ""); err == nil {
		t.Fatal("empty document identity accepted")
	}
}

func TestReloadIgnoresAnOldWatchAfterSwitchingDocuments(t *testing.T) {
	dir := t.TempDir()
	pathA, pathB := filepath.Join(dir, "A.silkui"), filepath.Join(dir, "B.silkui")
	writeRecoveryDesign(t, pathA, "A", "A before")
	writeRecoveryDesign(t, pathB, "B", "B current")
	scene := ged.NewGedScene()
	for _, path := range []string{pathA, pathB} {
		if err := openDesignDocument(scene, path, ""); err != nil {
			t.Fatal(err)
		}
	}
	writeRecoveryDesign(t, pathA, "A external", "A after")
	if applied, err := reloadCleanDesign(scene, pathA); err != nil || applied {
		t.Fatalf("stale A watch should be ignored after A -> B: applied=%v err=%v", applied, err)
	}
	if scene.Filename() != pathB || !scene.IsClean() || scene.Title() != "B.silkui" || recoveryButtonText(t, scene) != "B current" {
		t.Fatal("old watch replaced the current B document")
	}
}

func TestReloadProtectsDirtyRecoveredAndEditedDesigns(t *testing.T) {
	for _, recovered := range []bool{false, true} {
		t.Run(map[bool]string{false: "edited", true: "recovered"}[recovered], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "current.silkui")
			writeRecoveryDesign(t, path, "Saved", "Saved")
			recoveryPath := ""
			if recovered {
				recoveryPath = path + ".autosave"
				writeRecoveryDesign(t, recoveryPath, "Recovered", "Pending recovery")
			}
			scene := ged.NewGedScene()
			if err := openDesignDocument(scene, path, recoveryPath); err != nil {
				t.Fatal(err)
			}
			if !recovered {
				scene.PushCommand(gui.NewCompoundCommand())
			}
			before := scene.SaveDesign().String()
			count := scene.UndoStack().Count()
			writeRecoveryDesign(t, path, "External", "Do not replace my changes")
			if applied, err := reloadCleanDesign(scene, path); err != nil || applied {
				t.Fatalf("dirty document reloaded: applied=%v err=%v", applied, err)
			}
			if scene.SaveDesign().String() != before || scene.IsClean() || scene.UndoStack().Count() != count || scene.Filename() != path {
				t.Fatal("hot reload overwrote pending content or edit history")
			}
		})
	}
}

func TestReloadCleanCurrentDesignPreservesIdentityAndResetsHistory(t *testing.T) {
	dir := t.TempDir()
	// Windows runners can check out on D: while TempDir lives on C:.
	// A relative spelling only exists on the same volume.
	t.Chdir(dir)
	path := filepath.Join(dir, "current.silkui")
	writeRecoveryDesign(t, path, "Saved", "Before")
	scene := ged.NewGedScene()
	if err := openDesignDocument(scene, path, ""); err != nil {
		t.Fatal(err)
	}
	// A saved scene may still have undo history against its old widgets.
	scene.PushCommand(gui.NewCompoundCommand())
	scene.UndoStack().SetClean()
	writeRecoveryDesign(t, path, "External", "After")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(wd, path)
	if err != nil {
		t.Fatal(err)
	}
	if applied, err := reloadCleanDesign(scene, relative); err != nil || !applied {
		t.Fatalf("same clean document not reloaded across relative/absolute spellings: applied=%v err=%v", applied, err)
	}
	if scene.Filename() != path || scene.Title() != "current.silkui" || scene.FormTitle() != "External" || recoveryButtonText(t, scene) != "After" || !scene.IsClean() || scene.UndoStack().Count() != 0 {
		t.Fatal("clean reload lost identity, new contents or clean history")
	}
}

func TestReloadReadFailurePreservesCleanCurrentDesign(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.silkui")
	writeRecoveryDesign(t, path, "Saved", "Keep current view")
	scene := ged.NewGedScene()
	if err := openDesignDocument(scene, path, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if applied, err := reloadCleanDesign(scene, path); err == nil || applied {
		t.Fatalf("missing file reported as reloaded: applied=%v err=%v", applied, err)
	}
	if scene.Filename() != path || !scene.IsClean() || recoveryButtonText(t, scene) != "Keep current view" {
		t.Fatal("failed reload damaged existing design")
	}
	if applied, err := reloadCleanDesign(nil, path); applied || err != nil {
		t.Fatal("nil scene should safely ignore reload")
	}
	if applied, err := reloadCleanDesign(scene, ""); applied || err != nil {
		t.Fatal("empty event path should safely ignore reload")
	}
}

type recoveryTitleEdit struct {
	scene        *ged.GedScene
	before, next string
}

func (edit recoveryTitleEdit) Redo()        { edit.scene.SetFormTitle(edit.next) }
func (edit recoveryTitleEdit) Undo()        { edit.scene.SetFormTitle(edit.before) }
func (edit recoveryTitleEdit) Text() string { return "Edit form title" }

func TestReloadAfterOwnSavePreservesWidgetIdentityAndUndo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.silkui")
	writeRecoveryDesign(t, path, "Before edit", "Do not recreate me")
	scene := ged.NewGedScene()
	if err := openDesignDocument(scene, path, ""); err != nil {
		t.Fatal(err)
	}
	originalWidget := scene.Children()[0]
	scene.PushCommand(recoveryTitleEdit{scene, scene.FormTitle(), "Saved edit"})
	if !scene.Save() || !scene.UndoStack().CanUndo() {
		t.Fatal("test requires a just-saved document with valid undo history")
	}
	// Replay the file notification produced by that explicit Save.
	if applied, err := reloadCleanDesign(scene, path); err != nil || applied {
		t.Fatalf("identical own-save event must be a no-op: applied=%v err=%v", applied, err)
	}
	if !scene.IsClean() || !scene.UndoStack().CanUndo() || scene.UndoStack().Count() != 1 || scene.Children()[0] != originalWidget {
		t.Fatal("own-save reload cleared history or replaced widget identity")
	}
	scene.UndoStack().Undo()
	if scene.FormTitle() != "Before edit" || scene.IsClean() || scene.Children()[0] != originalWidget {
		t.Fatal("saved edit can no longer be undone against the original widgets")
	}
}
