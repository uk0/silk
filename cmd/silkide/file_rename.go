package main

import (
	"context"
	"path/filepath"
	"sort"

	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/internal/fileops"
)

// All reservation/commit state is UI-thread-owned. The explorer owns the one
// filesystem worker. Keep affected editors alive and editable while disk I/O is
// pending, but never allow a save/open/close to race the path identity change.
type editorPathMove struct {
	old, next string
	editor    *gui.CodeEditor
}
type editorRename struct {
	old, next string
	moves     []editorPathMove
	previous  <-chan struct{}
}

var activeFileRename *editorRename

type renameGitSuspension interface {
	Wait(context.Context) error
	Release()
}

// Narrow deterministic seam: production always reserves the shared SDK service.
var suspendRenameGit = func(path string) (renameGitSuspension, error) {
	return gui.SuspendEditorGitPath(path)
}

func fileRenameAffects(path string) bool {
	if activeFileRename == nil {
		return false
	}
	_, old := fileops.RebasePath(path, activeFileRename.old, activeFileRename.next)
	_, next := fileops.RebasePath(path, activeFileRename.next, activeFileRename.old)
	return old || next
}
func renameBusyError(path, message string) error {
	return diagnostic.New(diagnostic.FileOperationBusy, message, nil).At(diagnostic.Location{File: path})
}
func rejectBusyFile(path string) bool {
	if !fileRenameAffects(path) {
		return false
	}
	err := renameBusyError(path, "rename is still running; retry after completion")
	logEvent(ged.LogWarn, err.Error()+"\n"+diagnostic.New(diagnostic.FileOperationBusy, "", nil).Hint)
	silkideToast(err.Error(), gui.ToastWarning)
	return true
}
func rejectDuringRename() bool {
	if activeFileRename == nil {
		return false
	}
	return rejectBusyFile(activeFileRename.old)
}

// prepareEditorRename is installed on the actual F2/context-menu explorer
// path, not a second implementation of rename. Dirty buffers are NOT saved,
// closed or recreated. Completion reads their latest contents, including edits
// made while the filesystem worker was running.
func prepareEditorRename(tabs *gui.TabWidget, canvas *ged.GedView, oldPath, newPath string) (ged.RenamePreparation, error) {
	if activeFileRename != nil || macroBusy || globalDebug != nil || globalDebugAttempt != nil {
		return ged.RenamePreparation{}, renameBusyError(oldPath, "finish the current file operation, macro expansion or debug session before renaming")
	}
	// A designer document also owns autosave/reload/companion-generation paths.
	// Until those are migrated transactionally, reject before touching disk.
	if canvas != nil && canvas.GedScene() != nil {
		if _, ok := fileops.RebasePath(canvas.GedScene().Filename(), oldPath, newPath); ok {
			return ged.RenamePreparation{}, renameBusyError(oldPath, "close the affected design document before renaming; its autosave and reload paths are still active")
		}
	}
	tx := &editorRename{old: oldPath, next: newPath}
	for path, ed := range openEditors {
		if next, ok := fileops.RebasePath(path, oldPath, newPath); ok && ed != nil {
			tx.moves = append(tx.moves, editorPathMove{path, next, ed})
		}
	}
	sort.Slice(tx.moves, func(i, j int) bool { return tx.moves[i].old < tx.moves[j].old })
	for _, move := range tx.moves {
		for path, other := range openEditors {
			if other == nil || other == move.editor {
				continue
			}
			if mapped, ok := fileops.RebasePath(path, move.next, move.next); ok && mapped == move.next {
				return ged.RenamePreparation{}, diagnostic.New(diagnostic.FileConflict, "destination has another open editor", nil).At(diagnostic.Location{File: move.next})
			}
		}
	}
	git, err := suspendRenameGit(oldPath)
	if err != nil {
		return ged.RenamePreparation{}, err
	}
	activeFileRename = tx
	// Retire old providers before dispatch. Their queued deliveries cannot restore
	// old diagnostics/completions while the editor remains freely editable.
	var previous []<-chan struct{}
	for _, move := range tx.moves {
		move.editor.CancelGitStatus()
		if doc := lspDocuments[move.old]; doc != nil {
			previous = append(previous, doc.done)
		}
		lspDidCloseFile(move.old)
		delete(lspDocuments, move.old) // reject already-queued callbacks immediately
		move.editor.ClearExternalCompletions()
		move.editor.SetOccurrenceHighlights(nil)
	}
	if len(previous) > 0 {
		done := make(chan struct{})
		tx.previous = done
		go func() {
			for _, ch := range previous {
				<-ch
			}
			close(done)
		}()
	}
	return ged.RenamePreparation{Wait: git.Wait, Complete: func(err error) {
		if activeFileRename != tx {
			return
		} // completion is idempotent
		defer git.Release() // only after the UI owns the final path identity
		if err == nil {
			for _, move := range tx.moves {
				delete(openEditors, move.old)
			}
			for _, move := range tx.moves {
				openEditors[move.next] = move.editor
				delete(editorGitManualRefresh, move.editor)
				move.editor.SetFilePath(move.next)
				move.editor.SetErrors(nil)
				move.editor.SetSyntaxHighlights(move.editor.TextRevision(), nil)
				move.editor.SetAdditionalSymbols(move.editor.TextRevision(), nil)
				// Coverage belongs to the pre-rename source/profile. Do not display it as
				// evidence of coverage for a different path/package.
				move.editor.ClearCoverage()
				refreshEditorTab(tabs, move.editor, move.next)
			}
			migrateRenamedMetadata(tx, tabs, canvas)
		}
		// Release before restarting providers; helpers reject reserved paths.
		activeFileRename = nil
		for _, move := range tx.moves {
			path := move.old
			if err == nil {
				path = move.next
			} else {
				move.editor.RefreshGitStatus() // queued behind the still-held gate
			}
			openLSPDocumentAfter(path, move.editor.Text(), tx.previous)
			updateMacroEditor(tabs, path, move.editor)
		}
		syncOutlineToActiveEditor(tabs)
		if err == nil {
			logEvent(ged.LogInfo, "Renamed "+oldPath+" -> "+newPath)
		}
	}}, nil
}

func migrateRenamedMetadata(tx *editorRename, tabs *gui.TabWidget, canvas *ged.GedView) {
	remap := func(p string) string { next, _ := fileops.RebasePath(p, tx.old, tx.next); return next }
	if globalProblems != nil {
		rows := globalProblems.Problems()
		for i := range rows {
			rows[i].File = remap(rows[i].File)
			rows[i].Source.Doc = remap(rows[i].Source.Doc)
		}
		globalProblems.SetProblems(rows)
	}
	if globalReferencesPanel != nil {
		rows := globalReferencesPanel.Locations()
		for i := range rows {
			rows[i].File = remap(rows[i].File)
		}
		globalReferencesPanel.SetLocations(rows)
	}
	if globalBookmarks != nil {
		for _, mark := range globalBookmarks.Bookmarks() {
			next := remap(mark.File)
			if next != mark.File {
				globalBookmarks.Remove(mark.File, mark.Line)
				globalBookmarks.Add(next, mark.Line, mark.Label)
			}
		}
	}
	for key := range globalCoverage {
		if _, ok := fileops.RebasePath(filepath.FromSlash(key), tx.old, tx.next); ok {
			delete(globalCoverage, key)
			continue
		}
		oldKey := coverageKeyForPath(filepath.ToSlash(tx.old), globalCoverageModule, filepath.ToSlash(globalCoverageRoot))
		if oldKey != "" {
			if _, ok := fileops.RebasePath(filepath.FromSlash(key), filepath.FromSlash(oldKey), filepath.FromSlash(oldKey)); ok {
				delete(globalCoverage, key)
			}
		}
	}
	if globalPrefs != nil {
		recent := globalPrefs.store.StringList("files/recent", nil)
		seen := map[string]bool{}
		out := make([]string, 0, len(recent))
		for _, p := range recent {
			p = remap(p)
			if !seen[p] {
				out = append(out, p)
				seen[p] = true
			}
		}
		_ = globalPrefs.store.SetStringList("files/recent", out)
		_ = globalPrefs.store.SetString("files/lastDir", remap(globalPrefs.LastOpenedDir()))
		globalPrefs.SetWorkspaceSession(captureWorkspaceSession(tabs, canvas))
		if w := globalWorkbench; w != nil && w.welcome != nil {
			w.welcome.SetRecentFiles(out)
		}
	}
}
