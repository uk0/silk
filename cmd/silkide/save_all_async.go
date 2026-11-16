package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

type saveBatchTarget struct {
	editor   *gui.CodeEditor
	path     string
	revision uint64
}
type documentSaveBatch struct {
	owner            *workbench
	targets          []saveBatchTarget
	next             int
	scene            *ged.GedScene
	designCheckpoint gui.UndoCheckpoint
	designChanged    bool
	stop             func()
	closedStop       func()
	post             func(func())
	done             func(bool)
	finished         chan struct{} // closed once after the batch's UI state retires
}

var saveBatchPost = gui.Post

func saveBatchBusy() bool { return globalWorkbench != nil && globalWorkbench.saveBatch != nil }
func saveBatchAffects(path string) bool {
	w := globalWorkbench
	if w == nil || w.saveBatch == nil {
		return false
	}
	b := w.saveBatch
	key := workspaceSearchKey(path)
	for _, t := range b.targets {
		if savePathsOverlap(key, workspaceSearchKey(t.path)) {
			return true
		}
	}
	if b.scene != nil && b.scene.Filename() != "" {
		for _, p := range designSavePaths(b.scene.Filename()) {
			if savePathsOverlap(key, workspaceSearchKey(p)) {
				return true
			}
		}
	}
	return false
}

// Save All sequences distinct documents over the SAME physical writer. It does
// not enqueue unrelated documents into a replaceable-latest slot or read disk
// to discover dirtiness on UI. Each target's revision/identity is revalidated.
func requestSaveAllDocuments(tabs *gui.TabWidget, canvas *ged.GedView, done func(bool)) bool {
	w := globalWorkbench
	if w == nil || canvas == nil || canvas.IsClosed() || w.tabs != tabs || w.canvas != canvas || saveBatchBusy() || anySaveBusy() || rejectDuringRename() {
		return false
	}
	b := &documentSaveBatch{owner: w, scene: canvas.GedScene(), post: saveBatchPost, done: done, finished: make(chan struct{})}
	w.saveBatch = b
	b.closedStop = canvas.SubscribeClosed(func() { b.finish(false) })
	total := 0
	for _, path := range orderedEditorPaths(tabs) {
		ed := openEditors[path]
		if ed == nil || !ed.Modified() {
			continue
		}
		total += len(ed.Text())
		if total > editorCloseMemoryLimit || len(b.targets) >= 4096 {
			b.finish(false)
			return true
		}
		b.targets = append(b.targets, saveBatchTarget{editor: ed, path: path, revision: ed.TextRevision()})
	}
	if b.scene != nil {
		if err := b.scene.CommitPendingEdits(); err != nil {
			reportSaveFailure(err)
			b.finish(false)
			return true
		}
		if !b.current() {
			b.finish(false)
			return true
		}
		stack, ok := b.scene.UndoStack().(*gui.UndoStack)
		if !ok {
			b.finish(false)
			return true
		}
		b.designCheckpoint = stack.Checkpoint()
		scene := b.scene
		listener := scene.AddDesignChangedCallback(func() { b.designChanged = true })
		b.stop = func() { scene.RemoveDesignChangedCallback(listener) }
	}
	b.post(b.step)
	return true
}
func (b *documentSaveBatch) current() bool {
	return b.owner != nil && globalWorkbench == b.owner && b.owner.saveBatch == b && b.owner.canvas != nil && !b.owner.canvas.IsClosed() && b.owner.canvas.GedScene() == b.scene
}
func (b *documentSaveBatch) designCurrent() bool {
	return b.scene == nil || !b.designChanged && !b.scene.HasPendingEdits() && b.designCheckpoint.IsCurrent(b.scene.UndoStack())
}
func (b *documentSaveBatch) step() {
	if !b.current() || !b.designCurrent() {
		b.finish(false)
		return
	}
	for _, t := range b.targets[:b.next] {
		if openEditors[t.path] != t.editor || t.editor.FilePath() != t.path || t.editor.Modified() {
			b.finish(false)
			return
		}
	}
	if b.next < len(b.targets) {
		t := b.targets[b.next]
		if openEditors[t.path] != t.editor || t.editor.FilePath() != t.path || t.editor.TextRevision() != t.revision {
			b.finish(false)
			return
		}
		ok, err := requestOwnedSourceSave(t.editor, t.path, func(out sourceSaveOutcome) {
			if out.Err != nil || !out.Written || !out.Adopted {
				reportSaveFailure(out.Err)
				b.finish(false)
				return
			}
			if out.FormatErr != nil {
				silkideToast(i18n.T("gofmt failed; saved unformatted"), gui.ToastWarning)
			}
			b.next++
			b.post(b.step)
		})
		if !ok {
			reportSaveFailure(err)
			b.finish(false)
		}
		return
	}
	if b.stop != nil {
		b.stop()
		b.stop = nil
	}
	if b.scene != nil && !b.scene.IsClean() {
		if !requestDesignDocumentSave(b.owner.canvas, func(ok bool) { b.complete(ok) }) {
			b.finish(false)
		}
		return
	}
	b.complete(true)
}
func (b *documentSaveBatch) complete(ok bool) {
	if !ok || !b.current() {
		b.finish(false)
		return
	}
	for _, ed := range openEditors {
		if ed != nil && ed.Modified() {
			b.finish(false)
			return
		}
	}
	if b.scene != nil && !b.scene.IsClean() {
		b.finish(false)
		return
	}
	b.finish(true)
}
func (b *documentSaveBatch) finish(ok bool) {
	if b.owner == nil {
		return
	}
	w := b.owner
	if b.stop != nil {
		b.stop()
		b.stop = nil
	}
	if b.closedStop != nil {
		b.closedStop()
		b.closedStop = nil
	}
	owned := globalWorkbench == w && w.saveBatch == b
	if w.saveBatch == b {
		w.saveBatch = nil
	}
	b.owner = nil
	b.targets = nil
	b.scene = nil
	if b.finished != nil {
		close(b.finished)
		b.finished = nil
	}
	if owned {
		if ok {
			silkideToast(i18n.T("All documents are saved"), gui.ToastSuccess)
		} else {
			silkideToast(i18n.T("Save All did not complete"), gui.ToastWarning)
		}
	}
	done := b.done
	b.done = nil
	if done != nil {
		done(ok && owned)
	}
}
