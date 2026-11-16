package main

import (
	"errors"
	"path/filepath"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

var chooseDesignSavePath = gui.SaveFileDialog
var afterDesignSaveChooser = gui.PostAfterNativeCleanup

// requestCurrentDocumentSave is the actual toolbar/File/shortcut admission
// route. A true return means accepted work, NOT permission to quit/open/New.
// Destructive continuations must use done and revalidate their own intent.
func requestCurrentDocumentSave(tabs *gui.TabWidget, canvas *ged.GedView, done func(bool)) bool {
	if transitionBusy() && !(documentMode(tabs, canvas) == modeEdit && sourceSaveAllowedDuringPrompt(activeEditor(tabs), activeEditorPath(tabs))) {
		silkideToast(i18n.T("Finish the current document operation before saving"), gui.ToastWarning)
		return false
	}
	if saveBatchBusy() {
		silkideToast(i18n.T("Save All is still running"), gui.ToastWarning)
		return false
	}
	switch documentMode(tabs, canvas) {
	case modeDesign:
		return requestDesignDocumentSave(canvas, done)
	case modeEdit:
		ed := activeEditor(tabs)
		if ed == nil {
			return false
		}
		_, path := resolveEditorTabTarget(tabs, ed)
		return requestSourceDocumentSave(ed, path, done)
	}
	return false
}

func reportSaveFailure(err error) {
	if err == nil || errors.Is(err, async.ErrSuperseded) || errors.Is(err, errSaveOwnerClosed) {
		return
	}
	silkideToast(i18n.Tf("Save failed: %v", err), gui.ToastError)
}

func requestSourceDocumentSave(ed *gui.CodeEditor, path string, done func(bool)) bool {
	if transitionBusy() && !sourceSaveAllowedDuringPrompt(ed, path) || saveBatchBusy() {
		return false
	}
	ok, err := requestOwnedSourceSave(ed, path, func(out sourceSaveOutcome) {
		success := out.Err == nil && out.Written
		if !success {
			reportSaveFailure(out.Err)
		} else {
			if out.FormatErr != nil {
				silkideToast(i18n.T("gofmt failed; saved unformatted"), gui.ToastWarning)
			}
			silkideToast(i18n.Tf("Saved %s", filepath.Base(out.Path)), gui.ToastSuccess)
			refreshGitChanges(globalCanvas)
		}
		if done != nil {
			done(success && out.Adopted)
		}
	})
	if err != nil {
		reportSaveFailure(err)
	}
	return ok
}

func requestDesignDocumentSave(canvas *ged.GedView, done func(bool)) bool {
	return requestDesignDocumentSaveScoped(canvas, done, nil)
}
func requestDesignDocumentSaveScoped(canvas *ged.GedView, done func(bool), scope *designSaveToken) bool {
	w := globalWorkbench
	if w == nil || w.canvas != canvas || canvas == nil || canvas.IsClosed() || canvas.GedScene() == nil {
		return false
	}
	c := w.designSaves
	if c == nil {
		c = newOwnedDesignSaver(w, ownedDesignSaveSettings{})
	}
	if c.choosing || c.review != nil {
		return false
	}
	scene := canvas.GedScene()
	previous := scene.Filename()
	run := func(path string) bool {
		ok, err := c.requestScoped(path, func(out designSaveOutcome) {
			success := out.Err == nil && out.Written
			if !success {
				reportSaveFailure(out.Err)
			} else {
				if out.CompanionErr != nil {
					logEvent(ged.LogWarn, "Companion: "+out.CompanionErr.Error())
				}
				if out.RecoveryErr != nil {
					logEvent(ged.LogWarn, "Recovery cleanup: "+out.RecoveryErr.Error())
				}
				silkideToast(i18n.Tf("Saved %s", filepath.Base(out.Path)), gui.ToastSuccess)
			}
			if done != nil {
				done(success && out.Adopted)
			}
		}, scope)
		if err != nil {
			reportSaveFailure(err)
		}
		return ok
	}
	if previous != "" {
		return run(previous)
	}
	if anySaveBusy() {
		rejectDuringRename()
		return false
	}
	c.choosing = true
	c.chooserScope = scope
	id := new(designSaveToken)
	c.intent = id
	path := chooseDesignSavePath()
	if path == "" {
		c.choosing = false
		c.chooserScope = nil
		return false
	}
	// Do not capture drafts until native cleanup/focus restoration has finished.
	afterDesignSaveChooser(func() {
		c.choosing = false
		c.chooserScope = nil
		if !c.valid() || c.intent != id || canvas.GedScene() != scene || scene.Filename() != previous {
			if done != nil {
				done(false)
			}
			return
		}
		if !c.reviewSaveAs(scene, path, id, scope, run, done) && done != nil {
			done(false)
		}
	})
	return true
}
