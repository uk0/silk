package ged

import "path/filepath"

// This file implements the *editor workspace* — the split-group model Qt
// Creator (and VS Code) put under their editor area, and the piece
// EditorTabs.ToggleSplit is missing.
//
// EditorTabs today fakes a split: it builds a second gui.CodeEditor,
// copies the active tab's text into it and keeps the two in sync with
// SetText on every change. That is a *clone*, not a second view of one
// document: two buffers, two undo stacks, two carets fighting over
// SetText, and no way to show a third file next to the first one.
//
// The model here is the other half of a real split: panes (editor
// groups), each holding an ordered list of open *document identities*
// and one focused entry. A document identity is just its path — the
// workspace never stores text, so a document shown in two panes is the
// same document by construction and the host keeps exactly one buffer
// per path. The workspace only answers the structural questions the host
// cannot answer itself: which panes exist, which documents each shows,
// which pane/document has focus, and — when a view closes — whether the
// document is still visible somewhere else or may be released.
//
// It deliberately depends on nothing but path/filepath: no gui, no
// widgets, no rendering. That keeps it fully unit-testable headless, and
// leaves pane geometry (splitPaneRects and friends) to the widget layer.

// SplitOrientation describes how a pane sits next to its predecessor.
//
// The names say what the user sees rather than which way the divider
// runs — EditorTabs' `splitVertical bool` means "side-by-side", which is
// exactly the confusion worth avoiding. When wiring this model to that
// widget: SplitSideBySide == splitVertical true.
type SplitOrientation int

const (
	// SplitSideBySide places the pane to the right of the previous one
	// (left | right). It is the zero value, matching the common case.
	SplitSideBySide SplitOrientation = iota
	// SplitStacked places the pane below the previous one (top / bottom).
	SplitStacked
)

// Pane is one editor group: the ordered list of documents it shows plus
// which of them is focused. It is a plain record — the host reads it to
// render a tab bar per pane — and all structural changes go through
// Workspace so index bookkeeping stays in one place.
type Pane struct {
	// ID is stable for the pane's lifetime and never reused. Pane
	// indices shift when panes are closed; IDs do not, so every
	// Workspace operation addresses panes by ID.
	ID int
	// Docs holds the open document identities (paths) in tab order.
	Docs []string
	// Active indexes Docs, or is -1 when the pane shows nothing.
	Active int
	// Orientation records how this pane was split off from the pane
	// before it in Workspace order. It is meaningless for the first
	// pane, which has no predecessor.
	Orientation SplitOrientation
}

// IndexOf returns the position of path in the pane, or -1.
func (p *Pane) IndexOf(path string) int {
	for i, d := range p.Docs {
		if d == path {
			return i
		}
	}
	return -1
}

// Has reports whether the pane shows a view of path.
func (p *Pane) Has(path string) bool { return p.IndexOf(path) >= 0 }

// ActiveDoc returns the focused document's path, or "" for an empty pane.
func (p *Pane) ActiveDoc() string {
	if p.Active < 0 || p.Active >= len(p.Docs) {
		return ""
	}
	return p.Docs[p.Active]
}

// add appends a view of path unless the pane already has one, and
// returns the index it lives at either way.
func (p *Pane) add(path string) int {
	if i := p.IndexOf(path); i >= 0 {
		return i
	}
	p.Docs = append(p.Docs, path)
	return len(p.Docs) - 1
}

// remove drops the view at index i and keeps Active pointing at a
// sensible neighbour (the next tab, or the last one, or nothing).
func (p *Pane) remove(i int) {
	if i < 0 || i >= len(p.Docs) {
		return
	}
	p.Docs = append(p.Docs[:i], p.Docs[i+1:]...)
	switch {
	case len(p.Docs) == 0:
		p.Active = -1
	case p.Active > i:
		p.Active--
	case p.Active == i && p.Active >= len(p.Docs):
		p.Active = len(p.Docs) - 1
	}
}

// Workspace is the tree of editor panes over one shared set of
// documents. Use NewWorkspace; the zero value also works and grows its
// first pane on the first operation.
type Workspace struct {
	panes  []*Pane
	active int // index into panes
	nextID int
}

// NewWorkspace returns a workspace with a single empty pane.
func NewWorkspace() *Workspace {
	w := new(Workspace)
	w.ensurePane()
	return w
}

// normDocPath canonicalises a document identity. Callers are expected to
// hand in absolute paths (EditorTabs.OpenFile already resolves them);
// cleaning here only collapses "a/./b" and "a/x/../b" spellings so the
// same file cannot end up as two identities. The empty path is kept
// empty — filepath.Clean would turn it into ".".
func normDocPath(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

// newPane mints a pane with the next free ID.
func (w *Workspace) newPane(o SplitOrientation) *Pane {
	w.nextID++
	return &Pane{ID: w.nextID, Active: -1, Orientation: o}
}

// ensurePane guarantees at least one pane and a valid active index, then
// returns the active pane. Only mutating operations call it, so getters
// stay side-effect free.
func (w *Workspace) ensurePane() *Pane {
	if len(w.panes) == 0 {
		w.panes = append(w.panes, w.newPane(SplitSideBySide))
		w.active = 0
	}
	if w.active < 0 || w.active >= len(w.panes) {
		w.active = 0
	}
	return w.panes[w.active]
}

// PaneCount returns the number of panes.
func (w *Workspace) PaneCount() int { return len(w.panes) }

// Panes returns the panes in layout order (first pane first). The slice
// is a copy, the panes are shared.
func (w *Workspace) Panes() []*Pane {
	out := make([]*Pane, len(w.panes))
	copy(out, w.panes)
	return out
}

// PaneByID returns the pane with the given ID, or nil.
func (w *Workspace) PaneByID(id int) *Pane {
	if i := w.paneIndex(id); i >= 0 {
		return w.panes[i]
	}
	return nil
}

// paneIndex returns the layout position of the pane with the given ID, or -1.
func (w *Workspace) paneIndex(id int) int {
	for i, p := range w.panes {
		if p.ID == id {
			return i
		}
	}
	return -1
}

// ActivePane returns the focused pane, or nil when there are none.
func (w *Workspace) ActivePane() *Pane {
	if w.active < 0 || w.active >= len(w.panes) {
		return nil
	}
	return w.panes[w.active]
}

// ActivePaneID returns the focused pane's ID, or 0 (never a valid ID)
// when there is no pane.
func (w *Workspace) ActivePaneID() int {
	if p := w.ActivePane(); p != nil {
		return p.ID
	}
	return 0
}

// SetActivePane focuses a pane by ID and reports whether it existed.
func (w *Workspace) SetActivePane(id int) bool {
	i := w.paneIndex(id)
	if i < 0 {
		return false
	}
	w.active = i
	return true
}

// ActiveDoc returns the path of the document focused in the active pane,
// or "" when nothing is open there.
func (w *Workspace) ActiveDoc() string {
	if p := w.ActivePane(); p != nil {
		return p.ActiveDoc()
	}
	return ""
}

// ViewCount returns how many panes currently show path. This is the
// document registry the host needs: > 1 means several views share one
// buffer, 0 means the document may be released.
func (w *Workspace) ViewCount(path string) int {
	path = normDocPath(path)
	if path == "" {
		return 0
	}
	n := 0
	for _, p := range w.panes {
		if p.Has(path) {
			n++
		}
	}
	return n
}

// IsOpen reports whether any pane shows path.
func (w *Workspace) IsOpen(path string) bool { return w.ViewCount(path) > 0 }

// OpenDocs returns every distinct open document identity, in pane order.
// One entry per document however many views it has.
func (w *Workspace) OpenDocs() []string {
	var out []string
	seen := make(map[string]bool)
	for _, p := range w.panes {
		for _, d := range p.Docs {
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	return out
}

// OpenDoc opens path in the active pane, or just focuses it when that
// pane already shows it. It reports whether a new view was created —
// false when the document was merely re-focused, or when path is empty.
// A document already open in *another* pane still gets a new view here:
// two views, one shared document.
func (w *Workspace) OpenDoc(path string) bool {
	path = normDocPath(path)
	if path == "" {
		return false
	}
	p := w.ensurePane()
	if i := p.IndexOf(path); i >= 0 {
		p.Active = i
		return false
	}
	p.Active = p.add(path)
	return true
}

// SplitActive splits the active pane, inserting a sibling directly after
// it that shows the *same* document — the same identity, not a copy of
// its text. The new pane takes focus (as in Qt Creator and VS Code) and
// its ID is returned; splitting an empty pane yields an empty sibling.
func (w *Workspace) SplitActive(o SplitOrientation) int {
	src := w.ensurePane()
	pane := w.newPane(o)
	if doc := src.ActiveDoc(); doc != "" {
		pane.Docs = []string{doc}
		pane.Active = 0
	}
	at := w.active + 1
	w.panes = append(w.panes, nil)
	copy(w.panes[at+1:], w.panes[at:])
	w.panes[at] = pane
	w.active = at
	return pane.ID
}

// MoveDocToPane moves the view of path out of fromPaneID and into
// toPaneID, focusing it there and focusing the target pane. It reports
// whether the move happened. Moving onto a pane that already shows the
// document collapses the two views into one, and the source pane is left
// in place even when it ends up empty — use ClosePane to drop it.
func (w *Workspace) MoveDocToPane(path string, fromPaneID, toPaneID int) bool {
	path = normDocPath(path)
	from, to := w.PaneByID(fromPaneID), w.PaneByID(toPaneID)
	if path == "" || from == nil || to == nil || !from.Has(path) {
		return false
	}
	if from != to {
		from.remove(from.IndexOf(path))
	}
	to.Active = to.add(path)
	w.active = w.paneIndex(to.ID)
	return true
}

// DuplicateViewToOtherPane adds a second view of an already-open
// document to toPaneID and focuses it there, leaving the original view
// untouched. It reports false when the target pane is unknown or the
// document is not open anywhere — opening a fresh document is OpenDoc's
// job, this only adds a view of a document the host already holds.
func (w *Workspace) DuplicateViewToOtherPane(path string, toPaneID int) bool {
	path = normDocPath(path)
	to := w.PaneByID(toPaneID)
	if path == "" || to == nil || !w.IsOpen(path) {
		return false
	}
	to.Active = to.add(path)
	w.active = w.paneIndex(to.ID)
	return true
}

// CloseDoc closes one view — path in pane paneID — and reports whether
// the document is still open somewhere else (stillOpen) so the host
// knows when to release its buffer, plus whether a view was actually
// closed (ok). stillOpen is the truth about the workspace after the
// call, so it stays meaningful even when ok is false.
func (w *Workspace) CloseDoc(paneID int, path string) (stillOpen, ok bool) {
	path = normDocPath(path)
	p := w.PaneByID(paneID)
	if path == "" || p == nil {
		return w.IsOpen(path), false
	}
	i := p.IndexOf(path)
	if i < 0 {
		return w.IsOpen(path), false
	}
	p.remove(i)
	return w.IsOpen(path), true
}

// ClosePane removes a pane and merges its documents into the neighbour
// it collapses into (the pane before it, or the one after it for the
// first pane), so closing a split never closes the files it held. The
// closed pane's focused document keeps focus in the neighbour, and the
// neighbour takes over focus if the closed pane had it. The last
// remaining pane cannot be closed: it reports false, as does an unknown ID.
func (w *Workspace) ClosePane(paneID int) bool {
	if len(w.panes) <= 1 {
		return false
	}
	i := w.paneIndex(paneID)
	if i < 0 {
		return false
	}
	closed := w.panes[i]

	// The neighbour keeps its index after the removal when it precedes
	// the closed pane; the follower slides down into i.
	nb := i - 1
	if i == 0 {
		nb = i + 1
	}
	neighbour := w.panes[nb]
	focus := closed.ActiveDoc()
	for _, d := range closed.Docs {
		neighbour.add(d)
	}
	if focus != "" {
		neighbour.Active = neighbour.IndexOf(focus)
	}

	w.panes = append(w.panes[:i], w.panes[i+1:]...)
	switch {
	case w.active == i:
		w.active = w.paneIndex(neighbour.ID)
	case w.active > i:
		w.active--
	}
	return true
}
