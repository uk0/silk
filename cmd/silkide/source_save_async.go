package main

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/internal/fileops"
)

type sourceSaveOutcome struct {
	Path             string
	Written, Adopted bool
	Err, FormatErr   error
}
type sourceSaveMailbox struct {
	owner atomic.Pointer[ownedSourceSaver]
}
type sourceDiskBinding struct {
	state        *saveDiskState
	owner        *saveReservationOwner
	path, uiText string
}
type sourceSaveJob struct {
	id              *designSaveToken
	transitionScope *designSaveToken
	box             *sourceSaveMailbox
	editor          *gui.CodeEditor
	path, text      string
	revision        uint64
	future          *async.Future[designWriteResult]
	reservation     *saveReservation
	done            func(sourceSaveOutcome)
}
type sourceSaveSettings struct {
	queue  *async.WriteQueue[designWriteResult]
	post   func(func())
	format func(context.Context, string) (string, error)
	write  func(string, []byte, core.FileFingerprint) (core.FileFingerprint, error)
}
type ownedSourceSaver struct {
	owner    *workbench
	settings sourceSaveSettings
	pending  *sourceSaveJob
	intent   *designSaveToken
	bindings map[*gui.CodeEditor]*sourceDiskBinding
	closed   bool
	stop     func()
}

func newOwnedSourceSaver(w *workbench, settings sourceSaveSettings) *ownedSourceSaver {
	if settings.queue == nil {
		settings.queue = designSaveWrites
	}
	if settings.post == nil {
		settings.post = gui.Post
	}
	if settings.format == nil {
		settings.format = formatClosingEditor
	}
	if settings.write == nil {
		settings.write = func(path string, data []byte, expected core.FileFingerprint) (core.FileFingerprint, error) {
			return fileops.WriteAtomicAgainst(path, 0644, expected, func(out io.Writer) error { _, err := out.Write(data); return err })
		}
	}
	c := &ownedSourceSaver{owner: w, settings: settings, bindings: make(map[*gui.CodeEditor]*sourceDiskBinding)}
	w.sourceSaves = c
	c.stop = w.canvas.SubscribeClosed(c.Close)
	return c
}
func (c *ownedSourceSaver) valid() bool {
	return c != nil && !c.closed && c.owner != nil && globalWorkbench == c.owner && c.owner.sourceSaves == c && c.owner.canvas != nil && !c.owner.canvas.IsClosed()
}
func (c *ownedSourceSaver) owns(ed *gui.CodeEditor, path string) bool {
	if !c.valid() || ed == nil || openEditors[path] != ed || ed.FilePath() != path {
		return false
	}
	_, current := resolveEditorTabTarget(c.owner.tabs, ed)
	return current == path
}

// Admission is not durable success. The worker owns strings, flags, the pure
// physical baseline and revocable mailbox only; no editor/workbench references.
func (c *ownedSourceSaver) request(ed *gui.CodeEditor, path string, done func(sourceSaveOutcome)) (bool, error) {
	if !c.owns(ed, path) {
		return false, errSaveOwnerClosed
	}
	if fileRenameAffects(path) || workspaceSearchAffects(path) || editorCloseAffects(path) {
		return false, renameBusyError(path, "save target is reserved")
	}
	base := ed.SavedTextSnapshot()
	b := c.bindings[ed]
	if b == nil || b.path != path || b.uiText != base {
		b = &sourceDiskBinding{state: newSourceDiskState(base), owner: new(saveReservationOwner), path: path, uiText: base}
		c.bindings[ed] = b
	}
	// Different documents are not silently replaced in a latest-write queue.
	// Save All sequences them; a concurrent explicit request gets visible busy.
	if anySaveBusyExcept(b.owner) {
		return false, renameBusyError(path, "another document save is still running")
	}
	id := new(designSaveToken)
	c.intent = id
	c.retire(async.ErrSuperseded)
	if !c.owns(ed, path) || c.intent != id {
		return false, errSaveOwnerClosed
	}
	ticket, ok := reserveSavePaths(b.owner, []string{path})
	if !ok {
		return false, renameBusyError(path, "save target is reserved")
	}
	ed.SealTypingUndoGroup()
	j := &sourceSaveJob{id: id, transitionScope: sourceSaveTransitionScope(ed, path), box: new(sourceSaveMailbox), editor: ed, path: path, text: ed.Text(), revision: ed.TextRevision(), reservation: ticket, done: done}
	j.box.owner.Store(c)
	c.pending = j
	text, crlf := j.text, ed.LineEndings().Preferred == gui.LineEndingCRLF
	format, write, post, state, box := c.settings.format, c.settings.write, c.settings.post, b.state, j.box
	future, err := c.settings.queue.Submit(context.Background(), func(ctx context.Context) (designWriteResult, error) {
		if err := ctx.Err(); err != nil {
			return designWriteResult{}, err
		}
		out := text
		var formatErr error
		if strings.EqualFold(filepath.Ext(path), ".go") {
			if formatted, err := format(ctx, text); err == nil {
				out = formatted
				if crlf {
					out = strings.ReplaceAll(strings.ReplaceAll(out, "\r\n", "\n"), "\n", "\r\n")
				}
			} else {
				formatErr = err
			}
		}
		if err := ctx.Err(); err != nil {
			return designWriteResult{}, err
		}
		version, err := write(path, []byte(out), state.load())
		if err != nil {
			core.Warn("background source save: ", path, ": ", err)
			return designWriteResult{}, err
		}
		state.store(version)
		return designWriteResult{Written: true, Text: out, FormatErr: formatErr, Fingerprint: version}, nil
	})
	if err != nil {
		c.pending = nil
		box.owner.Store(nil)
		ticket.finish(true)
		ticket.finish(false)
		return false, err
	}
	j.future = future
	_ = future.OnComplete(func(f func()) { f() }, func(result designWriteResult, err error) {
		ticket.finish(true)
		if box.owner.Load() == nil {
			return
		}
		post(func() {
			if owner := box.owner.Load(); owner != nil {
				owner.complete(id, result, err)
			}
		})
	})
	return true, nil
}

func noteSynchronousSourceSave(ed *gui.CodeEditor, path, text string) {
	noteObservedSourceSave(ed, path, text, core.FingerprintBytes([]byte(text)))
}

// A caller which wrote on a worker publishes the worker's actual encoded hash,
// not a new potentially large UI-thread hash/read of the destination.
func noteObservedSourceSave(ed *gui.CodeEditor, path, text string, version core.FileFingerprint) {
	w := globalWorkbench
	if w == nil || w.sourceSaves == nil {
		return
	}
	b := w.sourceSaves.bindings[ed]
	if b != nil && b.path == path {
		b.state.store(version)
		b.uiText = text
	}
}
func (c *ownedSourceSaver) complete(id *designSaveToken, r designWriteResult, err error) {
	j := c.pending
	if j == nil || j.id != id {
		return
	}
	c.pending = nil
	j.box.owner.Store(nil)
	out := sourceSaveOutcome{Path: j.path, Written: r.Written, Err: err, FormatErr: r.FormatErr}
	if c.owns(j.editor, j.path) && err == nil && r.Written {
		ed := j.editor
		waiting := initialTransitionSourcePublication(ed, j.path)
		if ed.TextRevision() == j.revision && ed.Text() == j.text && r.Text != j.text {
			ed.ReplaceAllText(r.Text)
		}
		if !c.owns(ed, j.path) || c.intent != id {
			out.Err = async.ErrSuperseded
			j.reservation.finish(false)
			if j.done != nil {
				j.done(out)
			}
			return
		}
		// Publish physically saved bytes even when newer input remains dirty.
		ed.MarkSnapshotSaved(r.Text)
		if b := c.bindings[ed]; b != nil && b.path == j.path {
			b.uiText = r.Text
		}
		out.Adopted = !ed.Modified()
		if out.Adopted {
			waiting.sourcePublished(ed, j.path)
		}
		w := c.owner
		ed.RefreshGitStatus()
		if c.owns(ed, j.path) {
			refreshEditorTab(w.tabs, ed, j.path)
		}
		if c.owns(ed, j.path) {
			syncDocumentWindowTitle(w.frame, w.canvas)
		}
	} else if !c.owns(j.editor, j.path) {
		out.Err = errSaveOwnerClosed
	}
	if !c.owns(j.editor, j.path) {
		out.Err = errSaveOwnerClosed
	} else if c.intent != id {
		out.Err = async.ErrSuperseded
	}
	if out.Err == nil && out.Written && out.Adopted {
		noteTransitionSourceSaved(j.editor, j.path, r.Text, r.Fingerprint, j.transitionScope)
	}
	j.reservation.finish(false)
	if j.done != nil {
		j.done(out)
	}
}
func (c *ownedSourceSaver) retire(err error) {
	if j := c.pending; j != nil {
		c.pending = nil
		j.box.owner.Store(nil)
		j.reservation.finish(false)
		if j.future != nil {
			j.future.Cancel()
		}
		if j.done != nil {
			j.done(sourceSaveOutcome{Path: j.path, Err: err})
		}
	}
}
func (c *ownedSourceSaver) forget(ed *gui.CodeEditor) {
	if c.pending != nil && c.pending.editor == ed {
		c.retire(errSaveOwnerClosed)
	}
	delete(c.bindings, ed)
}
func (c *ownedSourceSaver) Close() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	if c.stop != nil {
		c.stop()
		c.stop = nil
	}
	c.retire(errSaveOwnerClosed)
	c.owner = nil
	c.bindings = nil
}

func requestOwnedSourceSave(ed *gui.CodeEditor, path string, done func(sourceSaveOutcome)) (bool, error) {
	w := globalWorkbench
	if w == nil || w.canvas == nil {
		return false, errSaveOwnerClosed
	}
	c := w.sourceSaves
	if c == nil {
		c = newOwnedSourceSaver(w, sourceSaveSettings{})
	}
	return c.request(ed, path, done)
}
