package main

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

const editorOpenPendingLimit = 128

var editorOpenPost = gui.Post
var errEditorOpenBusy = errors.New("open: file operation in progress")

type editorOpenToken struct{ marker byte }
type editorOpenIntent struct{ origin gui.NavPosition }
type editorOpenOptions struct {
	recent bool
	saved  *editorSession
	ready  func(*gui.CodeEditor) bool
	failed func(error)
}
type editorOpenSettings struct {
	read    func(context.Context, string) ([]byte, error)
	post    func(func())
	timeout time.Duration
	limit   int
}

// A worker retains a mailbox, never a page/workbench/job callback. Close clears
// its owner before dropping UI state, even if an OS call ignores cancellation.
type editorOpenMailbox struct {
	owner   atomic.Pointer[editorOpenController]
	payload atomic.Bool
}
type editorOpenJob struct {
	id            *editorOpenToken
	box           *editorOpenMailbox
	key, path     string
	page          *restorePage
	intent        *editorOpenIntent
	options       editorOpenOptions
	ctx           context.Context
	cancel        context.CancelFunc
	stopDeadline  func() bool
	running, done bool
}

// All mutable controller/job/page state is UI-owned. The two physical executor
// slots remain occupied after logical cancel/deadline until the reads return.
// This pool is separate from startup restoration; it is not a global two-reader
// promise. There is no idle timer, replacement-thread loop, or UI-thread join.
type editorOpenController struct {
	owner           *workbench
	tabs            *gui.TabWidget
	jobs            *async.Executor
	ctx             context.Context
	cancel          context.CancelFunc
	settings        editorOpenSettings
	all             map[*editorOpenToken]*editorOpenJob
	pending         map[string]*editorOpenJob
	queue           []*editorOpenJob
	active          int
	closed, pumping bool
	intent          *editorOpenIntent
	historyStamp    *editorOpenToken
	stopHistory     func()
}

func init() {
	i18n.AddMany("zh", map[string]string{
		"Opening file in background…":                            "正在后台打开文件…",
		"Waiting for a file reader…":                             "正在等待文件读取任务…",
		"File open timed out; close this tab to cancel":          "打开文件超时；关闭此页签可取消",
		"Too many pending file opens; close a loading tab first": "等待打开的文件过多；请先关闭一个加载页签",
		"Cancel File Opens":                                      "取消文件打开任务",
	})
}
func newEditorOpenController(w *workbench, settings editorOpenSettings) *editorOpenController {
	if settings.read == nil {
		settings.read = func(ctx context.Context, path string) ([]byte, error) {
			b, _, err := readRestoreSnapshot(ctx, path)
			return b, err
		}
	}
	if settings.post == nil {
		settings.post = editorOpenPost
	}
	if settings.timeout <= 0 {
		settings.timeout = 30 * time.Second
	}
	if settings.limit <= 0 {
		settings.limit = editorOpenPendingLimit
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &editorOpenController{owner: w, tabs: w.tabs, jobs: async.NewExecutor(2), ctx: ctx, cancel: cancel, settings: settings, all: map[*editorOpenToken]*editorOpenJob{}, pending: map[string]*editorOpenJob{}, historyStamp: new(editorOpenToken)}
	c.stopHistory = w.navigation.SubscribeChanged(func() { c.historyStamp = new(editorOpenToken) })
	return c
}
func editorOpenerFor(tabs *gui.TabWidget) *editorOpenController {
	w := globalWorkbench
	if w == nil || tabs == nil || w.tabs != tabs {
		return nil
	}
	if w.opens == nil {
		w.opens = newEditorOpenController(w, editorOpenSettings{})
	}
	if !w.opens.valid() {
		return nil
	}
	return w.opens
}
func (c *editorOpenController) valid() bool {
	return c != nil && !c.closed && c.owner != nil && globalWorkbench == c.owner && c.owner.opens == c && c.owner.tabs == c.tabs
}
func (c *editorOpenController) Busy() bool { return c != nil && !c.closed && len(c.all) != 0 }
func (c *editorOpenController) newIntent() *editorOpenIntent {
	c.intent = &editorOpenIntent{origin: editorNavigationPosition(c.tabs)}
	return c.intent
}
func requestEditorOpen(tabs *gui.TabWidget, path string, options editorOpenOptions) bool {
	c := editorOpenerFor(tabs)
	if c == nil {
		return false
	}
	return c.open(c.newIntent(), path, options)
}
func trackedOpenEditor(tabs *gui.TabWidget, path string) *gui.CodeEditor {
	if ed := openEditors[path]; ed != nil && editorPageIndex(tabs, ed) >= 0 {
		return ed
	}
	key := workspaceSearchKey(path)
	for p, ed := range openEditors {
		if ed != nil && workspaceSearchKey(p) == key && editorPageIndex(tabs, ed) >= 0 {
			return ed
		}
	}
	return nil
}
func (c *editorOpenController) open(intent *editorOpenIntent, path string, options editorOpenOptions) bool {
	if !c.valid() || path == "" {
		return false
	}
	// Reject/cancel an interactive competing intent at admission, not ready
	// callbacks. Blanket rejection at apply would strand already-read startup
	// pages after a canceled modal guard. A late ready page instead invalidates
	// the guard's captured source set before it can authorize teardown.
	if t := c.owner.transitions; t != nil {
		t.cancelForIntent("source-open:" + workspaceSearchKey(path))
	}
	if !c.valid() {
		return false
	}
	if transitionBusy() {
		silkideToast(i18n.T("Finish the current file operation before continuing"), gui.ToastWarning)
		return false
	}
	noteWorkspaceRestoreInteraction(c.tabs)
	path = canonicalEditorPath(path)
	if path == "" || rejectBusyFile(path) {
		return false
	}
	if ed := trackedOpenEditor(c.tabs, path); ed != nil {
		focusEditorTab(c.tabs, ed)
		if c.current(intent, ed) && (options.ready == nil || options.ready(ed)) && c.current(intent, ed) {
			forgetClosedEditor(c.tabs, ed.FilePath())
			if options.recent {
				recordRecentFile(ed.FilePath())
			}
		}
		return true
	}
	key := workspaceSearchKey(path)
	if job := c.pending[key]; job != nil && !job.done && !job.page.closed && editorPageIndex(c.tabs, job.page) >= 0 {
		job.intent = intent
		job.options = options
		job.page.origin = intent.origin
		if options.saved != nil {
			job.page.saved = *options.saved
			job.page.saved.Path = job.path
		}
		c.focusPending(intent, job.page)
		return true
	}
	if len(c.all) >= c.settings.limit {
		silkideToast(i18n.T("Too many pending file opens; close a loading tab first"), gui.ToastWarning)
		return false
	}
	saved := editorSession{Path: path}
	var replace *restorePage
	for i := 0; i < c.tabs.Count(); i++ {
		if p, ok := c.tabs.Stack().Page(i).(*restorePage); ok && !p.closed && workspaceSearchKey(p.saved.Path) == key {
			replace = p
			saved = p.saved
			saved.Path = path
			break
		}
	}
	if options.saved != nil {
		saved = *options.saved
		saved.Path = path
	}
	p := newRestorePage(saved)
	p.manual = true
	p.origin = intent.origin
	p.label.SetText(i18n.T("Waiting for a file reader…"))
	ctx, cancel := context.WithTimeout(c.ctx, c.settings.timeout)
	job := &editorOpenJob{id: new(editorOpenToken), box: new(editorOpenMailbox), key: key, path: path, page: p, intent: intent, options: options, ctx: ctx, cancel: cancel}
	job.box.owner.Store(c)
	job.box.payload.Store(true)
	c.all[job.id] = job
	c.pending[key] = job
	c.queue = append(c.queue, job)
	box, id, post := job.box, job.id, c.settings.post
	p.cancel = func() {
		if owner := box.owner.Load(); owner != nil {
			owner.cancelJob(id)
		}
	}
	job.stopDeadline = context.AfterFunc(ctx, func() {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			box.payload.Store(false)
			post(func() {
				if owner := box.owner.Load(); owner != nil {
					owner.expire(id)
				}
			})
		}
	})
	if replace != nil {
		idx := editorPageIndex(c.tabs, replace)
		if idx < 0 || !c.tabs.ReplaceTab(idx, p, filepath.Base(path), gui.LoadIcon("document")) {
			p.Close()
			return false
		}
		replace.Close()
	} else {
		c.tabs.AddTab(p, filepath.Base(path), gui.LoadIcon("document"))
	}
	if p.closed || editorPageIndex(c.tabs, p) < 0 {
		c.cancelJob(id)
		return false
	}
	c.focusPending(intent, p)
	c.startNext()
	return true
}
func (c *editorOpenController) current(intent *editorOpenIntent, ed *gui.CodeEditor) bool {
	return c.valid() && c.intent == intent && ed != nil && openEditors[ed.FilePath()] == ed && editorPageIndex(c.tabs, ed) >= 0 && c.tabs.Stack().CurrentPage() == ed && documentMode(c.tabs, c.owner.canvas) == modeEdit
}
func (c *editorOpenController) focusPending(intent *editorOpenIntent, p *restorePage) {
	if !c.valid() || c.intent != intent {
		return
	}
	idx := editorPageIndex(c.tabs, p)
	if idx < 0 {
		return
	}
	c.tabs.SetCurrentIndex(idx)
	showEditorWorkspace(c.tabs)
	if c.valid() && c.intent == intent && c.tabs.Stack().CurrentPage() == p {
		p.SetFocus()
	}
}
func (c *editorOpenController) startNext() {
	if !c.valid() || c.pumping {
		return
	}
	c.pumping = true
	defer func() { c.pumping = false }()
	for c.valid() && c.active < 2 && len(c.queue) > 0 {
		job := c.queue[0]
		c.queue[0] = nil
		c.queue = c.queue[1:]
		if job.done {
			continue
		}
		if job.page.closed || editorPageIndex(c.tabs, job.page) < 0 {
			c.cancelJob(job.id)
			continue
		}
		job.page.label.SetText(i18n.T("Opening file in background…"))
		path, read := job.path, c.settings.read
		future, err := async.Submit(job.ctx, c.jobs, func(ctx context.Context) ([]byte, error) { return read(ctx, path) })
		if err != nil {
			delete(c.all, job.id)
			c.finish(job, nil, err)
			c.release(job)
			continue
		}
		job.running = true
		c.active++
		box, id, post := job.box, job.id, c.settings.post
		// The immediate dispatcher here only handles immutable bytes and atomics.
		// All controller/widget work is dispatched separately through the UI post.
		_ = future.OnComplete(func(f func()) { f() }, func(data []byte, err error) {
			if !box.payload.Load() {
				data = nil
			}
			post(func() {
				if owner := box.owner.Load(); owner != nil {
					owner.complete(id, data, err)
				}
			})
		})
	}
}
func (c *editorOpenController) complete(id *editorOpenToken, data []byte, err error) {
	job := c.all[id]
	if job == nil {
		return
	}
	if job.running {
		job.running = false
		c.active--
	}
	delete(c.all, id) // Release admission before reentrant ready/failure callbacks.
	if !job.done {
		if err == nil {
			err = job.ctx.Err()
		}
		c.finish(job, data, err)
	}
	c.release(job)
	c.startNext()
}
func (c *editorOpenController) expire(id *editorOpenToken) {
	job := c.all[id]
	if job == nil || job.done {
		return
	}
	if !job.running {
		delete(c.all, id)
	}
	c.finish(job, nil, context.DeadlineExceeded)
	if !job.running {
		c.release(job)
	}
	c.startNext()
}
func (c *editorOpenController) finish(job *editorOpenJob, data []byte, err error) {
	if job.done {
		return
	}
	job.done = true
	job.box.payload.Store(false)
	if c.pending[job.key] == job {
		delete(c.pending, job.key)
	}
	options, p, intent := job.options, job.page, job.intent
	job.options = editorOpenOptions{}
	job.page = nil
	job.intent = nil
	if job.stopDeadline != nil {
		job.stopDeadline()
	}
	job.cancel()
	if p != nil {
		p.cancel = nil
	}
	if !c.valid() || p == nil || p.closed || editorPageIndex(c.tabs, p) < 0 {
		return
	}
	focusBefore := gui.CurrentFocusWidget()
	focusOwned := c.tabs.Stack().CurrentPage() == p && documentMode(c.tabs, c.owner.canvas) == modeEdit && (p.HasFocus() || c.tabs.HasFocus() || c.tabs.TabBar().HasFocus())
	foreground := focusOwned && c.intent == intent
	if err == nil && fileMutationBusy(job.path) {
		err = errEditorOpenBusy
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			p.label.SetText(i18n.T("File open timed out; close this tab to cancel"))
		} else {
			p.label.SetText(err.Error())
		}
		if !errors.Is(err, context.DeadlineExceeded) && (errors.Is(err, context.Canceled) || errors.Is(err, errRestoreNotRegular) || errors.Is(err, errEditorOpenBusy) || errors.Is(err, os.ErrNotExist)) {
			idx := editorPageIndex(c.tabs, p)
			p.Close()
			c.tabs.RemoveTab(idx)
			if foreground {
				if ed := activeEditor(c.tabs); ed != nil {
					ed.SetFocus()
				}
			}
		}
		if foreground && !errors.Is(err, context.Canceled) {
			if options.failed != nil {
				options.failed(err)
			} else {
				silkideToast(err.Error(), gui.ToastWarning)
			}
		}
		return
	}
	if trackedOpenEditor(c.tabs, job.path) != nil {
		idx := editorPageIndex(c.tabs, p)
		p.Close()
		c.tabs.RemoveTab(idx)
		return
	}
	ed := attachFileInEditor(c.tabs, job.path, data, p, false)
	if ed == nil {
		return
	}
	p.Close()
	if openEditors[job.path] != ed || editorPageIndex(c.tabs, ed) < 0 {
		return
	}
	// Installing a background snapshot must not use RestoreNavigationPosition:
	// it is an explicit navigation command and intentionally takes focus.
	ed.ScrollToLineCol(p.saved.Line, p.saved.Column)
	if p.saved.ScrollY >= 0 && !math.IsNaN(p.saved.ScrollY) && !math.IsInf(p.saved.ScrollY, 0) {
		ed.SetScrollY(p.saved.ScrollY)
	}
	focusAfter := gui.CurrentFocusWidget()
	transferFocus := focusOwned && c.valid() && c.tabs.Stack().CurrentPage() == ed && documentMode(c.tabs, c.owner.canvas) == modeEdit && (focusAfter == nil || focusAfter == focusBefore || focusAfter == ed)
	if transferFocus {
		ed.SetFocus()
	}
	if foreground && transferFocus && c.current(intent, ed) {
		if options.ready != nil && !options.ready(ed) {
			return
		}
		if c.current(intent, ed) {
			forgetClosedEditor(c.tabs, job.path)
			if options.recent {
				recordRecentFile(job.path)
			}
			ed.SetFocus()
		}
	}
}
func (c *editorOpenController) release(job *editorOpenJob) {
	// Canceled queued work must leave the queue now, not wait behind a stuck
	// physical read. Otherwise repeated open/cancel batches retain unbounded jobs.
	for i, queued := range c.queue {
		if queued == job {
			copy(c.queue[i:], c.queue[i+1:])
			c.queue[len(c.queue)-1] = nil
			c.queue = c.queue[:len(c.queue)-1]
			break
		}
	}
	if len(c.queue) == 0 {
		c.queue = nil
	}
	if job.stopDeadline != nil {
		job.stopDeadline()
	}
	job.cancel()
	job.box.payload.Store(false)
	job.box.owner.Store(nil)
	delete(c.all, job.id)
	if c.pending[job.key] == job {
		delete(c.pending, job.key)
	}
	job.options = editorOpenOptions{}
	job.page = nil
	job.intent = nil
}
func (c *editorOpenController) cancelJob(id *editorOpenToken) {
	job := c.all[id]
	if job == nil || job.done {
		return
	}
	job.done = true
	job.box.payload.Store(false)
	if c.pending[job.key] == job {
		delete(c.pending, job.key)
	}
	job.options = editorOpenOptions{}
	job.page = nil
	job.intent = nil
	if job.stopDeadline != nil {
		job.stopDeadline()
	}
	job.cancel()
	if !job.running {
		c.release(job)
	}
}
func (c *editorOpenController) Close() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	c.cancel()
	if c.stopHistory != nil {
		c.stopHistory()
		c.stopHistory = nil
	}
	for _, job := range c.all {
		job.box.owner.Store(nil)
		job.box.payload.Store(false)
		if job.stopDeadline != nil {
			job.stopDeadline()
		}
		job.cancel()
		if job.page != nil {
			job.page.cancel = nil
		}
		job.page = nil
		job.options = editorOpenOptions{}
		job.intent = nil
	}
	c.jobs.Close()
	c.all = nil
	c.pending = nil
	c.queue = nil
	c.intent = nil
	c.owner = nil
	c.tabs = nil
}
func closeEditorOpens() {
	if w := globalWorkbench; w != nil && w.opens != nil {
		w.opens.Close()
	}
}

func editorFileOpensPending(tabs *gui.TabWidget) bool {
	w := globalWorkbench
	return w != nil && w.tabs == tabs && w.opens != nil && w.opens.valid() && len(w.opens.pending) > 0
}
func cancelEditorFileOpens(tabs *gui.TabWidget) bool {
	w := globalWorkbench
	if w == nil || w.tabs != tabs {
		return false
	}
	return cancelOwnedEditorFileOpens(tabs, w.opens)
}

// Menus and modal palettes bind the controller they observed. A delayed action
// must not cancel a new controller installed for the same TabWidget.
func cancelOwnedEditorFileOpens(tabs *gui.TabWidget, c *editorOpenController) bool {
	if !c.valid() || c.tabs != tabs {
		return false
	}
	var pages []*restorePage
	for _, job := range c.pending {
		pages = append(pages, job.page)
	}
	c.intent = nil
	for _, p := range pages {
		if p != nil {
			idx := editorPageIndex(tabs, p)
			p.Close()
			if idx >= 0 {
				tabs.RemoveTab(idx)
			}
		}
	}
	c.startNext()
	return len(pages) > 0
}
