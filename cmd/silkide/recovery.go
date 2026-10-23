package main

import (
	"errors"
	"path/filepath"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

// openDesignDocument separates the document's identity from its recovery
// source. An autosave is pending content for originalPath, never a new document:
// Save, session capture, the window title and the next autosave must continue to
// address the original .silkui. Neither input file is written or deleted here.
func openDesignDocument(scene *ged.GedScene, originalPath, recoveryPath string) error {
	if scene == nil || originalPath == "" {
		return errors.New("cannot open a design without a scene and original path")
	}
	sourcePath := originalPath
	if recoveryPath != "" {
		sourcePath = recoveryPath
	}
	if err := scene.OpenFile(sourcePath); err != nil {
		return err
	}
	scene.SetFilename(originalPath)
	scene.SetTitle(filepath.Base(originalPath))

	// A new document cannot undo commands against the previous document's
	// detached widgets. SetClean before Clear also resets a previously dirty
	// stack; UndoStack.Clear deliberately retains an existing dirty state.
	stack := scene.UndoStack()
	stack.SetClean()
	stack.Clear()
	if recoveryPath != "" {
		// Recovery has no reconstructible edit history. Create a dirty position
		// then discard its empty marker: Clear retains dirty without advertising
		// a fake Undo that would mark recovered content saved without changing it.
		// A successful ordinary Save is what establishes the next clean point.
		stack.Push(gui.NewCompoundCommand())
		stack.Clear()
	}
	scene.NotifyDesignChanged()
	scene.Update()
	return nil
}

// reloadCleanDesign must run on the UI thread: queued watcher events can arrive
// after the canvas has switched documents or acquired unsaved edits. A watcher
// subscription is not permission to overwrite whichever document is open now.
// False with no error means the event was safely ignored.
func reloadCleanDesign(scene *ged.GedScene, changedPath string) (bool, error) {
	if scene == nil || scene.Filename() == "" || changedPath == "" {
		return false, nil
	}
	originalPath := scene.Filename()
	current, err := filepath.Abs(originalPath)
	if err != nil {
		return false, err
	}
	changed, err := filepath.Abs(changedPath)
	if err != nil {
		return false, err
	}
	if current != changed || !scene.IsClean() {
		return false, nil
	}
	// Saving this very scene also emits a watcher event. Compare canonical
	// documents instead of timestamps: a no-op reload would replace widget
	// identities and discard the still-valid history of the just-saved edit.
	disk, err := core.LoadTDocFile(originalPath)
	if err != nil {
		return false, err
	}
	if disk.String() == scene.SaveDesign().String() {
		return false, nil
	}
	if err := openDesignDocument(scene, originalPath, ""); err != nil {
		return false, err
	}
	return true, nil
}
