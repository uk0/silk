package main

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

const restoreFileLimit = 32 << 20
const restoreNodeLimit = 100000

var workspaceRestorePost = gui.Post
var errRestoreNotRegular = errors.New("restore: not a regular file")

type restoredFile struct {
	modified      time.Time
	size          int64
	data          []byte
	doc, recovery *core.TDoc
	recoveryPath  string
	recoveryErr   error
}
type restoreTask struct {
	path   string
	page   *restorePage
	design bool
	review bool
}
type workspaceRestoreOptions struct {
	read            func(context.Context, string, bool) (restoredFile, error)
	post            func(func())
	timeout         time.Duration
	confirmRecovery func(gui.IWidget, string, string) bool
}

// UI owns this controller and widgets. Workers own immutable paths and byte/doc
// snapshots. Cancellation is cooperative: an OS open/stat may not return. Keep
// its executor slot occupied, never spawn replacements, and never join it in the
// UI loop or unconditionally on process shutdown. No idle polling is introduced.
type workspaceRestore struct {
	owner  *workbench
	tabs   *gui.TabWidget
	canvas *ged.GedView

	state   workspaceSession
	pages   []*restorePage
	queue   []restoreTask
	ctx     context.Context
	cancel  context.CancelFunc
	jobs    *async.Executor
	options workspaceRestoreOptions
	active  int
	closed  bool

	design                *ged.GedScene
	designPending         bool
	designChanged         bool
	designListener        int
	designInitialFilename string

	initializing     bool
	interacted       bool
	deferredRecovery bool
	reviewQueued     bool
}

type restorePage struct {
	gui.Widget
	saved  editorSession
	label  *gui.Label
	cancel context.CancelFunc
	closed bool
	manual bool
	origin gui.NavPosition
}

func init() {
	i18n.AddMany("zh", map[string]string{
		"Restoring file in background…":                           "正在后台恢复文件…",
		"Waiting for a restore worker…":                           "正在等待恢复任务…",
		"Restore timed out; close this tab to cancel":             "恢复超时；关闭此页签可取消",
		"Cancel Session Restore":                                  "取消会话恢复",
		"Review Session Recovery":                                 "检查会话恢复",
		"Autosave recovery deferred; use Review Session Recovery": "自动保存恢复已延后，请使用“检查会话恢复”",
	})
}
func newRestorePage(saved editorSession) *restorePage {
	p := new(restorePage)
	p.Init(p)
	p.SetFocusPolicy(gui.TabFocus)
	p.saved = saved
	p.label = gui.NewLabel(i18n.T("Waiting for a restore worker…"))
	p.label.SetWrap(true)
	p.label.SetParent(p)
	return p
}
func (p *restorePage) OnResize() { p.label.SetBounds(16, 16, max(0, p.Width()-32), 50) }
func (p *restorePage) Close() {
	p.closed = true
	if p.cancel != nil {
		p.cancel()
	}
}
func (p *restorePage) Draw(g paint.Painter) {
	g.SetBrush1(gui.Theme().ViewBGColor)
	g.Rectangle(0, 0, p.Width(), p.Height())
	g.Fill()
}
func editorPageIndex(tabs *gui.TabWidget, page gui.IWidget) int {
	if tabs != nil {
		for i := 0; i < tabs.Count(); i++ {
			if tabs.Stack().Page(i) == page {
				return i
			}
		}
	}
	return -1
}
func newWorkspaceRestore(state workspaceSession, tabs *gui.TabWidget, canvas *ged.GedView, options workspaceRestoreOptions) *workspaceRestore {
	ctx, cancel := context.WithCancel(context.Background())
	if options.confirmRecovery == nil {
		options.confirmRecovery = confirmRecoverFromAutosave
	}
	if options.read == nil {
		options.read = readRestoredFile
	}
	if options.post == nil {
		options.post = workspaceRestorePost
	}
	if options.timeout <= 0 {
		options.timeout = 30 * time.Second
	}
	r := &workspaceRestore{initializing: true, tabs: tabs, canvas: canvas, state: state, ctx: ctx, cancel: cancel, jobs: async.NewExecutor(2), options: options}
	r.state.Editors = append([]editorSession(nil), state.Editors...)
	if w := globalWorkbench; w != nil && w.tabs == tabs {
		r.owner = w
		if w.restore != nil {
			w.restore.Close()
		}
		w.restore = r
	}
	if canvas != nil && state.Design != "" {
		r.design = canvas.GedScene()
		r.designInitialFilename = r.design.Filename()
		r.designPending = true
		r.designListener = r.design.AddDesignChangedCallback(func() { r.designChanged = true; r.designPending = false })
		r.queue = append(r.queue, restoreTask{path: canonicalEditorPath(state.Design), design: true})
	}
	seen := map[string]bool{}
	if tabs != nil {
		for _, saved := range r.state.Editors {
			if saved.Path == "" {
				continue
			}
			saved.Path = canonicalEditorPath(saved.Path)
			key := workspaceSearchKey(saved.Path)
			if seen[key] {
				continue
			}
			seen[key] = true
			if openEditors[saved.Path] != nil {
				continue
			}
			p := newRestorePage(saved)
			r.pages = append(r.pages, p)
			tabs.AddTab(p, filepath.Base(saved.Path), gui.LoadIcon("document"))
			r.queue = append(r.queue, restoreTask{path: saved.Path, page: p})
		}
	}
	active := canonicalEditorPath(state.ActiveFile)
	for _, p := range r.pages {
		if p.saved.Path == active {
			tabs.SetCurrentIndex(editorPageIndex(tabs, p))
			break
		}
	}
	if r.owner != nil {
		r.owner.activate(state.Mode)
		if state.Mode == modeEdit && tabs != nil {
			if pending, ok := tabs.Stack().CurrentPage().(*restorePage); ok {
				// Own initial keyboard focus while loading. Completion transfers
				// it only if the user has not focused another field or workspace.
				pending.SetFocus()
			}
		}
	}
	r.initializing = false
	r.startNext()
	return r
}
func (r *workspaceRestore) valid() bool {
	return !r.closed && (r.owner == nil || globalWorkbench == r.owner && r.owner.restore == r && r.owner.tabs == r.tabs)
}
func (r *workspaceRestore) Busy() bool {
	return r != nil && !r.closed && (r.active > 0 || len(r.queue) > 0)
}
func (r *workspaceRestore) startNext() {
	for r.valid() && r.active < 2 && len(r.queue) > 0 {
		task := r.queue[0]
		r.queue[0] = restoreTask{}
		r.queue = r.queue[1:]
		if task.design && (!r.designPending || r.designChanged) {
			if task.review {
				r.reviewQueued = false
			}
			continue
		}
		if task.page != nil && (task.page.closed || editorPageIndex(r.tabs, task.page) < 0) {
			continue
		}
		ctx, cancel := context.WithTimeout(r.ctx, r.options.timeout)
		if task.page != nil {
			task.page.cancel = cancel
			task.page.label.SetText(i18n.T("Restoring file in background…"))
		}
		read, post := r.options.read, r.options.post
		future, err := async.Submit(ctx, r.jobs, func(ctx context.Context) (restoredFile, error) { return read(ctx, task.path, task.design) })
		if err != nil {
			cancel()
			if task.review {
				r.reviewQueued = false
				r.deferredRecovery = true
			}
			r.fail(task, err)
			continue
		}
		r.active++
		stopDeadline := context.AfterFunc(ctx, func() {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				post(func() {
					if r.valid() {
						r.fail(task, ctx.Err())
					}
				})
			}
		})
		_ = future.OnComplete(post, func(value restoredFile, err error) {
			stopDeadline()
			cancel()
			if !r.valid() {
				return
			}
			r.active--
			if task.review {
				r.reviewQueued = false
				if err != nil {
					r.deferredRecovery = true
				}
			}
			if task.page == nil || !task.page.closed {
				if err != nil {
					r.fail(task, err)
				} else {
					r.apply(task, value)
				}
			}
			r.startNext()
		})
	}
	if r.active == 0 && len(r.queue) == 0 && !r.designPending {
		r.unlisten()
	}
}
func (r *workspaceRestore) fail(task restoreTask, err error) {
	if task.page != nil {
		if !task.page.closed && editorPageIndex(r.tabs, task.page) >= 0 {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, errRestoreNotRegular) {
				hadFocus := task.page.HasFocus()
				idx := editorPageIndex(r.tabs, task.page)
				task.page.Close()
				r.tabs.RemoveTab(idx)
				if hadFocus && documentMode(r.tabs, r.canvas) == modeEdit {
					if ed := activeEditor(r.tabs); ed != nil {
						ed.SetFocus()
					} else if next, ok := r.tabs.Stack().CurrentPage().(*restorePage); ok {
						next.SetFocus()
					}
				}
				return
			}
			text := err.Error()
			if errors.Is(err, context.DeadlineExceeded) {
				text = i18n.T("Restore timed out; close this tab to cancel")
			}
			task.page.label.SetText(text)
		}
	} else if r.valid() {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, errRestoreNotRegular) {
			r.designPending = false
		}
		logEvent(ged.LogWarn, "Session design restore: "+err.Error())
	}
}
func (r *workspaceRestore) apply(task restoreTask, value restoredFile) {
	if !r.valid() {
		return
	}
	if task.design {
		if !r.designCurrent() {
			return
		}
		if value.recoveryPath != "" && r.interacted && !task.review {
			r.deferredRecovery = true
			logEvent(ged.LogInfo, i18n.T("Autosave recovery deferred; use Review Session Recovery"))
			return
		}
		recovery := value.recoveryPath != "" && r.options.confirmRecovery(r.canvas, task.path, value.recoveryPath)
		// A modal pumps queued input. Never overwrite a new/edited/replaced scene.
		if !r.designCurrent() {
			return
		}
		doc := value.doc
		if recovery {
			if value.recoveryErr != nil {
				r.fail(task, value.recoveryErr)
				return
			}
			doc = value.recovery
		}
		r.unlisten()
		r.designPending = false
		r.deferredRecovery = false
		if err := applyDesignDocument(r.design, doc, task.path, recovery); err != nil {
			r.fail(task, err)
			return
		}
		rebindAutoSaver(r.design)
		startReloader(r.canvas)
		if globalReloader != nil {
			if err := globalReloader.WatchFileState(task.path, value.modified, value.size); err != nil {
				logEvent(ged.LogWarn, "Session watch: "+err.Error())
			}
		}
		return
	}
	p := task.page
	idx := editorPageIndex(r.tabs, p)
	if p.closed || idx < 0 || rejectBusyFile(task.path) {
		return
	}
	if openEditors[task.path] != nil {
		// A manual open won the race. Do not replace its source, caret or Undo.
		p.Close()
		r.tabs.RemoveTab(idx)
		return
	}
	wantFocus := r.tabs.Stack().CurrentPage() == p && (p.HasFocus() || r.tabs.HasFocus() || r.tabs.TabBar().HasFocus())
	ed := attachFileInEditor(r.tabs, task.path, value.data, p, false)
	if ed == nil {
		return
	}
	p.Close()
	if openEditors[task.path] != ed || editorPageIndex(r.tabs, ed) < 0 {
		return
	}
	ed.ScrollToLineCol(p.saved.Line, p.saved.Column)
	if p.saved.ScrollY >= 0 && !math.IsNaN(p.saved.ScrollY) && !math.IsInf(p.saved.ScrollY, 0) {
		ed.SetScrollY(p.saved.ScrollY)
	}
	if wantFocus && documentMode(r.tabs, r.canvas) == modeEdit && r.tabs.Stack().CurrentPage() == ed {
		ed.SetFocus()
	}
}
func (r *workspaceRestore) unlisten() {
	if r.design != nil && r.designListener != 0 {
		r.design.RemoveDesignChangedCallback(r.designListener)
		r.designListener = 0
	}
}
func (r *workspaceRestore) Close() {
	if r == nil || r.closed {
		return
	}
	r.closed = true
	r.cancel()
	r.jobs.Close()
	r.unlisten()
	for _, p := range r.pages {
		p.Close()
	}
	// Workers may be parked in OS I/O. Release host/widget references immediately.
	r.owner = nil
	r.tabs = nil
	r.canvas = nil
	r.design = nil
	r.pages = nil
	r.queue = nil
	r.state = workspaceSession{}
	r.deferredRecovery = false
	r.reviewQueued = false
}
func cancelWorkspaceRestore(tabs *gui.TabWidget) bool {
	w := globalWorkbench
	if w == nil || w.tabs != tabs || w.restore == nil || !w.restore.Busy() {
		return false
	}
	r := w.restore
	pages := append([]*restorePage(nil), r.pages...)
	r.Close()
	for _, p := range pages {
		if idx := editorPageIndex(tabs, p); idx >= 0 {
			tabs.RemoveTab(idx)
		}
	}
	return true
}

func workspaceRestoreBusy(tabs *gui.TabWidget) bool {
	w := globalWorkbench
	return w != nil && w.tabs == tabs && w.restore != nil && w.restore.Busy()
}

func sessionRecoveryFor(tabs *gui.TabWidget) *workspaceRestore {
	if w := globalWorkbench; w != nil && w.tabs == tabs {
		return w.restore
	}
	return nil
}
func noteWorkspaceRestoreInteraction(tabs *gui.TabWidget) {
	if w := globalWorkbench; w != nil && w.tabs == tabs && w.designOpen != nil {
		w.designOpen.cancelPending()
	}
	if r := sessionRecoveryFor(tabs); r != nil && !r.initializing && !r.closed && r.designPending {
		r.interacted = true
	}
}

// Explicit manual/New design intent retires only the startup design branch;
// unrelated source restoration keeps its physical readers and owned pages.
func (r *workspaceRestore) retireDesign() {
	if r == nil || r.closed {
		return
	}
	r.designChanged = true
	r.designPending = false
	r.deferredRecovery = false
	r.reviewQueued = false
	r.unlisten()
}
func (r *workspaceRestore) canReviewRecovery() bool {
	return r != nil && r.deferredRecovery && !r.reviewQueued && r.designCurrent()
}
func reviewSessionRecovery(tabs *gui.TabWidget, expected *workspaceRestore) {
	r := sessionRecoveryFor(tabs)
	if r == nil || r != expected || !r.canReviewRecovery() {
		return
	}
	r.reviewQueued = true
	r.deferredRecovery = false
	r.queue = append([]restoreTask{{path: canonicalEditorPath(r.state.Design), design: true, review: true}}, r.queue...)
	if r.owner != nil {
		r.owner.activate(modeDesign)
	}
	r.startNext()
}

func (r *workspaceRestore) designCurrent() bool {
	if !r.valid() || !r.designPending || r.designChanged || r.design == nil || r.canvas == nil {
		return false
	}
	return r.canvas.GedScene() == r.design && r.design.Filename() == r.designInitialFilename && r.design.IsClean()
}
