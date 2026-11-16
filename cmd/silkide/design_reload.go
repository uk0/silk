package main

import (
	"context"
	"sync/atomic"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/hotreload"
	"github.com/uk0/silk/i18n"
)

type designReloadPath struct{ path, key string }

// UI-owned association for retiring a watcher even after its workbench changed.
var globalDesignReloadBridge *designReloadBridge

// The watcher retains this bounded, revocable mailbox, never a canvas/frame.
// A burst holds one latest path and at most one queued UI wakeup. Source reads
// begin only after UI admission has captured the current scene/edit identity.
type designReloadBridge struct {
	controller atomic.Pointer[designOpenController]
	reloader   atomic.Pointer[hotreload.Reloader]
	target     atomic.Pointer[designReloadPath]
	latest     atomic.Pointer[designReloadPath]
	queued     atomic.Bool
	post       func(func())
}

func init() {
	i18n.AddMany("zh", map[string]string{
		"Reloading design %s in background": "正在后台重新加载设计 %s",
	})
}

func newDesignReloadBridge(c *designOpenController) *designReloadBridge {
	b := &designReloadBridge{post: c.settings.post}
	b.controller.Store(c)
	b.bind(c.canvas.GedScene().Filename())
	return b
}

// UI-only: a file retarget invalidates old queued notifications without I/O.
func (b *designReloadBridge) bind(path string) {
	path = canonicalEditorPath(path)
	next := &designReloadPath{path, workspaceSearchKey(path)}
	previous := b.target.Swap(next)
	if previous == nil || previous.key != next.key {
		b.latest.Store(nil)
	}
}

// Worker-only data/atomic operations; no UI state reads and no source I/O.
func (b *designReloadBridge) signal(path string) error {
	if b.controller.Load() == nil {
		return nil
	}
	target := b.target.Load()
	if target == nil || target.path == "" || workspaceSearchKey(path) != target.key {
		return nil
	}
	b.latest.Store(&designReloadPath{path, target.key})
	if b.queued.CompareAndSwap(false, true) {
		b.post(b.deliver)
	}
	return nil
}

func (b *designReloadBridge) deliver() {
	// Clear before exchanging the value: a simultaneous signal either joins
	// this batch or posts the next one, never gets stranded behind a true flag.
	b.queued.Store(false)
	value := b.latest.Swap(nil)
	c, owner, target := b.controller.Load(), b.reloader.Load(), b.target.Load()
	if value == nil || target == nil || value.key != target.key || c == nil || !c.valid() || c.reloadBridge != b {
		return
	}
	_, _ = reloadOwnedCleanDesign(owner, c.canvas, value.path)
}

func (b *designReloadBridge) close() {
	b.controller.Store(nil)
	b.reloader.Store(nil)
	b.target.Store(nil)
	b.latest.Store(nil)
}

// UI-only and intentionally not an explicit open: do not show a recovery or
// discard prompt, reveal the canvas, set focus, or replace a pending user open.
func (c *designOpenController) queueReload(owner *hotreload.Reloader, path string) bool {
	if !c.valid() || transitionBusy() || c.admitting != nil || owner == nil || globalReloader != owner || owner.Stats().Closed {
		return false
	}
	scene := c.canvas.GedScene()
	path = canonicalEditorPath(path)
	if scene == nil || path == "" || scene.Filename() == "" || !scene.IsClean() || c.canvas.HasCapture() ||
		workspaceSearchKey(canonicalEditorPath(scene.Filename())) != workspaceSearchKey(path) || savePathBusy(path) || saveBatchAffects(path) || rejectBusyFile(path) {
		return false
	}
	if c.pending != nil && !c.pending.reload {
		return false // explicit user intent always wins over a watcher event
	}
	c.cancelPending()
	id := new(designOpenToken)
	c.intent = id
	ctx, cancel := context.WithTimeout(c.ctx, c.settings.timeout)
	j := &designOpenJob{id: id, box: new(designOpenMailbox), path: path, filename: scene.Filename(), scene: scene,
		focus: gui.CurrentFocusWidget(), mode: documentMode(c.owner.tabs, c.canvas), ctx: ctx, cancel: cancel,
		reload: true, reloadOwner: owner}
	c.admit(j)
	return true
}

func (c *designOpenController) applyReload(j *designOpenJob, doc *core.TDoc, version core.FileFingerprint) {
	scene, path, id := j.scene, j.filename, j.id
	c.retire(j) // release our edit observer before the owned load notifies others
	if !c.valid() || c.intent != id || c.canvas.GedScene() != scene || scene.Filename() != path {
		return
	}
	changed, err := reloadCleanDesign(scene, path, doc)
	if !c.valid() || c.intent != id || c.canvas.GedScene() != scene || scene.Filename() != path {
		return
	}
	if err != nil {
		logEvent(ged.LogWarn, "Design reload: "+path+": "+err.Error())
	} else if scene.IsClean() {
		bindDesignDisk(scene, path, version)
		if changed {
			logEvent(ged.LogInfo, "Reloaded "+path)
		}
	}
}

func (c *designOpenController) detachReloader(owner *hotreload.Reloader) {
	if c == nil {
		return
	}
	if b := c.reloadBridge; b != nil && b.reloader.Load() == owner {
		b.close()
		c.reloadBridge = nil
	}
	if j := c.pending; j != nil && j.reload && j.reloadOwner == owner {
		c.retire(j)
		if c.intent == j.id {
			c.intent = nil
		}
	}
}
