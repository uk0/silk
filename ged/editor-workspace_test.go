package ged

import "testing"

// These tests cover the split-group model only: no widgets, no GL, no
// files on disk. Document identities are plain paths, so a "document"
// here is just a string the workspace hands back out.

// wsDocsOf returns the document list of a pane, failing the test when the
// pane ID is unknown.
func wsDocsOf(t *testing.T, w *Workspace, paneID int) []string {
	t.Helper()
	p := w.PaneByID(paneID)
	if p == nil {
		t.Fatalf("pane %d not found", paneID)
	}
	return p.Docs
}

// wsSameDocs compares a pane's document list against the expected order.
func wsSameDocs(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("docs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("docs = %v, want %v", got, want)
		}
	}
}

// TestWorkspaceOpenDocFocusesExisting: opening a document adds one view
// to the active pane; re-opening it only moves focus, it does not create
// a second tab for the same file in the same pane.
func TestWorkspaceOpenDocFocusesExisting(t *testing.T) {
	w := NewWorkspace()
	if w.PaneCount() != 1 {
		t.Fatalf("PaneCount = %d, want 1 pane in a fresh workspace", w.PaneCount())
	}
	if w.ActiveDoc() != "" {
		t.Fatalf("ActiveDoc = %q, want empty", w.ActiveDoc())
	}

	if !w.OpenDoc("/p/a.go") {
		t.Fatal("OpenDoc(/p/a.go) = false, want true (new view)")
	}
	if !w.OpenDoc("/p/b.go") {
		t.Fatal("OpenDoc(/p/b.go) = false, want true (new view)")
	}
	if w.ActiveDoc() != "/p/b.go" {
		t.Fatalf("ActiveDoc = %q, want /p/b.go", w.ActiveDoc())
	}

	if w.OpenDoc("/p/a.go") {
		t.Error("re-OpenDoc(/p/a.go) = true, want false (focus, not a new view)")
	}
	wsDocs := wsDocsOf(t, w, w.ActivePaneID())
	wsSameDocs(t, wsDocs, "/p/a.go", "/p/b.go")
	if w.ActiveDoc() != "/p/a.go" {
		t.Errorf("ActiveDoc = %q, want /p/a.go after re-open", w.ActiveDoc())
	}
	if n := w.ViewCount("/p/a.go"); n != 1 {
		t.Errorf("ViewCount(/p/a.go) = %d, want 1", n)
	}
	if got := w.OpenDocs(); len(got) != 2 {
		t.Errorf("OpenDocs = %v, want 2 distinct documents", got)
	}
	if w.OpenDoc("") {
		t.Error("OpenDoc(\"\") = true, want false")
	}
}

// TestWorkspaceSplitSharesDocument: a split creates a sibling pane that
// shows the SAME document identity (two views, one document) and takes
// focus. This is the behaviour EditorTabs.ToggleSplit fakes by cloning
// text into a second editor.
func TestWorkspaceSplitSharesDocument(t *testing.T) {
	w := NewWorkspace()
	first := w.ActivePaneID()
	w.OpenDoc("/p/a.go")

	second := w.SplitActive(SplitSideBySide)
	if w.PaneCount() != 2 {
		t.Fatalf("PaneCount = %d, want 2 after SplitActive", w.PaneCount())
	}
	if second == first || second == 0 {
		t.Fatalf("split pane ID = %d, want a fresh ID (first = %d)", second, first)
	}
	if w.ActivePaneID() != second {
		t.Errorf("ActivePaneID = %d, want the new pane %d", w.ActivePaneID(), second)
	}
	if w.ActiveDoc() != "/p/a.go" {
		t.Errorf("ActiveDoc = %q, want /p/a.go in the new pane", w.ActiveDoc())
	}
	wsSameDocs(t, wsDocsOf(t, w, first), "/p/a.go")
	wsSameDocs(t, wsDocsOf(t, w, second), "/p/a.go")
	if n := w.ViewCount("/p/a.go"); n != 2 {
		t.Errorf("ViewCount(/p/a.go) = %d, want 2 views of one document", n)
	}
	if got := w.OpenDocs(); len(got) != 1 || got[0] != "/p/a.go" {
		t.Errorf("OpenDocs = %v, want one shared document", got)
	}

	// Pane order: the sibling is inserted directly after its source.
	panes := w.Panes()
	if panes[0].ID != first || panes[1].ID != second {
		t.Errorf("pane order = [%d %d], want [%d %d]", panes[0].ID, panes[1].ID, first, second)
	}
}

// TestWorkspaceSplitOrientationRecorded: each split remembers how it was
// made, and splitting an empty pane yields an empty sibling.
func TestWorkspaceSplitOrientationRecorded(t *testing.T) {
	w := NewWorkspace()
	w.OpenDoc("/p/a.go")

	side := w.SplitActive(SplitSideBySide)
	if o := w.PaneByID(side).Orientation; o != SplitSideBySide {
		t.Errorf("side-by-side pane Orientation = %v, want SplitSideBySide", o)
	}
	stacked := w.SplitActive(SplitStacked)
	if o := w.PaneByID(stacked).Orientation; o != SplitStacked {
		t.Errorf("stacked pane Orientation = %v, want SplitStacked", o)
	}

	// An empty pane splits into another empty pane.
	empty := NewWorkspace()
	id := empty.SplitActive(SplitStacked)
	p := empty.PaneByID(id)
	if len(p.Docs) != 0 || p.Active != -1 {
		t.Errorf("split of an empty pane = %v (Active %d), want empty pane with Active -1", p.Docs, p.Active)
	}
}

// TestWorkspaceCloseDocKeepsSharedDocOpen: closing one view of a shared
// document leaves the other view (and the host's buffer) alive; closing
// the last view reports the document as released.
func TestWorkspaceCloseDocKeepsSharedDocOpen(t *testing.T) {
	w := NewWorkspace()
	first := w.ActivePaneID()
	w.OpenDoc("/p/a.go")
	second := w.SplitActive(SplitSideBySide)

	stillOpen, ok := w.CloseDoc(first, "/p/a.go")
	if !ok {
		t.Fatal("CloseDoc(first) ok = false, want true")
	}
	if !stillOpen {
		t.Error("CloseDoc(first) stillOpen = false, want true (the split still shows it)")
	}
	if n := w.ViewCount("/p/a.go"); n != 1 {
		t.Errorf("ViewCount = %d, want 1 remaining view", n)
	}
	if p := w.PaneByID(first); len(p.Docs) != 0 || p.Active != -1 {
		t.Errorf("first pane = %v (Active %d), want empty with Active -1", p.Docs, p.Active)
	}
	if w.PaneCount() != 2 {
		t.Errorf("PaneCount = %d, want 2 (an emptied pane stays until ClosePane)", w.PaneCount())
	}

	stillOpen, ok = w.CloseDoc(second, "/p/a.go")
	if !ok {
		t.Fatal("CloseDoc(second) ok = false, want true")
	}
	if stillOpen {
		t.Error("CloseDoc(second) stillOpen = true, want false (last view closed, release it)")
	}
	if w.IsOpen("/p/a.go") {
		t.Error("IsOpen(/p/a.go) = true after closing every view")
	}
	if w.ActiveDoc() != "" {
		t.Errorf("ActiveDoc = %q, want empty", w.ActiveDoc())
	}

	// Closing something that is not there changes nothing and says so.
	if _, ok := w.CloseDoc(second, "/p/a.go"); ok {
		t.Error("CloseDoc of an already-closed view ok = true, want false")
	}
	if _, ok := w.CloseDoc(9999, "/p/a.go"); ok {
		t.Error("CloseDoc with an unknown pane ok = true, want false")
	}
}

// TestWorkspaceCloseDocReportsUnaffectedDoc: a failed close still reports
// the document's real state, so the host never releases a live buffer.
func TestWorkspaceCloseDocReportsUnaffectedDoc(t *testing.T) {
	w := NewWorkspace()
	w.OpenDoc("/p/a.go")
	stillOpen, ok := w.CloseDoc(9999, "/p/a.go")
	if ok {
		t.Error("ok = true for an unknown pane, want false")
	}
	if !stillOpen {
		t.Error("stillOpen = false, want true — the document is untouched and still open")
	}
}

// TestWorkspaceActiveBookkeeping: closing views inside a pane keeps
// focus on a neighbouring tab and never dangles past the end.
func TestWorkspaceActiveBookkeeping(t *testing.T) {
	w := NewWorkspace()
	id := w.ActivePaneID()
	w.OpenDoc("/p/a.go")
	w.OpenDoc("/p/b.go")
	w.OpenDoc("/p/c.go")
	if w.ActiveDoc() != "/p/c.go" {
		t.Fatalf("ActiveDoc = %q, want /p/c.go", w.ActiveDoc())
	}

	// Closing the last tab (which has focus) falls back to the one before.
	if _, ok := w.CloseDoc(id, "/p/c.go"); !ok {
		t.Fatal("CloseDoc(/p/c.go) failed")
	}
	if w.ActiveDoc() != "/p/b.go" {
		t.Errorf("ActiveDoc = %q, want /p/b.go", w.ActiveDoc())
	}

	// Closing a tab before the focused one shifts the index, not the focus.
	if _, ok := w.CloseDoc(id, "/p/a.go"); !ok {
		t.Fatal("CloseDoc(/p/a.go) failed")
	}
	if w.ActiveDoc() != "/p/b.go" {
		t.Errorf("ActiveDoc = %q, want /p/b.go to keep focus", w.ActiveDoc())
	}
	if p := w.PaneByID(id); p.Active != 0 {
		t.Errorf("Active = %d, want 0 after the earlier tab closed", p.Active)
	}

	// SetActivePane only accepts real panes.
	if w.SetActivePane(9999) {
		t.Error("SetActivePane(9999) = true, want false")
	}
	if w.ActivePaneID() != id {
		t.Errorf("ActivePaneID = %d, want %d unchanged", w.ActivePaneID(), id)
	}
}

// TestWorkspaceMoveDocToPane: a document moves between panes (tab drag),
// focus follows it, and the source pane's own focus is repaired.
func TestWorkspaceMoveDocToPane(t *testing.T) {
	w := NewWorkspace()
	first := w.ActivePaneID()
	w.OpenDoc("/p/a.go")
	w.OpenDoc("/p/b.go")
	second := w.SplitActive(SplitSideBySide) // shows /p/b.go

	if !w.MoveDocToPane("/p/a.go", first, second) {
		t.Fatal("MoveDocToPane = false, want true")
	}
	wsSameDocs(t, wsDocsOf(t, w, first), "/p/b.go")
	wsSameDocs(t, wsDocsOf(t, w, second), "/p/b.go", "/p/a.go")
	if w.ActivePaneID() != second || w.ActiveDoc() != "/p/a.go" {
		t.Errorf("focus = pane %d doc %q, want pane %d doc /p/a.go", w.ActivePaneID(), w.ActiveDoc(), second)
	}
	if p := w.PaneByID(first); p.ActiveDoc() != "/p/b.go" {
		t.Errorf("source pane ActiveDoc = %q, want /p/b.go", p.ActiveDoc())
	}
	if n := w.ViewCount("/p/a.go"); n != 1 {
		t.Errorf("ViewCount(/p/a.go) = %d, want 1 (moved, not copied)", n)
	}

	// Moving a document onto the pane that already shows it collapses the
	// two views into the single existing one.
	if !w.MoveDocToPane("/p/b.go", first, second) {
		t.Fatal("MoveDocToPane(/p/b.go) = false, want true")
	}
	if n := w.ViewCount("/p/b.go"); n != 1 {
		t.Errorf("ViewCount(/p/b.go) = %d, want 1 after merging onto an existing view", n)
	}
	wsSameDocs(t, wsDocsOf(t, w, first))
	if w.ActiveDoc() != "/p/b.go" {
		t.Errorf("ActiveDoc = %q, want /p/b.go", w.ActiveDoc())
	}

	// Bad arguments are refused.
	if w.MoveDocToPane("/p/nope.go", second, first) {
		t.Error("moving a document the source pane does not have = true, want false")
	}
	if w.MoveDocToPane("/p/b.go", second, 9999) {
		t.Error("moving into an unknown pane = true, want false")
	}
}

// TestWorkspaceDuplicateViewToOtherPane: a second view of an open
// document appears in another pane while the original view stays put.
func TestWorkspaceDuplicateViewToOtherPane(t *testing.T) {
	w := NewWorkspace()
	first := w.ActivePaneID()
	w.OpenDoc("/p/a.go")
	second := w.SplitActive(SplitSideBySide)
	w.OpenDoc("/p/c.go") // into the new pane

	if !w.DuplicateViewToOtherPane("/p/c.go", first) {
		t.Fatal("DuplicateViewToOtherPane = false, want true")
	}
	wsSameDocs(t, wsDocsOf(t, w, first), "/p/a.go", "/p/c.go")
	wsSameDocs(t, wsDocsOf(t, w, second), "/p/a.go", "/p/c.go")
	if n := w.ViewCount("/p/c.go"); n != 2 {
		t.Errorf("ViewCount(/p/c.go) = %d, want 2 views", n)
	}
	if w.ActivePaneID() != first || w.ActiveDoc() != "/p/c.go" {
		t.Errorf("focus = pane %d doc %q, want pane %d doc /p/c.go", w.ActivePaneID(), w.ActiveDoc(), first)
	}

	// Duplicating again is idempotent: still one view per pane.
	if !w.DuplicateViewToOtherPane("/p/c.go", first) {
		t.Fatal("second DuplicateViewToOtherPane = false, want true")
	}
	wsSameDocs(t, wsDocsOf(t, w, first), "/p/a.go", "/p/c.go")

	// A document nobody has open cannot be *duplicated* — that is OpenDoc.
	if w.DuplicateViewToOtherPane("/p/ghost.go", second) {
		t.Error("duplicating a closed document = true, want false")
	}
	if w.DuplicateViewToOtherPane("/p/c.go", 9999) {
		t.Error("duplicating into an unknown pane = true, want false")
	}
}

// TestWorkspaceClosePaneMerges: closing a pane hands its documents to the
// neighbour instead of closing the files, keeps the focused document
// focused, and moves the active pane along.
func TestWorkspaceClosePaneMerges(t *testing.T) {
	w := NewWorkspace()
	first := w.ActivePaneID()
	w.OpenDoc("/p/a.go")
	w.OpenDoc("/p/b.go")
	second := w.SplitActive(SplitSideBySide) // shows /p/b.go
	w.OpenDoc("/p/c.go")                     // second pane: [b, c], focus c

	if !w.ClosePane(second) {
		t.Fatal("ClosePane = false, want true")
	}
	if w.PaneCount() != 1 {
		t.Fatalf("PaneCount = %d, want 1", w.PaneCount())
	}
	if w.PaneByID(second) != nil {
		t.Error("closed pane still resolvable by ID")
	}
	// /p/b.go was already in the neighbour (one view survives), /p/c.go moves over.
	wsSameDocs(t, wsDocsOf(t, w, first), "/p/a.go", "/p/b.go", "/p/c.go")
	if n := w.ViewCount("/p/b.go"); n != 1 {
		t.Errorf("ViewCount(/p/b.go) = %d, want 1 after the merge", n)
	}
	if w.ActivePaneID() != first {
		t.Errorf("ActivePaneID = %d, want the surviving pane %d", w.ActivePaneID(), first)
	}
	if w.ActiveDoc() != "/p/c.go" {
		t.Errorf("ActiveDoc = %q, want /p/c.go to keep focus through the merge", w.ActiveDoc())
	}
	if w.IsOpen("/p/c.go") != true {
		t.Error("closing a pane must not close its documents")
	}
}

// TestWorkspaceClosePaneFirstMergesForward: the first pane has no
// predecessor, so it collapses into the pane after it, and the active
// index is repaired when a pane before the active one disappears.
func TestWorkspaceClosePaneFirstMergesForward(t *testing.T) {
	w := NewWorkspace()
	first := w.ActivePaneID()
	w.OpenDoc("/p/a.go")
	second := w.SplitActive(SplitSideBySide)
	w.OpenDoc("/p/c.go") // second pane: [a, c], focus c

	if !w.ClosePane(first) {
		t.Fatal("ClosePane(first) = false, want true")
	}
	if w.PaneCount() != 1 || w.ActivePaneID() != second {
		t.Fatalf("panes = %d, active = %d, want 1 pane %d", w.PaneCount(), w.ActivePaneID(), second)
	}
	wsSameDocs(t, wsDocsOf(t, w, second), "/p/a.go", "/p/c.go")
	if w.ActiveDoc() != "/p/a.go" {
		t.Errorf("ActiveDoc = %q, want /p/a.go (the closed pane's focus wins)", w.ActiveDoc())
	}
}

// TestWorkspaceClosePaneRefusals: the last pane survives, unknown IDs are
// refused, and an empty pane merges without stealing focus.
func TestWorkspaceClosePaneRefusals(t *testing.T) {
	w := NewWorkspace()
	w.OpenDoc("/p/a.go")
	if w.ClosePane(w.ActivePaneID()) {
		t.Error("ClosePane on the only pane = true, want false")
	}
	if w.PaneCount() != 1 || w.ActiveDoc() != "/p/a.go" {
		t.Errorf("workspace changed by a refused ClosePane: panes=%d doc=%q", w.PaneCount(), w.ActiveDoc())
	}

	first := w.ActivePaneID()
	second := w.SplitActive(SplitSideBySide)
	if w.ClosePane(9999) {
		t.Error("ClosePane(9999) = true, want false")
	}
	// Empty out the second pane, then close it: the neighbour keeps its own focus.
	w.CloseDoc(second, "/p/a.go")
	w.SetActivePane(first)
	if !w.ClosePane(second) {
		t.Fatal("ClosePane of an empty pane = false, want true")
	}
	if w.PaneCount() != 1 || w.ActivePaneID() != first || w.ActiveDoc() != "/p/a.go" {
		t.Errorf("after closing an empty pane: panes=%d active=%d doc=%q", w.PaneCount(), w.ActivePaneID(), w.ActiveDoc())
	}
}

// TestWorkspaceZeroValueAndPathIdentity: the zero value grows its first
// pane on demand, and equivalent path spellings are one document.
func TestWorkspaceZeroValueAndPathIdentity(t *testing.T) {
	var w Workspace
	if w.PaneCount() != 0 || w.ActivePane() != nil || w.ActiveDoc() != "" {
		t.Fatalf("zero value should start empty: panes=%d", w.PaneCount())
	}
	if !w.OpenDoc("/p/x/../a.go") {
		t.Fatal("OpenDoc on a zero-value workspace = false, want true")
	}
	if w.PaneCount() != 1 {
		t.Fatalf("PaneCount = %d, want 1 pane created on demand", w.PaneCount())
	}
	if w.ActiveDoc() != "/p/a.go" {
		t.Errorf("ActiveDoc = %q, want the cleaned path /p/a.go", w.ActiveDoc())
	}
	if w.OpenDoc("/p/a.go") {
		t.Error("the same file spelled differently opened a second view, want one identity")
	}
	if n := w.ViewCount("/p/./a.go"); n != 1 {
		t.Errorf("ViewCount(/p/./a.go) = %d, want 1 (same identity)", n)
	}
	if w.ViewCount("") != 0 || w.IsOpen("") {
		t.Error("the empty path must never count as an open document")
	}
}
