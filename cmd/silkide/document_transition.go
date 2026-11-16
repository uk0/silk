package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/internal/filetxn"
)

type transitionDisk struct {
	version core.FileFingerprint
	err     error
	dirty   bool
}
type transitionSource struct {
	editor     *gui.CodeEditor // UI only
	path, text string
	revision   uint64
}
type transitionInput struct{ path, text string } // worker only
type transitionMailbox struct {
	owner atomic.Pointer[documentTransitionController]
}
type transitionSettings struct {
	queue            *async.WriteQueue[designWriteResult]
	post, afterModal func(func())
	read             func(context.Context, string, int64) ([]byte, bool, error)
}
type documentTransitionController struct {
	owner    *workbench
	settings transitionSettings
	active   *documentTransition
	closed   bool
	stop     func()
}
type documentTransition struct {
	controller    *documentTransitionController
	id            *designSaveToken
	box           *transitionMailbox
	key           string
	scene         *ged.GedScene
	checkpoint    ged.DesignEditCheckpoint
	sources       []transitionSource
	allSources    bool
	hadModal      bool
	sourcePrompt  bool
	waitingWrites bool
	resumeSaving  bool
	initialWait   bool
	priorBatch    <-chan struct{}
	disk          []transitionDisk
	next          int
	ctx           context.Context
	cancel        context.CancelFunc
	future        *async.Future[designWriteResult]
	cancelSave    func()
	done          func(bool)
}

func newTransitionController(w *workbench, s transitionSettings) *documentTransitionController {
	if s.queue == nil {
		s.queue = designSaveWrites
	}
	if s.post == nil {
		s.post = gui.Post
	}
	if s.afterModal == nil {
		s.afterModal = gui.PostAfterNativeCleanup
	}
	if s.read == nil {
		s.read = func(ctx context.Context, path string, limit int64) ([]byte, bool, error) {
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
			before, err := filetxn.ReadLimit(path, limit)
			return before.Bytes(), before.Exists(), err
		}
	}
	c := &documentTransitionController{owner: w, settings: s}
	w.transitions = c
	c.stop = w.canvas.SubscribeClosed(c.Close)
	return c
}
func (c *documentTransitionController) valid() bool {
	return c != nil && !c.closed && c.owner != nil && globalWorkbench == c.owner && c.owner.transitions == c && c.owner.canvas != nil && !c.owner.canvas.IsClosed()
}
func transitionBusy() bool {
	w := globalWorkbench
	return w != nil && w.transitions != nil && w.transitions.active != nil
}
func transitionController(w *workbench) *documentTransitionController {
	if w == nil || w.canvas == nil {
		return nil
	}
	if w.transitions == nil {
		return newTransitionController(w, transitionSettings{})
	}
	return w.transitions
}

// A different explicit intent retires an older UI continuation, but NEVER its
// blocked physical slot. Repeated gestures while a modal is up cannot stack it.
func (c *documentTransitionController) cancelForIntent(key string) {
	if c != nil && c.active != nil && c.active.key != key && !dirtyPromptUp {
		c.active.finish(false)
	}
}
func (c *documentTransitionController) begin(key string, canvas *ged.GedView, targets []transitionSource, all bool, done func(bool)) bool {
	if !c.valid() || dirtyPromptUp {
		return false
	}
	c.cancelForIntent(key)
	if !c.valid() || c.active != nil || c.owner.designSaves != nil && c.owner.designSaves.choosing || activeFileRename != nil || activeEditorClose != nil || workspaceSearchBusy() {
		silkideToast(i18n.T("Finish the current file operation before continuing"), gui.ToastWarning)
		return false
	}
	total := 0
	for i := range targets {
		t := &targets[i]
		if t.editor == nil || openEditors[t.path] != t.editor || t.editor.FilePath() != t.path {
			return false
		}
		t.text = t.editor.Text()
		t.revision = t.editor.TextRevision()
		if len(t.text) > editorCloseMemoryLimit-total || len(targets) > 4096 {
			return false
		}
		total += len(t.text)
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &documentTransition{controller: c, id: new(designSaveToken), box: new(transitionMailbox), key: key, sources: targets, allSources: all, ctx: ctx, cancel: cancel, done: done}
	if canvas != nil {
		if canvas != c.owner.canvas {
			cancel()
			return false
		}
		j.scene = canvas.GedScene()
	}
	j.box.owner.Store(c)
	c.active = j
	if j.scene != nil {
		j.checkpoint = j.scene.EditCheckpoint()
	}
	if anySaveBusy() || saveBatchBusy() {
		j.initialWait = true
		if b := c.owner.saveBatch; b != nil {
			j.priorBatch = b.finished
		}
		j.waitForWrites(false)
		return true
	}
	j.reviewDesign()
	return true
}
func (j *documentTransition) basic() bool {
	c := j.controller
	return c != nil && c.valid() && c.active == j && (j.scene == nil || c.owner.canvas.GedScene() == j.scene)
}
func (j *documentTransition) sourcesCurrent() bool {
	if !j.basic() {
		return false
	}
	if j.allSources && len(openEditors) != len(j.sources) {
		return false
	}
	for _, t := range j.sources {
		if openEditors[t.path] != t.editor || t.editor.FilePath() != t.path || t.editor.TextRevision() != t.revision {
			return false
		}
		_, path := resolveEditorTabTarget(j.controller.owner.tabs, t.editor)
		if path != t.path {
			return false
		}
	}
	return true
}
func (j *documentTransition) current() bool {
	return j.sourcesCurrent() && (j.scene == nil || j.checkpoint.IsCurrent(j.scene))
}
func (j *documentTransition) postModal(choice gui.DialogResult, design bool) {
	box, id, post := j.box, j.id, j.controller.settings.afterModal
	post(func() {
		if c := box.owner.Load(); c != nil {
			if current := c.active; current != nil && current.id == id {
				if design {
					current.designChoice(choice)
				} else {
					current.sourceChoice(choice)
				}
			}
		}
	})
}
func (j *documentTransition) reviewDesign() {
	if !j.basic() {
		j.finish(false)
		return
	}
	if j.scene != nil && !j.checkpoint.ModelCurrent(j.scene) {
		j.finish(false)
		return
	}
	if j.scene == nil || j.scene.IsClean() {
		j.inspectSources()
		return
	}
	j.hadModal = true
	choice := askDirty(func() gui.DialogResult { return askDiscardDirtyDesign(j.controller.owner.canvas) })
	if !j.basic() {
		j.finish(false)
		return
	}
	if choice == gui.DialogCancel {
		j.finish(false)
		return
	}
	j.postModal(choice, true)
}
func (j *documentTransition) designChoice(choice gui.DialogResult) {
	if !j.sourcesCurrent() || j.scene == nil {
		j.finish(false)
		return
	}
	switch choice {
	case gui.DialogOK:
		if !j.checkpoint.ModelCurrent(j.scene) {
			j.finish(false)
			return
		}
		w := j.controller.owner
		if w.designSaves == nil {
			newOwnedDesignSaver(w, ownedDesignSaveSettings{post: j.controller.settings.post, queue: j.controller.settings.queue})
		}
		saver, scope := w.designSaves, j.id
		j.cancelSave = func() { saver.cancelScope(scope) }
		ok := requestDesignDocumentSaveScoped(w.canvas, func(saved bool) {
			j.cancelSave = nil
			if !j.basic() || !saved || !j.scene.IsClean() {
				j.finish(false)
				return
			}
			j.checkpoint = j.scene.EditCheckpoint()
			j.inspectSources()
		}, scope)
		if !ok {
			j.finish(false)
			return
		}
		if !j.basic() {
			return
		}
	case gui.DialogNo:
		if !j.checkpoint.IsCurrent(j.scene) {
			j.finish(false)
			return
		}
		j.scene.DiscardPendingEdits()
		if !j.basic() || !j.checkpoint.ModelCurrent(j.scene) {
			j.finish(false)
			return
		}
		j.checkpoint = j.scene.EditCheckpoint()
		j.inspectSources()
	default:
		j.finish(false)
	}
}
func (j *documentTransition) inspectSources() {
	if !j.current() {
		j.finish(false)
		return
	}
	if len(j.sources) == 0 {
		j.permit()
		return
	}
	inputs := make([]transitionInput, len(j.sources))
	paths := make([]string, len(j.sources))
	for i, t := range j.sources {
		inputs[i] = transitionInput{t.path, t.text}
		paths[i] = t.path
	}
	ticket, ok := reserveSavePaths(new(saveReservationOwner), paths)
	if !ok {
		j.finish(false)
		return
	}
	read, ctx, post, box, id := j.controller.settings.read, j.ctx, j.controller.settings.post, j.box, j.id
	future, err := j.controller.settings.queue.Submit(ctx, func(ctx context.Context) (designWriteResult, error) {
		result := make([]transitionDisk, len(inputs))
		remaining := int64(editorCloseMemoryLimit)
		for i, input := range inputs {
			if err := ctx.Err(); err != nil {
				return designWriteResult{}, err
			}
			data, exists, err := read(ctx, input.path, max(1, remaining))
			remaining -= int64(len(data))
			d := transitionDisk{err: err, dirty: input.text != ""}
			if err == nil {
				d.version = core.MissingFingerprint()
				if exists {
					d.version = core.FingerprintBytes(data)
					d.dirty = string(data) != input.text
				}
			}
			result[i] = d
			if remaining < 0 {
				return designWriteResult{}, fmt.Errorf("close inspection exceeds memory budget")
			}
		}
		return designWriteResult{Inspections: result}, ctx.Err()
	})
	ticket.finish(false)
	if err != nil {
		ticket.finish(true)
		reportSaveFailure(err)
		j.finish(false)
		return
	}
	j.future = future
	_ = future.OnComplete(func(f func()) { f() }, func(r designWriteResult, err error) {
		ticket.finish(true)
		if box.owner.Load() == nil {
			return
		}
		post(func() {
			if c := box.owner.Load(); c != nil {
				if current := c.active; current != nil && current.id == id {
					current.inspected(r.Inspections, err)
				}
			}
		})
	})
}
func (j *documentTransition) inspected(disk []transitionDisk, err error) {
	j.future = nil
	if !j.current() || err != nil || len(disk) != len(j.sources) {
		if err != nil && !errors.Is(err, context.Canceled) {
			reportSaveFailure(err)
		}
		j.finish(false)
		return
	}
	j.disk = append([]transitionDisk(nil), disk...)
	var dirty []string
	for i, d := range disk {
		if d.dirty {
			dirty = append(dirty, j.sources[i].path)
		}
	}
	if len(dirty) == 0 {
		j.permit()
		return
	}
	j.sourcePrompt = true
	choice := askDirty(func() gui.DialogResult {
		if !j.allSources && len(j.sources) == 1 {
			return askCloseDirtyEditor(j.sources[0].editor, j.sources[0].path)
		}
		return askQuitDirtyEditors(dirty)
	})
	j.sourcePrompt = false
	if !j.basic() {
		j.finish(false)
		return
	}
	if choice == gui.DialogCancel {
		j.finish(false)
		return
	}
	j.postModal(choice, false)
}

// A real modal loop may dispatch an explicit Save followed by Close on one of
// its listed sources. Recognize only a successfully published/observed clean
// source; never drop a changed unsaved target or untrack a replacement object.
func sourceSaveAllowedDuringPrompt(ed *gui.CodeEditor, path string) bool {
	w := globalWorkbench
	if w == nil || w.transitions == nil {
		return false
	}
	j := w.transitions.active
	if j == nil || !j.basic() || !j.allSources || !j.sourcePrompt {
		return false
	}
	for _, t := range j.sources {
		if t.editor == ed && t.path == path {
			return true
		}
	}
	return false
}
func sourceSaveTransitionScope(ed *gui.CodeEditor, path string) *designSaveToken {
	if sourceSaveAllowedDuringPrompt(ed, path) {
		return globalWorkbench.transitions.active.id
	}
	return nil
}

// A save already admitted before close may format its exact captured buffer.
// Recognize only that same revision before publication. Never rebase later
// user input, a replacement editor, or a newly admitted transition.
func initialTransitionSourcePublication(ed *gui.CodeEditor, path string) *documentTransition {
	w := globalWorkbench
	if w == nil || w.transitions == nil {
		return nil
	}
	j := w.transitions.active
	if j == nil || !j.initialWait || !j.sourcesCurrent() {
		return nil
	}
	for _, t := range j.sources {
		if t.editor == ed && t.path == path {
			return j
		}
	}
	return nil
}
func (j *documentTransition) sourcePublished(ed *gui.CodeEditor, path string) {
	if j == nil || !j.basic() || !j.initialWait || ed.Modified() {
		return
	}
	for i, t := range j.sources {
		if t.editor == ed && t.path == path && openEditors[path] == ed && ed.FilePath() == path {
			j.sources[i].text, j.sources[i].revision = ed.Text(), ed.TextRevision()
			return
		}
	}
}
func noteInitialTransitionDesignSaved(scene *ged.GedScene, path string) {
	w := globalWorkbench
	if w == nil || w.transitions == nil {
		return
	}
	j := w.transitions.active
	if j == nil || !j.initialWait || !j.basic() || j.scene != scene {
		return
	}
	if next, ok := j.checkpoint.AfterSave(scene, path); ok {
		j.checkpoint = next
	}
}
func noteTransitionSourceSaved(ed *gui.CodeEditor, path, text string, version core.FileFingerprint, scope *designSaveToken) {
	w := globalWorkbench
	if scope == nil || w == nil || w.transitions == nil || w.transitions.active == nil || w.transitions.active.id != scope || !w.transitions.active.basic() || ed.Modified() || !version.Known() {
		return
	}
	j := globalWorkbench.transitions.active
	for i, t := range j.sources {
		if t.editor == ed && t.path == path {
			j.sources[i].text = text
			j.sources[i].revision = ed.TextRevision()
			j.disk[i] = transitionDisk{version: version}
			return
		}
	}
}
func cleanSourceCloseDuringPrompt(ed *gui.CodeEditor, path string) bool {
	if !sourceSaveAllowedDuringPrompt(ed, path) || ed.Modified() {
		return false
	}
	j := globalWorkbench.transitions.active
	for i, t := range j.sources {
		if t.editor == ed && t.path == path {
			return !j.disk[i].dirty && j.disk[i].err == nil && j.disk[i].version.Known() && t.revision == ed.TextRevision() && t.text == ed.Text()
		}
	}
	return false
}
func noteTransitionSourceClosed(ed *gui.CodeEditor, path string) {
	w := globalWorkbench
	if w == nil || w.transitions == nil || w.transitions.active == nil {
		return
	}
	j := w.transitions.active
	if !j.sourcePrompt || !j.allSources {
		return
	}
	for i, t := range j.sources {
		if t.editor == ed && t.path == path {
			j.sources = append(j.sources[:i], j.sources[i+1:]...)
			j.disk = append(j.disk[:i], j.disk[i+1:]...)
			return
		}
	}
}
func (j *documentTransition) sourceChoice(choice gui.DialogResult) {
	if !j.current() {
		j.finish(false)
		return
	}
	switch choice {
	case gui.DialogNo:
		j.permit()
	case gui.DialogOK:
		j.saveNextSource()
	default:
		j.finish(false)
	}
}
func (j *documentTransition) saveNextSource() {
	if !j.current() {
		j.finish(false)
		return
	}
	if anySaveBusy() {
		j.waitForWrites(true)
		return
	}
	for j.next < len(j.sources) && !j.disk[j.next].dirty {
		j.next++
	}
	if j.next == len(j.sources) {
		j.permit()
		return
	}
	i := j.next
	t := j.sources[i]
	d := j.disk[i]
	if d.err != nil || !d.version.Known() {
		reportSaveFailure(d.err)
		j.finish(false)
		return
	}
	w := j.controller.owner
	c := w.sourceSaves
	if c == nil {
		c = newOwnedSourceSaver(w, sourceSaveSettings{post: j.controller.settings.post, queue: j.controller.settings.queue})
	}
	// Save was explicitly chosen for the observed disk snapshot. The atomic
	// writer still refuses any subsequent change, including after this dialog.
	c.bindings[t.editor] = &sourceDiskBinding{path: t.path, uiText: t.editor.SavedTextSnapshot(), state: newSaveDiskState(d.version), owner: new(saveReservationOwner)}
	ok, err := c.request(t.editor, t.path, func(out sourceSaveOutcome) {
		j.cancelSave = nil
		if !j.basic() || out.Err != nil || !out.Written || !out.Adopted || t.editor.Modified() {
			reportSaveFailure(out.Err)
			j.finish(false)
			return
		}
		j.sources[i].text = t.editor.Text()
		j.sources[i].revision = t.editor.TextRevision()
		if out.FormatErr != nil {
			silkideToast(i18n.T("gofmt failed; saved unformatted"), gui.ToastWarning)
		}
		j.next = i + 1
		j.saveNextSource()
	})
	if !ok {
		reportSaveFailure(err)
		j.finish(false)
		return
	}
	if !j.basic() {
		return
	}
	if pending := c.pending; pending != nil {
		id := pending.id
		j.cancelSave = func() {
			if c.pending != nil && c.pending.id == id {
				c.retire(async.ErrSuperseded)
			}
		}
	}
}
func (j *documentTransition) permit() {
	if !j.current() {
		j.finish(false)
		return
	}
	if anySaveBusy() {
		j.waitForWrites(false)
		return
	}
	// The no-work Open/New path is UI-only and crossed no native modal. Keep
	// it immediate without a synchronous filesystem or save fallback. Native
	// quit always posts, so its CanClose callback returns before teardown.
	if !j.hadModal && len(j.sources) == 0 && (j.key == "new" || strings.HasPrefix(j.key, "open:")) {
		j.finish(true)
		return
	}
	box, id, post := j.box, j.id, j.controller.settings.post
	post(func() {
		if c := box.owner.Load(); c != nil {
			if current := c.active; current != nil && current.id == id {
				if anySaveBusy() {
					current.waitForWrites(false)
				} else {
					current.finish(current.current())
				}
			}
		}
	})
}

func (j *documentTransition) waitForWrites(saveNext bool) {
	if saveNext {
		j.resumeSaving = true
	}
	if j.waitingWrites {
		return
	}
	j.waitingWrites = true
	ctx, box, id, post, batch := j.ctx, j.box, j.id, j.controller.settings.post, j.priorBatch
	// One owned notification waiter, no widget refs, blocked OS joins, polling
	// or queue barrier that could supersede an unrelated queued write.
	go func() {
		// A batch has UI gaps between writes. Waiting only for an empty physical
		// queue would resume teardown in that gap. The channel retains no UI refs.
		if batch != nil {
			select {
			case <-batch:
			case <-ctx.Done():
			}
		}
		err := waitSaveReservations(ctx)
		if box.owner.Load() == nil {
			return
		}
		post(func() {
			if c := box.owner.Load(); c != nil {
				if current := c.active; current != nil && current.id == id {
					current.waitingWrites = false
					if err != nil {
						current.finish(false)
					} else {
						if current.initialWait {
							current.initialWait = false
							current.priorBatch = nil
							if current.current() {
								current.reviewDesign()
							} else {
								current.finish(false)
							}
						} else if current.resumeSaving {
							current.resumeSaving = false
							current.saveNextSource()
						} else {
							current.permit()
						}
					}
				}
			}
		})
	}()
}
func (j *documentTransition) finish(ok bool) {
	c := j.controller
	if c == nil {
		return
	}
	owned := j.basic()
	if c.active == j {
		c.active = nil
	}
	j.box.owner.Store(nil)
	j.cancel()
	if j.future != nil {
		j.future.Cancel()
		j.future = nil
	}
	cancelSave := j.cancelSave
	j.cancelSave = nil
	done := j.done
	j.done = nil
	j.controller = nil
	j.scene = nil
	j.sources = nil
	j.disk = nil
	if cancelSave != nil {
		cancelSave()
	}
	if done != nil {
		done(ok && owned)
	}
}
func (c *documentTransitionController) Close() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	if c.stop != nil {
		c.stop()
		c.stop = nil
	}
	if c.active != nil {
		c.active.finish(false)
	}
	c.owner = nil
}
