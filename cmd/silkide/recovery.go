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
	doc, err := core.LoadTDocFile(sourcePath)
	if err != nil {
		return err
	}
	return applyDesignDocument(scene, doc, originalPath, recoveryPath != "")
}

// applyDesignDocument is the UI-only half of opening a captured document.
func applyDesignDocument(scene *ged.GedScene, doc *core.TDoc, originalPath string, recovered bool) error {
	if scene == nil || doc == nil || originalPath == "" {
		return errors.New("invalid captured design")
	}
	if err := scene.LoadDesign(doc); err != nil {
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
	if recovered {
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

// reloadCleanDesign is the UI-only, I/O-free install of an owned prepared
// snapshot. The caller must also validate admission/edit/lifetime state; a
// subscription or a currently clean scene is not sufficient authority alone.
func reloadCleanDesign(scene *ged.GedScene, changedPath string, disk *core.TDoc) (bool, error) {
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
	var kind string
	if disk == nil || disk.Value(&kind) != nil || kind != "form" {
		return false, errors.New("reload: expected a prepared silkui form document")
	}
	if disk.String() == scene.SaveDesign().String() {
		return false, nil
	}
	if err := applyDesignDocument(scene, disk, originalPath, false); err != nil {
		return false, err
	}
	return true, nil
}
