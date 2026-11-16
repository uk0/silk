package main

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/hotreload"
	"github.com/uk0/silk/i18n"
)

var designOpenPost = gui.Post

type designOpenToken struct{ marker byte }
type designOpenSettings struct {
	read                func(context.Context, string) (designOpenSnapshot, error)
	readReload          func(context.Context, string) (designOpenSnapshot, error)
	post                func(func())
	afterModal          func(func())
	timeout             time.Duration
	confirmDiscard      func(*ged.GedView) bool
	confirmDiscardAsync func(string, *ged.GedView, func(bool)) bool
	confirmRecovery     func(gui.IWidget, string, string) bool
	installed           func(*ged.GedView, string, string, designOpenSnapshot)
}

// Worker callbacks retain only this revocable mailbox, not a scene or frame.
type designOpenMailbox struct {
	owner   atomic.Pointer[designOpenController]
	payload atomic.Bool
}
type designOpenJob struct {
	id                     *designOpenToken
	box                    *designOpenMailbox
	path, filename         string
	scene                  *ged.GedScene
	listener               int
	recent, running, done  bool
	focus                  gui.IWidget
	ctx                    context.Context
	cancel                 context.CancelFunc
	stopDeadline           func() bool
	undoCount, undoCurrent int
	undoClean              bool
	modalFocusGap          bool
	reload                 bool
	reloadOwner            *hotreload.Reloader
	mode                   workspaceMode
}

// The single design viewport has one physical reader and one replaceable latest
// queued request. Logical timeout/cancel never frees a physically blocked slot.
// Startup recovery and source-file opening have separate pools, not this limit.
// Mutable state, prompts and widget creation are UI-owned; no idle polling.
type designOpenController struct {
	owner           *workbench
	canvas          *ged.GedView
	settings        designOpenSettings
	jobs            *async.Executor
	ctx             context.Context
	cancel          context.CancelFunc
	all             map[*designOpenToken]*designOpenJob
	pending, queued *designOpenJob
	intent          *designOpenToken
	admitting       *designOpenToken
	admittingPath   string
	active          int
	closed          bool
	stopClosed      func()
	status          string
	reloadBridge    *designReloadBridge
}

func init() {
	i18n.AddMany("zh", map[string]string{
		"Cancel Design Open": "取消设计打开任务",
		"Opening design %s in background; Cancel Design Open to stop": "正在后台打开设计 %s；可使用“取消设计打开任务”停止",
		"Design open timed out; current design was kept":              "打开设计超时；已保留当前设计",
		"Design open canceled; current design or input changed":       "设计打开已取消；当前设计或输入已发生变化",
	})
}

func newDesignOpenController(w *workbench, s designOpenSettings) *designOpenController {
	if s.read == nil {
		s.read = readManualDesign
	}
	if s.readReload == nil {
		s.readReload = readReloadDesign
	}
	if s.post == nil {
		s.post = designOpenPost
	}
	if s.afterModal == nil {
		s.afterModal = gui.PostAfterNativeCleanup
	}
	if s.timeout <= 0 {
		s.timeout = 30 * time.Second
	}
	if s.confirmDiscardAsync == nil {
		s.confirmDiscardAsync = func(path string, canvas *ged.GedView, done func(bool)) bool {
			return requestDesignTransition("open:"+workspaceSearchKey(path), canvas, done)
		}
	}
	if s.confirmRecovery == nil {
		s.confirmRecovery = confirmRecoverFromAutosave
	}
	if s.installed == nil {
		s.installed = installOpenedDesign
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &designOpenController{owner: w, canvas: w.canvas, settings: s, jobs: async.NewExecutor(1), ctx: ctx, cancel: cancel, all: map[*designOpenToken]*designOpenJob{}}
	c.stopClosed = w.canvas.SubscribeClosed(c.Close)
	return c
}

func designOpenerFor(tabs *gui.TabWidget, canvas *ged.GedView) *designOpenController {
	w := globalWorkbench
	if w == nil || w.tabs != tabs || w.canvas != canvas || canvas.IsClosed() {
		return nil
	}
	if w.designOpen == nil {
		w.designOpen = newDesignOpenController(w, designOpenSettings{})
	}
	if !w.designOpen.valid() {
		return nil
	}
	return w.designOpen
}
func (c *designOpenController) valid() bool {
	return c != nil && !c.closed && c.owner != nil && globalWorkbench == c.owner && c.owner.designOpen == c && c.owner.canvas == c.canvas && !c.canvas.IsClosed()
}
func (c *designOpenController) Busy() bool {
	return c != nil && !c.closed && (len(c.all) != 0 || c.admitting != nil)
}
func (c *designOpenController) current(j *designOpenJob) bool {
	if !c.valid() || j == nil || j.done || c.pending != j || c.intent != j.id || j.scene == nil || c.canvas.GedScene() != j.scene || j.scene.Filename() != j.filename {
		return false
	}
	history := j.scene.UndoStack()
	if history.Count() != j.undoCount || history.Current() != j.undoCurrent || history.IsClean() != j.undoClean {
		return false
	}
	if j.reload {
		return j.reloadOwner != nil && globalReloader == j.reloadOwner && !j.reloadOwner.Stats().Closed &&
			!j.scene.HasPendingEdits() && !c.canvas.HasCapture() && documentMode(c.owner.tabs, c.canvas) == j.mode &&
			gui.CurrentFocusWidget() == j.focus
	}
	return !j.scene.HasPendingEdits() && !c.canvas.HasCapture() &&
		documentMode(c.owner.tabs, c.canvas) == modeDesign && designOpenFocusOwned(j.focus, gui.CurrentFocusWidget(), c.canvas, j.modalFocusGap)
}

// Native focus restoration is conditional (e.g. Cocoa may leave no key input
// owner). After our own modal has retired, an empty focus is not another field's
// intent and must not discard its explicit answer. This narrow exception applies
// only to the canvas we intentionally activated at admission. Never restore or
// steal focus here; a real foreign input owner still wins, as do all edit guards.
func designOpenFocusOwned(saved, current gui.IWidget, canvas *ged.GedView, modalRetired bool) bool {
	return current == saved || modalRetired && current == nil && saved == canvas
}
func requestDesignOpen(path string, tabs *gui.TabWidget, canvas *ged.GedView, recent bool) bool {
	c := designOpenerFor(tabs, canvas)
	if c == nil {
		return false
	}
	return c.open(path, recent)
}
func (c *designOpenController) open(path string, recent bool) bool {
	path = canonicalEditorPath(path)
	if c.valid() {
		if t := c.owner.transitions; t != nil {
			t.cancelForIntent("open:" + workspaceSearchKey(path))
		}
		if c.admitting != nil && c.admittingPath == path {
			return true
		}
	}
	if !c.valid() || path == "" || rejectBusyFile(path) {
		return false
	}
	if j := c.pending; j != nil && workspaceSearchKey(j.path) == workspaceSearchKey(path) && c.current(j) {
		j.recent = recent
		return true
	}
	c.cancelPending()
	intent := new(designOpenToken)
	c.intent = intent
	c.admitting = intent
	c.admittingPath = path
	scene := c.canvas.GedScene()
	if scene == nil {
		c.admitting = nil
		c.admittingPath = ""
		return false
	}
	// A modal can run queued completions. Retire older requests first and check
	// both workbench and scene after the explicit discard/save decision.
	if confirm := c.settings.confirmDiscard; confirm != nil {
		return c.continueOpen(intent, scene, path, recent, confirm(c.canvas))
	}
	accepted := c.settings.confirmDiscardAsync(path, c.canvas, func(ok bool) { c.continueOpen(intent, scene, path, recent, ok) })
	if !accepted && c.admitting == intent {
		c.admitting = nil
		c.admittingPath = ""
	}
	return accepted
}

func (c *designOpenController) continueOpen(intent *designOpenToken, scene *ged.GedScene, path string, recent, allowed bool) bool {
	if c.admitting == intent {
		c.admitting = nil
		c.admittingPath = ""
	}
	if !allowed || !c.valid() || c.intent != intent || c.canvas.GedScene() != scene || scene.HasPendingEdits() {
		return false
	}
	// Reveal before admission: this intentional activation is not an interaction
	// that cancels the request we are about to create.
	noteWorkspaceRestoreInteraction(c.owner.tabs)
	if r := sessionRecoveryFor(c.owner.tabs); r != nil {
		r.retireDesign()
	}
	showDesignWorkspace(c.canvas)
	if !c.valid() || c.intent != intent || c.canvas.GedScene() != scene {
		return false
	}
	ctx, cancel := context.WithTimeout(c.ctx, c.settings.timeout)
	j := &designOpenJob{id: intent, box: new(designOpenMailbox), path: path, filename: scene.Filename(), scene: scene, recent: recent, focus: gui.CurrentFocusWidget(), ctx: ctx, cancel: cancel}
	c.admit(j)
	return true
}

// Shared physical-reader budget for explicit opens and automatic reloads. A
// canceled OS read keeps its slot until it returns, regardless of request kind.
func (c *designOpenController) admit(j *designOpenJob) {
	scene, ctx := j.scene, j.ctx
	j.undoCount, j.undoCurrent, j.undoClean = scene.UndoStack().Count(), scene.UndoStack().Current(), scene.UndoStack().IsClean()
	j.box.owner.Store(c)
	j.box.payload.Store(true)
	c.all[j.id] = j
	c.pending = j
	c.queued = j
	j.listener = scene.AddDesignChangedCallback(func() {
		if c.pending == j {
			c.cancelPending()
		}
	})
	box, id, post := j.box, j.id, c.settings.post
	j.stopDeadline = context.AfterFunc(ctx, func() {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			box.payload.Store(false)
			post(func() {
				if owner := box.owner.Load(); owner != nil {
					owner.expire(id)
				}
			})
		}
	})
	if j.reload {
		c.setStatus(i18n.Tf("Reloading design %s in background", filepath.Base(j.path)))
	} else {
		c.setStatus(i18n.Tf("Opening design %s in background; Cancel Design Open to stop", filepath.Base(j.path)))
	}
	c.startNext()
}
func (c *designOpenController) startNext() {
	if !c.valid() || c.active != 0 || c.queued == nil {
		return
	}
	j := c.queued
	c.queued = nil
	if !c.current(j) {
		c.dropChanged(j)
		return
	}
	path, read := j.path, c.settings.read
	if j.reload {
		read = c.settings.readReload
	}
	future, err := async.Submit(j.ctx, c.jobs, func(ctx context.Context) (designOpenSnapshot, error) { return read(ctx, path) })
	if err != nil {
		c.fail(j, err)
		return
	}
	j.running = true
	c.active++
	box, id, post := j.box, j.id, c.settings.post
	_ = future.OnComplete(func(f func()) { f() }, func(value designOpenSnapshot, err error) {
		if !box.payload.Load() {
			value = designOpenSnapshot{}
		}
		post(func() {
			if owner := box.owner.Load(); owner != nil {
				owner.complete(id, value, err)
			}
		})
	})
}
func (c *designOpenController) complete(id *designOpenToken, value designOpenSnapshot, err error) {
	j := c.all[id]
	if j == nil {
		return
	}
	if j.running {
		j.running = false
		c.active--
	}
	if !j.done {
		if err == nil {
			err = j.ctx.Err()
		}
		if j.stopDeadline != nil {
			j.stopDeadline()
		}
		j.cancel()
		if c.deliver(j, value, err) {
			c.startNext()
			return // the owned modal continuation retires this prepared job
		}
	}
	c.retire(j)
	c.startNext()
}
func (c *designOpenController) deliver(j *designOpenJob, value designOpenSnapshot, err error) bool {
	// Own-save watcher notifications are not user attempts to mutate a busy
	// file. Silently retire them rather than stacking a misleading busy toast.
	if j.reload && (transitionBusy() || savePathBusy(j.path) || saveBatchAffects(j.path)) {
		return false
	}
	if !c.current(j) {
		c.dropChanged(j)
		return false
	}
	if err != nil {
		c.fail(j, err)
		return false
	}
	if rejectBusyFile(j.path) {
		return false
	}
	if !j.reload && value.recoveryPath != "" {
		recovered := c.settings.confirmRecovery(c.canvas, j.path, value.recoveryPath)
		// GLFW may return before native destruction/conditional focus repair.
		// Wait for cleanup without forcing focus; only then may an unowned
		// canvas focus gap be distinguished from a real foreign input owner.
		box, id := j.box, j.id
		c.settings.afterModal(func() {
			if owner := box.owner.Load(); owner != nil {
				if pending := owner.all[id]; pending != nil && !pending.done {
					pending.modalFocusGap = true
					owner.applyPrepared(pending, value, recovered)
					owner.retire(pending)
					owner.startNext()
				}
			}
		})
		return true
	}
	c.applyPrepared(j, value, false)
	return false
}

func (c *designOpenController) applyPrepared(j *designOpenJob, value designOpenSnapshot, recovered bool) {
	if !c.current(j) {
		c.dropChanged(j)
		return
	}
	doc, readErr := value.doc, value.sourceErr
	if recovered {
		doc, readErr = value.recovery, value.recoveryErr
	}
	if readErr != nil {
		c.fail(j, readErr)
		return
	}
	if doc == nil {
		c.fail(j, errors.New("open: empty design snapshot"))
		return
	}
	if !c.current(j) || rejectBusyFile(j.path) {
		return
	}
	if j.reload {
		c.applyReload(j, doc, value.fingerprint)
		return
	}
	scene, path, previous, recent, id := j.scene, j.path, j.filename, j.recent, j.id
	// Stop our listener before the single, UI-owned document installation. A new
	// intent dispatched by other observers still wins over subsequent host effects.
	c.retire(j)
	if err := applyDesignDocument(scene, doc, path, recovered); err != nil {
		if c.valid() && c.intent == id {
			silkideToast(err.Error(), gui.ToastWarning)
		}
		return
	}
	if !c.valid() || c.intent != id || c.canvas.GedScene() != scene || scene.Filename() != path {
		return
	}
	bindDesignDisk(scene, path, value.fingerprint)
	c.settings.installed(c.canvas, path, previous, value)
	if !c.valid() || c.intent != id || c.canvas.GedScene() != scene || scene.Filename() != path {
		return
	}
	syncDocumentWindowTitle(c.owner.frame, c.canvas)
	if recent {
		recordRecentFile(path)
	}
	logEvent(ged.LogInfo, "Opened "+path)
	if recovered {
		silkideToast(i18n.T("Recovered from autosave"), gui.ToastSuccess)
	} else {
		silkideToast(i18n.Tf("Opened %s", filepath.Base(path)), gui.ToastInfo)
	}
}

func (c *designOpenController) dropChanged(j *designOpenJob) {
	if c.valid() && c.pending == j && !j.done && !j.reload {
		text := i18n.T("Design open canceled; current design or input changed")
		logEvent(ged.LogInfo, text)
		silkideToast(text, gui.ToastInfo)
	}
	c.retire(j)
}
func installOpenedDesign(canvas *ged.GedView, path, previous string, value designOpenSnapshot) {
	rebindAutoSaver(canvas.GedScene())
	startReloader(canvas)
	if globalReloader != nil {
		if previous != "" && workspaceSearchKey(previous) != workspaceSearchKey(path) {
			globalReloader.Unwatch(previous)
		}
		if err := globalReloader.WatchFileState(path, value.modified, value.size); err != nil {
			logEvent(ged.LogWarn, "Design watch: "+err.Error())
		}
	}
}
func (c *designOpenController) fail(j *designOpenJob, err error) {
	if c.valid() && c.pending == j && !j.done && !errors.Is(err, context.Canceled) {
		if j.reload {
			logEvent(ged.LogWarn, "Design reload: "+j.path+": "+err.Error())
			c.retire(j)
			return
		}
		text := err.Error()
		if errors.Is(err, context.DeadlineExceeded) {
			text = i18n.T("Design open timed out; current design was kept")
		}
		logEvent(ged.LogWarn, "Design open: "+text)
		silkideToast(text, gui.ToastWarning)
	}
	c.retire(j)
}
func (c *designOpenController) expire(id *designOpenToken) {
	if j := c.all[id]; j != nil && !j.done {
		c.fail(j, context.DeadlineExceeded)
	}
	c.startNext()
}
func (c *designOpenController) retire(j *designOpenJob) {
	if j == nil {
		return
	}
	if c.pending == j {
		c.pending = nil
		c.clearStatus()
	}
	if c.queued == j {
		c.queued = nil
	}
	j.done = true
	j.box.payload.Store(false)
	if j.stopDeadline != nil {
		j.stopDeadline()
	}
	j.cancel()
	if j.scene != nil && j.listener != 0 {
		j.scene.RemoveDesignChangedCallback(j.listener)
	}
	j.listener = 0
	j.scene = nil
	j.focus = nil
	j.modalFocusGap = false
	j.reloadOwner = nil
	if !j.running {
		j.box.owner.Store(nil)
		delete(c.all, j.id)
	}
}
func (c *designOpenController) cancelPending() bool {
	if c == nil || c.pending == nil {
		return false
	}
	c.retire(c.pending)
	return true
}
func (c *designOpenController) setStatus(text string) {
	c.status = text
	if c.owner != nil && c.owner.frame != nil {
		if s := c.owner.frame.StatusBar(); s != nil {
			s.ShowMessage(text)
		}
	}
}
func (c *designOpenController) clearStatus() {
	if c.status != "" && c.owner != nil && c.owner.frame != nil {
		if s := c.owner.frame.StatusBar(); s != nil && s.Message() == c.status {
			s.ClearMessage()
		}
	}
	c.status = ""
}
func (c *designOpenController) Close() {
	if c == nil || c.closed {
		return
	}
	c.retireAdmittingTransition()
	if c.closed {
		return
	}
	c.closed = true
	if c.reloadBridge != nil {
		c.reloadBridge.close()
		c.reloadBridge = nil
	}
	c.cancel()
	c.clearStatus()
	if c.stopClosed != nil {
		c.stopClosed()
		c.stopClosed = nil
	}
	for _, j := range c.all {
		c.retire(j)
		j.box.owner.Store(nil)
	}
	c.jobs.Close()
	c.all = nil
	c.pending = nil
	c.queued = nil
	c.intent = nil
	c.admitting = nil
	c.admittingPath = ""
	c.owner = nil
	c.canvas = nil
}

func (c *designOpenController) retireAdmittingTransition() {
	// Only this live opener's still-admitting path owns the parent save/guard.
	// Closing an obsolete controller must not retire a newer opener's intent.
	if !c.valid() || c.admitting == nil {
		return
	}
	key := "open:" + workspaceSearchKey(c.admittingPath)
	if t := c.owner.transitions; t != nil && t.active != nil && t.active.key == key {
		t.active.finish(false)
	}
}
func cancelOwnedDesignOpen(c *designOpenController) bool {
	if !c.valid() {
		return false
	}
	c.intent = nil
	hadGuard := c.admitting != nil
	c.retireAdmittingTransition()
	c.admitting = nil
	c.admittingPath = ""
	return c.cancelPending() || hadGuard
}
func designOpenPending(tabs *gui.TabWidget) bool {
	w := globalWorkbench
	return w != nil && w.tabs == tabs && w.designOpen.valid() && (w.designOpen.pending != nil || w.designOpen.admitting != nil)
}
