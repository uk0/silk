package gui

import (
	"sort"
	"time"
)

// A hierarchy owns at most one queued batch. Queue closures hold the revocable
// batch, not a captured tree: hiding/detaching a root releases all its entries.
// Entries are only changed layout ancestors, not every widget in a window.
type layoutBatch struct {
	root    *Widget
	entries []*layoutEntry
	next    int
	running bool
	queued  bool
}

type layoutEntry struct {
	batch  *layoutBatch
	widget *Widget
	parent *Widget
	depth  int
}

// RequestLayout coalesces a layout of this widget and its layout ancestors onto
// the next UI turn. It does not resize a native window to its preferred size.
// Like all widget mutation, this is UI-thread-only; workers must use Post.
// Existing InvalidateLayout/InvalidateParentLayout retain immediate semantics.
func (w *Widget) RequestLayout() { requestWidgetLayout(w.Self()) }

// InvalidateSizeHints notifies layout ancestors that this widget's preferred
// size changed. Custom SDK widgets should call it after changing their hints.
// It does not run getters, Layout or Resize inside the setter.
func (w *Widget) InvalidateSizeHints() {
	if independentLayoutRoot(w) {
		return // a native form/popup's parent is an owner, not its layout host
	}
	if parent := w.Parent(); parent != nil {
		requestWidgetLayout(parent)
	}
}

func layoutParent(w *Widget) *Widget {
	if w.parent != nil {
		return w.parent.NakedWidget()
	}
	return nil
}

func independentLayoutRoot(w *Widget) bool { return w.wt == WtForm || w.wt == WtPopup }

func requestWidgetLayout(widget IWidget) {
	assertUIThread("Widget.RequestLayout")
	if widget == nil {
		return
	}
	root, depth, visible, found := widget.NakedWidget(), 0, true, false
	for p := widget.NakedWidget(); p != nil; p = layoutParent(p) {
		if _, ok := p.self.(ILayout); ok {
			p.layoutDirty = true
			found = true
		}
		visible = visible && !p.hidden
		root = p
		if independentLayoutRoot(p) || p.parent == nil {
			break
		}
		depth++
	}
	// Construction and hidden trees retain dirty bits, but create no idle work.
	// A later Resize/Show installs geometry and resumes outstanding descendants.
	if !found || !visible || layoutDimension(root.w) == 0 || layoutDimension(root.h) == 0 {
		return
	}
	batch := root.pendingLayout
	post := batch == nil
	if post {
		batch = &layoutBatch{root: root}
		root.pendingLayout = batch
	}
	for p := widget.NakedWidget(); p != nil; p = layoutParent(p) {
		if _, ok := p.self.(ILayout); ok {
			entry := p.queuedLayout
			if entry == nil || entry.batch != batch {
				if entry != nil {
					entry.widget = nil // revoke an older owner/active batch
				}
				entry = &layoutEntry{batch: batch, widget: p, parent: layoutParent(p), depth: depth}
				p.queuedLayout = entry
				batch.entries = append(batch.entries, entry)
			}
		}
		if p == root {
			break
		}
		depth--
	}
	batch.schedule()
}

func (batch *layoutBatch) schedule() {
	root := batch.root
	if root != nil && !batch.queued && !batch.running &&
		(root.activeLayout == nil || root.activeLayout == batch) {
		batch.queued = true
		Post(batch.run)
	}
}

func (batch *layoutBatch) retire() {
	if root := batch.root; root != nil {
		if root.pendingLayout == batch {
			root.pendingLayout = nil
		}
		if root.activeLayout == batch && !batch.running {
			root.activeLayout = nil
		}
	}
	for _, entry := range batch.entries {
		if w := entry.widget; w != nil && w.queuedLayout == entry {
			w.queuedLayout = nil
		}
		entry.widget = nil
	}
	batch.entries, batch.root = nil, nil
}

func (batch *layoutBatch) run() {
	batch.queued = false
	root := batch.root
	if root == nil {
		return
	}
	if batch.running || root.activeLayout != nil && root.activeLayout != batch {
		// A Layout callback may enter a nested native modal pump. Never recurse
		// into that same hierarchy; other independent windows can still layout.
		// The active pass explicitly wakes its successor when it completes.
		// Self-posting here spins at 100% CPU inside a nested modal pump.
		return
	}
	if root.pendingLayout == batch {
		root.pendingLayout = nil
	}
	root.activeLayout = batch
	batch.running = true
	complete := true
	defer func() {
		batch.running = false
		if complete {
			if root.activeLayout == batch {
				root.activeLayout = nil
			}
			batch.retire()
			if pending := root.pendingLayout; pending != nil {
				pending.schedule()
			}
		} else {
			batch.schedule()
		}
	}()
	if root.hidden || layoutDimension(root.w) == 0 || layoutDimension(root.h) == 0 {
		return
	}
	if batch.next == 0 {
		sort.SliceStable(batch.entries, func(i, j int) bool { return batch.entries[i].depth < batch.entries[j].depth })
	}
	deadline := time.Now().Add(uiTaskTimeBudget)
	for work := 0; batch.next < len(batch.entries); work++ {
		if work >= uiTaskBatchLimit || work > 0 && !time.Now().Before(deadline) {
			// One custom Layout is non-preemptible; do not combine an unbounded
			// number of independent layouts into that one UI callback as well.
			complete = false
			return
		}
		entry := batch.entries[batch.next]
		batch.next++
		w := entry.widget
		if w == nil || w.queuedLayout != entry {
			continue
		}
		w.queuedLayout, entry.widget = nil, nil
		if !w.layoutDirty || w.hidden {
			continue
		}
		if layoutParent(w) != entry.parent {
			w.RequestLayout() // admit against current parent, never stale geometry
			continue
		}
		w.layoutNow()
	}
	if batch.root != nil {
		root.self.Update()
	}
}

// Used by the legacy synchronous entry points too. A resize that already laid
// out a queued descendant satisfies that entry without a duplicate traversal.
func (w *Widget) layoutNow() {
	if w.layoutRunning {
		w.RequestLayout() // a synchronous Resize/invalidator reentered Layout
		return
	}
	w.layoutDirty = false
	if layout, ok := w.self.(ILayout); ok {
		w.layoutRunning = true
		defer func() { w.layoutRunning = false }()
		layout.Layout()
	}
}

// These walks run no Layout/Resize callbacks. Hierarchy changes revoke entries
// before native/show callbacks, then admit dirty descendants under current owners.
func (w *Widget) cancelQueuedLayouts() {
	if w.pendingLayout != nil {
		w.pendingLayout.retire()
	}
	if w.activeLayout != nil {
		w.activeLayout.retire()
	}
	if entry := w.queuedLayout; entry != nil {
		entry.widget, w.queuedLayout = nil, nil
	}
	w.eachChild(func(_ int, child IWidget) bool { child.NakedWidget().cancelQueuedLayouts(); return true })
}

func (w *Widget) resumeDirtyLayouts() {
	if w.layoutDirty {
		w.RequestLayout()
	}
	w.eachChild(func(_ int, child IWidget) bool { child.NakedWidget().resumeDirtyLayouts(); return true })
}
