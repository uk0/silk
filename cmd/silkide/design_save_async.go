package main

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/internal/fileops"
)

// Production Save, recovery and destructive continuations share this physical
// writer. UI admission remains distinct from durable success and adoption.
var designSaveWrites = async.NewWriteQueue[designWriteResult]()

var errSaveOwnerClosed = errors.New("save: UI owner closed")
var errSaveViewChanged = errors.New("save: canvas or request changed during admission")

type designSaveToken struct{ marker byte }
type designSaveMailbox struct {
	owner atomic.Pointer[ownedDesignSaver]
}
type designSaveOutcome struct {
	Path             string
	Written, Adopted bool
	Err              error
	CompanionErr     error // warning only: a successful primary stays saved
	RecoveryErr      error
}
type designWriteResult struct {
	Written      bool
	CompanionErr error
	RecoveryErr  error
	Text         string
	FormatErr    error
	Inspections  []transitionDisk
	Fingerprint  core.FileFingerprint
}
type ownedDesignSaveSettings struct {
	queue     *async.WriteQueue[designWriteResult]
	post      func(func())
	write     func(context.Context, *ged.DesignWriteSnapshot, core.FileFingerprint) error
	companion func(string, *core.TDoc, *ged.CodeSnapshot) error
	cleanup   func(string, core.FileFingerprint) error
	recovery  func(context.Context, *core.TDoc, string) error
	inspect   func(context.Context, string) (core.FileFingerprint, error)
}
type ownedDesignSaveJob struct {
	id          *designSaveToken
	scope       *designSaveToken
	box         *designSaveMailbox
	scene       *ged.GedScene
	receipt     *ged.PreparedDesignSave
	future      *async.Future[designWriteResult]
	reservation *saveReservation
	done        func(designSaveOutcome)
}
type ownedDesignSaver struct {
	owner            *workbench
	settings         ownedDesignSaveSettings
	intent           *designSaveToken
	pending          *ownedDesignSaveJob
	review           *designSaveReview
	closed           bool
	choosing         bool
	chooserScope     *designSaveToken
	admittingScope   *designSaveToken
	stop             func()
	baselineScene    *ged.GedScene
	baselines        map[string]*saveDiskState
	reservationOwner *saveReservationOwner
}

func newOwnedDesignSaver(w *workbench, settings ownedDesignSaveSettings) *ownedDesignSaver {
	if settings.queue == nil {
		settings.queue = designSaveWrites
	}
	if settings.post == nil {
		settings.post = gui.Post
	}
	if settings.write == nil {
		settings.write = func(ctx context.Context, snapshot *ged.DesignWriteSnapshot, expected core.FileFingerprint) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return snapshot.WriteAgainst(expected)
		}
	}
	if settings.companion == nil {
		settings.companion = regenerateCompanionSnapshot
	}
	if settings.cleanup == nil {
		settings.cleanup = cleanupSavedRecovery
	}
	if settings.recovery == nil {
		settings.recovery = func(ctx context.Context, doc *core.TDoc, path string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return doc.SaveFileAtomic(path)
		}
	}
	if settings.inspect == nil {
		settings.inspect = func(ctx context.Context, path string) (core.FileFingerprint, error) {
			if err := ctx.Err(); err != nil {
				return core.FileFingerprint{}, err
			}
			return fileops.InspectFingerprint(path)
		}
	}
	c := &ownedDesignSaver{owner: w, settings: settings, reservationOwner: new(saveReservationOwner)}
	w.designSaves = c
	c.stop = w.canvas.SubscribeClosed(c.Close)
	return c
}
func (c *ownedDesignSaver) valid() bool {
	return c != nil && !c.closed && c.owner != nil && globalWorkbench == c.owner &&
		c.owner.designSaves == c && c.owner.canvas != nil && !c.owner.canvas.IsClosed()
}

// requestOwnedDesignSave reports ADMISSION, never durable success. Only its UI
// callback reports both the real physical outcome and separate adoption result.
// Callers must not use this bool to continue close/open/quit. No chooser, focus,
// MRU or autosave mutation happens here. The target must be explicit. A typed
// companion snapshot is captured on UI; generation/writes run after the primary
// on the same physical worker and report their warning separately.
// Rejection returns an error without calling done; an admitted request resolves
// done exactly once, including explicit logical retirement. Retirement does not
// promise that a physically running write did not/will not reach disk.
func requestOwnedDesignSave(path string, done func(designSaveOutcome)) (bool, error) {
	w := globalWorkbench
	if w == nil || w.canvas == nil {
		return false, errSaveOwnerClosed
	}
	c := w.designSaves
	if c == nil {
		c = newOwnedDesignSaver(w, ownedDesignSaveSettings{})
	}
	return c.request(path, done)
}
func (c *ownedDesignSaver) request(path string, done func(designSaveOutcome)) (bool, error) {
	return c.requestScoped(path, done, nil)
}
func (c *ownedDesignSaver) requestScoped(path string, done func(designSaveOutcome), scope *designSaveToken) (bool, error) {
	if !c.valid() || c.owner.canvas.HasCapture() || path == "" {
		return false, errSaveViewChanged
	}
	if fileRenameAffects(path) || workspaceSearchAffects(path) || editorCloseAffects(path) {
		return false, renameBusyError(path, "save target is reserved by another file operation")
	}
	id := new(designSaveToken)
	c.intent = id
	c.admittingScope = scope
	defer func() {
		if c.admittingScope == scope {
			c.admittingScope = nil
		}
	}()
	c.retireReview(async.ErrSuperseded)
	c.retirePending(async.ErrSuperseded)
	// Resolving an older continuation can re-enter request/close the owner.
	if !c.valid() || c.intent != id {
		return false, errSaveViewChanged
	}
	scene := c.owner.canvas.GedScene()
	if scene == nil {
		return false, errSaveViewChanged
	}
	receipt, err := scene.PrepareSave(path)
	if err != nil {
		return false, err
	}
	code := scene.CaptureCodeSnapshot()
	if !c.valid() || c.intent != id || c.owner.canvas.GedScene() != scene || c.owner.canvas.HasCapture() || !scene.PreparedSaveCurrent(receipt) {
		return false, errSaveViewChanged
	}
	paths := designSavePaths(receipt.Writer().Path())
	for _, p := range paths {
		if fileRenameAffects(p) || workspaceSearchAffects(p) || editorCloseAffects(p) {
			return false, renameBusyError(p, "save output is reserved")
		}
		for editorPath, ed := range openEditors {
			if workspaceSearchKey(editorPath) == workspaceSearchKey(p) && ed != nil && ed.Modified() {
				return false, renameBusyError(p, "save or close the edited output buffer before regenerating")
			}
		}
	}
	ticket, ok := reserveSavePaths(c.reservationOwner, paths)
	if !ok {
		return false, renameBusyError(path, "save output is reserved")
	}
	j := &ownedDesignSaveJob{id: id, scope: scope, box: new(designSaveMailbox), scene: scene, receipt: receipt, done: done, reservation: ticket}
	j.box.owner.Store(c)
	c.pending = j
	// Capture only pure work plus a revocable mailbox. In particular, neither
	// closure below retains j, receipt, scene, workbench or a UI continuation.
	snapshot, write, companion, post, box := receipt.Writer(), c.settings.write, c.settings.companion, c.settings.post, j.box
	cleanup := c.settings.cleanup
	state := c.diskState(scene, snapshot.Path())
	future, err := c.settings.queue.Submit(context.Background(), func(ctx context.Context) (designWriteResult, error) {
		if err := ctx.Err(); err != nil {
			return designWriteResult{}, err
		}
		// Observe the recovery before the primary write, not just before delete.
		// A backup changed/created while primary I/O runs must be preserved.
		recoveryBefore, recoveryReadErr := fileops.InspectFingerprint(snapshot.Path() + ".autosave")
		err := write(ctx, snapshot, state.load())
		if err != nil && !errors.Is(err, context.Canceled) {
			core.Warn("background design write: ", snapshot.Path(), ": ", err)
		}
		if err != nil {
			return designWriteResult{}, err
		}
		if version, ok := snapshot.WrittenFingerprint(); ok {
			state.store(version)
		}
		recoveryErr := recoveryReadErr
		if recoveryErr == nil {
			recoveryErr = safeCleanupSavedRecovery(cleanup, snapshot.Path(), recoveryBefore)
		}
		// Do not skip companion coherence merely because UI retired a primary
		// which already physically committed. This stage retains its real result
		// and physical slot until all work returns; it never loads current UI.
		warning := writeCapturedCompanion(companion, snapshot, code)
		if warning != nil {
			core.Warn("background design companion: ", snapshot.Path(), ": ", warning)
		}
		return designWriteResult{Written: true, CompanionErr: warning, RecoveryErr: recoveryErr}, nil
	})
	if err != nil {
		c.pending = nil
		j.box.owner.Store(nil)
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

func (c *ownedDesignSaver) cancelScope(scope *designSaveToken) {
	if c == nil || scope == nil {
		return
	}
	if c.chooserScope == scope || c.admittingScope == scope {
		c.intent = nil
	}
	if c.pending != nil && c.pending.scope == scope {
		c.retirePending(async.ErrSuperseded)
	}
	if c.review != nil && c.review.scope == scope {
		c.retireReview(async.ErrSuperseded)
	}
}
func writeCapturedCompanion(generate func(string, *core.TDoc, *ged.CodeSnapshot) error, snapshot *ged.DesignWriteSnapshot, code *ged.CodeSnapshot) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("companion generation panicked after primary save: %v", p)
		}
	}()
	return generate(snapshot.Path(), snapshot.DocumentCopy(), code)
}

func (c *ownedDesignSaver) complete(id *designSaveToken, result designWriteResult, err error) {
	j := c.pending
	if j == nil || j.id != id {
		return
	}
	c.pending = nil
	j.box.owner.Store(nil)
	out := designSaveOutcome{Path: j.receipt.Writer().Path(), Written: result.Written, Err: err, CompanionErr: result.CompanionErr, RecoveryErr: result.RecoveryErr}
	if c.valid() && c.intent == id && c.owner.canvas.GedScene() == j.scene && err == nil && result.Written {
		out.Adopted = j.scene.CompleteSave(j.receipt)
		if out.Adopted {
			noteInitialTransitionDesignSaved(j.scene, out.Path)
		}
	}
	if !c.valid() {
		out.Err = errSaveOwnerClosed
	} else if c.intent != id {
		out.Err = async.ErrSuperseded
	} else if c.owner.canvas.GedScene() != j.scene {
		out.Err = errSaveViewChanged
	}
	if out.Err == nil && out.Written {
		// Publish the saved/dirty title with this completion, not the next 500ms
		// title poll. The helper preserves an active source editor's own title.
		syncDocumentWindowTitle(c.owner.frame, c.owner.canvas)
	}
	j.reservation.finish(false)
	if j.done != nil {
		j.done(out)
	}
}
func (c *ownedDesignSaver) retirePending(err error) {
	if j := c.pending; j != nil {
		c.pending = nil
		j.box.owner.Store(nil)
		j.reservation.finish(false)
		if j.future != nil {
			j.future.Cancel()
		}
		if j.done != nil {
			j.done(designSaveOutcome{Path: j.receipt.Writer().Path(), Err: err})
		}
	}
}

// Close is logical/UI retirement only. A physical writer may still complete;
// its pure snapshot remains in the shared queue until it really returns.
func (c *ownedDesignSaver) Close() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	if c.stop != nil {
		c.stop()
		c.stop = nil
	}
	c.retirePending(errSaveOwnerClosed)
	c.retireReview(errSaveOwnerClosed)
	c.owner = nil
	c.baselines = nil
	c.baselineScene = nil
}
