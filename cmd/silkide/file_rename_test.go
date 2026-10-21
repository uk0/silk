package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/internal/fileops"
)

func renameWorkbench(t *testing.T) *workbench {
	t.Helper()
	w := testWorkbench(t)
	oldLSP, oldPrefs, oldTxn := globalLSP, globalPrefs, activeFileRename
	oldProblems, oldRefs := globalProblems, globalReferencesPanel
	oldCanvas, oldDebug, oldAttempt, oldBusy := globalCanvas, globalDebug, globalDebugAttempt, macroBusy
	globalLSP = nil
	globalPrefs = nil
	activeFileRename = nil
	globalCanvas = nil
	globalDebug = nil
	globalDebugAttempt = nil
	macroBusy = false
	globalProblems = ged.NewProblemsPanel()
	globalReferencesPanel = ged.NewReferencesPanel()
	keepCoverageGlobals(t)
	globalCoverage = nil
	t.Cleanup(func() {
		globalLSP, globalPrefs, activeFileRename = oldLSP, oldPrefs, oldTxn
		globalProblems, globalReferencesPanel = oldProblems, oldRefs
		globalCanvas, globalDebug, globalDebugAttempt, macroBusy = oldCanvas, oldDebug, oldAttempt, oldBusy
	})
	return w
}
func TestEditorRenameDirectoryPreservesDirtyBuffersAndSaveIdentity(t *testing.T) {
	w := renameWorkbench(t)
	root := t.TempDir()
	old := filepath.Join(root, "source")
	next := filepath.Join(root, "renamed")
	if err := os.MkdirAll(filepath.Join(old, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	a := writeFile(t, old, "main.go", "package p\n")
	b := writeFile(t, filepath.Join(old, "nested"), "macro.silkgo", "package p\n//silk:macro M()\nvar V int\n//silk:end\n")
	untouched := writeFile(t, root, "source-sibling.go", "package p\n")
	for _, p := range []string{a, b, untouched} {
		if !openFileInEditor(w.tabs, p) {
			t.Fatal("open", p)
		}
	}
	ed, macroEd := openEditors[a], openEditors[b]
	ed.ReplaceAllText("package p\nvar Unsaved = 1\n")
	ed.ScrollToLineCol(1, 4)
	macroEd.ReplaceAllText(macroEd.Text() + "//silk:expand M()\n")
	revision := ed.TextRevision()
	cursorLine, cursorCol := ed.CursorLine(), ed.CursorCol()
	globalReferencesPanel.SetLocations([]ged.ReferenceLoc{{File: a, Line: 2}, {File: untouched, Line: 1}})
	globalProblems.SetProblems([]ged.Problem{{File: a, Message: "build failure", Source: ged.BuildSource}, {File: untouched, Message: "keep", Source: ged.BuildSource}})
	plan, err := prepareEditorRename(w.tabs, w.canvas, old, next)
	if err != nil {
		t.Fatal(err)
	}
	if saveEditorToDisk(ed, a) || closeEditorTab(w.tabs, editorTabIndex(w.tabs, ed)) || canQuit(w.canvas) {
		t.Fatal("reserved operation was allowed")
	}
	if openFileInEditor(w.tabs, filepath.Join(next, "main.go")) {
		t.Fatal("opened destination during rename")
	}
	if !fileRenameAffects(b) || fileRenameAffects(untouched) {
		t.Fatal("reservation path boundaries")
	}
	if _, err := prepareEditorRename(w.tabs, w.canvas, untouched, untouched+"2"); diagnostic.CodeOf(err) != diagnostic.FileOperationBusy {
		t.Fatal("overlapping rename accepted", err)
	}
	// Typing remains enabled, and commit must take the latest buffer, not a
	// snapshot captured when the rename dialog was accepted.
	macroEd.ReplaceAllText(macroEd.Text() + "// typed while moving\n")
	latest := macroEd.Text()
	macroRev := macroEd.TextRevision()
	awaitEditorRenamePlan(t, plan)
	if err := fileops.RenameNoReplace(old, next); err != nil {
		t.Fatal(err)
	}
	plan.Complete(nil)
	plan.Complete(nil)
	na, nb := filepath.Join(next, "main.go"), filepath.Join(next, "nested", "macro.silkgo")
	if activeFileRename != nil || openEditors[a] != nil || openEditors[b] != nil || openEditors[na] != ed || openEditors[nb] != macroEd || openEditors[untouched] == nil {
		t.Fatal("map migration")
	}
	if ed.FilePath() != na || ed.TextRevision() != revision || ed.CursorLine() != cursorLine || ed.CursorCol() != cursorCol || !ed.Modified() {
		t.Fatal("editor recreated or lost state")
	}
	if macroEd.Text() != latest || macroEd.TextRevision() != macroRev || macroEditors[nb] == nil || macroEditors[b] != nil {
		t.Fatal("macro migration")
	}
	if got := globalReferencesPanel.Locations(); got[0].File != na || got[1].File != untouched {
		t.Fatal("references", got)
	}
	if got := globalProblems.Problems(); len(got) != 2 || got[0].File != na || got[1].File != untouched {
		t.Fatal("problems", got)
	}
	if !strings.HasPrefix(w.tabs.TabTitle(editorTabIndex(w.tabs, ed)), "main.go") {
		t.Fatal("tab title")
	}
	if saveEditorToDisk(ed, a) {
		t.Fatal("stale save target accepted")
	}
	if !saveEditorToDisk(ed, na) {
		t.Fatal("save renamed buffer")
	}
	if !strings.Contains(diskText(t, na), "Unsaved = 1") {
		t.Fatal("new path not saved")
	}
	if _, err := os.Stat(a); !os.IsNotExist(err) {
		t.Fatal("old path recreated", err)
	}
	ed.Undo()
	if ed.Text() != "package p\nvar Unsaved = 1\n" { // gofmt adds one blank line, undo formatting first
		t.Fatal("undo history lost", ed.Text())
	}
	ed.Undo()
	if ed.Text() != "package p\n" {
		t.Fatal("pre-rename edit no longer undoable", ed.Text())
	}
	macroEd.Undo()
	if strings.Contains(macroEd.Text(), "typed while moving") {
		t.Fatal("inflight edit undo lost")
	}
}
func TestEditorRenameFailureRestoresProvidersAndKeepsBuffers(t *testing.T) {
	w := renameWorkbench(t)
	root := t.TempDir()
	a := writeFile(t, root, "a.silkgo", "package p\n")
	b := writeFile(t, root, "b.silkgo", "do not replace")
	if !openFileInEditor(w.tabs, a) {
		t.Fatal("open")
	}
	ed := openEditors[a]
	ed.ReplaceAllText("package p\n// edits\n")
	before := ed.Text()
	rev := ed.TextRevision()
	plan, err := prepareEditorRename(w.tabs, w.canvas, a, b)
	if err != nil {
		t.Fatal(err)
	}
	awaitEditorRenamePlan(t, plan)
	cause := fileops.RenameNoReplace(a, b)
	if !os.IsExist(cause) {
		t.Fatal("expected filesystem conflict", cause)
	}
	plan.Complete(cause)
	if activeFileRename != nil || openEditors[a] != ed || openEditors[b] != nil || ed.FilePath() != a || ed.Text() != before || ed.TextRevision() != rev || macroEditors[a] == nil {
		t.Fatal("failure damaged session")
	}
	if diskText(t, b) != "do not replace" || diskText(t, a) != "package p\n" {
		t.Fatal("disk overwritten")
	}
}
func TestEditorRenameRejectsOpenDestinationAndDesignBeforeMutation(t *testing.T) {
	w := renameWorkbench(t)
	root := t.TempDir()
	a := writeFile(t, root, "a.go", "package p\n")
	b := writeFile(t, root, "b.go", "package other\n")
	for _, p := range []string{a, b} {
		if !openFileInEditor(w.tabs, p) {
			t.Fatal("open")
		}
	}
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	} // buffer still owns destination
	if _, err := prepareEditorRename(w.tabs, w.canvas, a, b); diagnostic.CodeOf(err) != diagnostic.FileConflict {
		t.Fatal("open destination", err)
	}
	design := w.canvas.GedScene().Filename()
	if _, err := prepareEditorRename(w.tabs, w.canvas, filepath.Dir(design), filepath.Dir(design)+"-new"); diagnostic.CodeOf(err) != diagnostic.FileOperationBusy {
		t.Fatal("active design not protected", err)
	}
	if activeFileRename != nil || !reflect.DeepEqual(orderedEditorPaths(w.tabs), []string{a, b}) {
		t.Fatal("rejected operation changed state")
	}
}

func TestEditorRenamePersistsNewSessionAndRecentPaths(t *testing.T) {
	w := renameWorkbench(t)
	config := t.TempDir()
	t.Setenv("HOME", config)
	t.Setenv("APPDATA", config)
	t.Setenv("XDG_CONFIG_HOME", config)
	globalPrefs = testPreferences(t)
	root := t.TempDir()
	old := writeFile(t, root, "before.go", "package p\n")
	next := filepath.Join(root, "after.go")
	if !openFileInEditor(w.tabs, old) {
		t.Fatal("open")
	}
	ed := openEditors[old]
	ed.ReplaceAllText("package p\nvar Unsaved int\n")
	ed.ScrollToLineCol(1, 5)
	globalPrefs.AddRecentFile(old)
	globalPrefs.SetWorkspaceSession(captureWorkspaceSession(w.tabs, w.canvas))
	plan, err := prepareEditorRename(w.tabs, w.canvas, old, next)
	if err != nil {
		t.Fatal(err)
	}
	awaitEditorRenamePlan(t, plan)
	if err := fileops.RenameNoReplace(old, next); err != nil {
		t.Fatal(err)
	}
	plan.Complete(nil)
	flushTestPreferences(t, globalPrefs)
	saved := testPreferences(t)
	s, ok := saved.WorkspaceSession()
	if !ok || s.ActiveFile != next || len(s.Editors) != 1 || s.Editors[0].Path != next || s.Editors[0].Line != 1 || s.Editors[0].Column != 5 {
		t.Fatal("session readback", s)
	}
	if got := saved.RecentFiles(); len(got) != 1 || got[0] != next {
		t.Fatal("recent readback", got)
	}
	if !ed.Modified() || diskText(t, next) != "package p\n" {
		t.Fatal("rename saved or discarded dirty buffer")
	}
}

func TestEditorRenameRealGoplsKeepsUnsavedDefinition(t *testing.T) {
	if testing.Short() {
		t.Skip("real gopls integration")
	}
	bin, err := exec.LookPath("gopls")
	if err != nil {
		t.Skip("gopls not installed")
	}
	w := renameWorkbench(t)
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module renameqa\n\ngo 1.26\n")
	old := writeFile(t, root, "before 空间.go", "package demo\nfunc baseline() {}\n")
	next := filepath.Join(root, "after 空间.go")
	client, err := core.LaunchLSPClient(bin)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for range client.Notifications() {
		}
	}()
	if _, err := client.Initialize(core.LSPInitializeParams{ProcessID: os.Getpid(), RootURI: fileURIOf(root), Capabilities: ideLSPCapabilities()}); err != nil {
		client.Close()
		t.Fatal(err)
	}
	oldDocs, oldVersions := lspDocuments, lspVersions
	lspDocuments = map[string]*lspDocument{}
	lspVersions = map[string]int{}
	globalLSP = client
	defer func() {
		for _, d := range lspDocuments {
			d.close()
			waitDocument(t, d)
		}
		client.Close()
		lspDocuments, lspVersions = oldDocs, oldVersions
	}()
	if !openFileInEditor(w.tabs, old) {
		t.Fatal("open")
	}
	ed := openEditors[old]
	ed.ReplaceAllText("package demo\nvar UnsavedRenamed = 7\nfunc Use(){ _ = UnsavedRenamed }\n")
	previous := lspDocuments[old]
	if _, err := previous.sync(); err != nil {
		t.Fatal(err)
	}
	plan, err := prepareEditorRename(w.tabs, w.canvas, old, next)
	if err != nil {
		t.Fatal(err)
	}
	if lspDocuments[old] != nil {
		t.Fatal("old callback guard still live")
	}
	ed.ReplaceAllText(ed.Text() + "// typed during rename\n")
	awaitEditorRenamePlan(t, plan)
	if err := fileops.RenameNoReplace(old, next); err != nil {
		t.Fatal(err)
	}
	plan.Complete(nil)
	doc := lspDocuments[next]
	if doc == nil {
		t.Fatal("new document missing")
	}
	snap, err := doc.sync()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-previous.done:
	default:
		t.Fatal("new URI opened before old close")
	}
	if snap.text != ed.Text() {
		t.Fatal("LSP reopened with stale or disk text")
	}
	col := strings.Index(strings.Split(ed.Text(), "\n")[2], "UnsavedRenamed") + 2
	locs, err := client.Definition(fileURIOf(next), 2, col)
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) == 0 || uriToPath(locs[0].URI) != next || locs[0].Range.Start.Line != 1 {
		t.Fatalf("unsaved definition after rename: %+v", locs)
	}
	if !ed.Modified() || strings.Contains(diskText(t, next), "UnsavedRenamed") {
		t.Fatal("rename unexpectedly saved buffer")
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old file recreated", err)
	}
	t.Logf("real gopls definition resolved renamed unsaved URI %s at %d:%d", locs[0].URI, locs[0].Range.Start.Line, locs[0].Range.Start.Character)
}

// The product explorer awaits this phase on its filesystem worker. Model tests
// have no event loop, but must exercise the same barrier before the same OS call.
func awaitEditorRenamePlan(t *testing.T, plan ged.RenamePreparation) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if plan.Wait != nil {
		if err := plan.Wait(ctx); err != nil {
			plan.Complete(err)
			t.Fatal(err)
		}
	}
}
