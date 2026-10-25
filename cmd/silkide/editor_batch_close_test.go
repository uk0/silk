package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/internal/filetxn"
)

type closeQueue struct{ workers, posts []func() }

func batchQueue(t *testing.T) *closeQueue {
	t.Helper()
	oldPost, oldLaunch, oldAsk, oldRead, oldCommit, oldJob := editorClosePost, editorCloseLaunch, askCloseEditors, editorCloseRead, editorCloseCommit, activeEditorClose
	q := &closeQueue{}
	activeEditorClose = nil
	editorClosePost = func(f func()) { q.posts = append(q.posts, f) }
	editorCloseLaunch = func(f func()) { q.workers = append(q.workers, f) }
	askCloseEditors = func(*gui.TabWidget, []string) gui.DialogResult { return gui.DialogCancel }
	t.Cleanup(func() {
		if activeEditorClose != nil {
			activeEditorClose.cancel()
			activeEditorClose = nil
		}
		editorClosePost, editorCloseLaunch, askCloseEditors, editorCloseRead, editorCloseCommit, activeEditorClose = oldPost, oldLaunch, oldAsk, oldRead, oldCommit, oldJob
	})
	return q
}
func (q *closeQueue) worker(t *testing.T) {
	t.Helper()
	if len(q.workers) != 1 {
		t.Fatalf("workers=%d", len(q.workers))
	}
	f := q.workers[0]
	q.workers = q.workers[1:]
	f()
}
func (q *closeQueue) post(t *testing.T) {
	t.Helper()
	if len(q.posts) == 0 {
		t.Fatal("missing UI delivery")
	}
	f := q.posts[0]
	q.posts = q.posts[1:]
	f()
}
func (q *closeQueue) drain(t *testing.T) {
	t.Helper()
	for n := 0; len(q.posts) > 0 || len(q.workers) > 0; n++ {
		if n > 100 {
			t.Fatal("unbounded close loop")
		}
		if len(q.posts) > 0 {
			q.post(t)
		} else {
			q.worker(t)
		}
	}
}
func batchFiles(t *testing.T, w *workbench) ([]string, []*gui.CodeEditor) {
	t.Helper()
	root := t.TempDir()
	var paths []string
	var editors []*gui.CodeEditor
	for _, name := range []string{"A.txt", "B.txt", "C.txt"} {
		p := writeFile(t, root, name, name)
		openFileInEditor(w.tabs, p)
		paths = append(paths, p)
		editors = append(editors, activeEditor(w.tabs))
	}
	return paths, editors
}
func TestEditorBatchCloseMenuAvailable(t *testing.T) {
	w := testWorkbench(t)
	batchQueue(t)
	p := writeFile(t, t.TempDir(), "a.txt", "alpha")
	openFileInEditor(w.tabs, p)
	m := tabContext(t, w.tabs, 0)
	for _, id := range []string{"tab.closeOthers", "tab.closeRight", "tab.closeAll"} {
		b := menuCommandButton(m, id)
		if b == nil || b.Icon() == nil {
			t.Fatal("missing batch-close command", id)
		}
		if b.IsEnabled() != (id == "tab.closeAll") {
			t.Fatal("wrong availability", id)
		}
	}
	if !menuCommandButton(buildFileMenu(w.tabs, w.canvas), "file.closeAll").IsEnabled() {
		t.Fatal("File entry disabled")
	}
}
func TestEditorBatchCloseScopesAndNewPages(t *testing.T) {
	for _, mode := range []editorCloseMode{closeOtherEditors, closeRightEditors, closeAllEditors} {
		t.Run(string(rune('A'+mode)), func(t *testing.T) {
			w := testWorkbench(t)
			q := batchQueue(t)
			paths, eds := batchFiles(t, w)
			if !startEditorBatchClose(w.tabs, eds[1], mode) || w.tabs.Count() != 3 || len(q.posts) != 0 {
				t.Fatal("not asynchronous")
			}
			newPath := writeFile(t, t.TempDir(), "new.txt", "new")
			openFileInEditor(w.tabs, newPath)
			q.drain(t)
			want := []bool{mode == closeRightEditors, mode != closeAllEditors, false}
			for i, p := range paths {
				if (openEditors[p] != nil) != want[i] {
					t.Fatal("wrong target", mode, p)
				}
				if diskText(t, p) != filepath.Base(p) {
					t.Fatal("clean file rewritten")
				}
			}
			if openEditors[newPath] == nil || activeEditorClose != nil {
				t.Fatal("new page closed or job retained")
			}
		})
	}
}
func TestEditorBatchCloseCancelBeforeAnyRemoval(t *testing.T) {
	w := testWorkbench(t)
	q := batchQueue(t)
	paths, eds := batchFiles(t, w)
	eds[2].ReplaceAllText("dirty")
	asked := false
	askCloseEditors = func(_ *gui.TabWidget, got []string) gui.DialogResult {
		asked = true
		if len(got) != 1 || got[0] != paths[2] {
			t.Fatal("wrong dirty list")
		}
		return gui.DialogCancel
	}
	startEditorBatchClose(w.tabs, nil, closeAllEditors)
	q.drain(t)
	if !asked || w.tabs.Count() != 3 || eds[2].Text() != "dirty" || diskText(t, paths[2]) != "C.txt" {
		t.Fatal("cancel lost work")
	}
}
func TestEditorBatchCloseSaveDiscardAndExactlyOneRemovalPerTurn(t *testing.T) {
	for _, choice := range []gui.DialogResult{gui.DialogOK, gui.DialogNo} {
		t.Run(string(rune('A'+choice)), func(t *testing.T) {
			w := testWorkbench(t)
			q := batchQueue(t)
			paths, eds := batchFiles(t, w)
			eds[0].ReplaceAllText("saved A")
			eds[2].ReplaceAllText("saved C")
			askCloseEditors = func(*gui.TabWidget, []string) gui.DialogResult { return choice }
			startEditorBatchClose(w.tabs, nil, closeAllEditors)
			q.worker(t)
			q.post(t)
			if choice == gui.DialogOK {
				q.worker(t)
				q.post(t)
				for i := 0; i < 4; i++ {
					q.post(t)
				}
			}
			for n := 3; n > 0; n-- {
				if w.tabs.Count() != n {
					t.Fatal("closed more than one per turn")
				}
				q.post(t)
			}
			q.post(t)
			if activeEditorClose != nil || len(w.closedEditors.items) != 3 {
				t.Fatal("cleanup/history incomplete")
			}
			wantA, wantC := "A.txt", "C.txt"
			if choice == gui.DialogOK {
				wantA, wantC = "saved A", "saved C"
			}
			if diskText(t, paths[0]) != wantA || diskText(t, paths[2]) != wantC {
				t.Fatal("wrong save decision")
			}
			if !reopenClosedEditor(w.tabs) || activeEditor(w.tabs).Text() != wantC {
				t.Fatal("reopen lost saved or retained discarded edits")
			}
		})
	}
}
func TestEditorBatchCloseDiskConflictAndSaveFailureKeepAll(t *testing.T) {
	for _, kind := range []string{"conflict", "save-error", "read-error"} {
		t.Run(kind, func(t *testing.T) {
			w := testWorkbench(t)
			q := batchQueue(t)
			paths, eds := batchFiles(t, w)
			eds[0].ReplaceAllText("unsaved A")
			eds[1].ReplaceAllText("unsaved B")
			askCloseEditors = func(*gui.TabWidget, []string) gui.DialogResult { return gui.DialogOK }
			if kind == "save-error" {
				editorCloseCommit = func(...filetxn.Change) error { return errors.New("disk full") }
			}
			if kind == "read-error" {
				editorCloseRead = func(string, int64) (filetxn.Snapshot, error) { return filetxn.Snapshot{}, errors.New("unreadable") }
			}
			startEditorBatchClose(w.tabs, nil, closeAllEditors)
			q.worker(t)
			q.post(t)
			if kind == "conflict" {
				if err := os.WriteFile(paths[1], []byte("external"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			q.drain(t)
			if w.tabs.Count() != 3 || eds[0].Text() != "unsaved A" || !eds[0].Modified() || diskText(t, paths[0]) != "A.txt" {
				t.Fatal("failed batch lost/overwrote work")
			}
			if kind == "conflict" && diskText(t, paths[1]) != "external" {
				t.Fatal("external edit overwritten")
			}
		})
	}
}
func TestEditorBatchCloseStaleBufferAndCancellation(t *testing.T) {
	for _, stage := range []string{"scan-edit", "scan-cancel", "save-edit", "save-cancel", "close-cancel"} {
		t.Run(stage, func(t *testing.T) {
			w := testWorkbench(t)
			q := batchQueue(t)
			paths, eds := batchFiles(t, w)
			eds[0].ReplaceAllText("captured")
			askCloseEditors = func(*gui.TabWidget, []string) gui.DialogResult { return gui.DialogOK }
			startEditorBatchClose(w.tabs, nil, closeAllEditors)
			if strings.HasPrefix(stage, "scan-") {
				if stage == "scan-edit" {
					eds[0].ReplaceAllText("newer")
				} else {
					cancelEditorBatchClose()
				}
				if startEditorBatchClose(w.tabs, nil, closeAllEditors) {
					t.Fatal("concurrent job accepted")
				}
				q.drain(t)
				if w.tabs.Count() != 3 || diskText(t, paths[0]) != "A.txt" {
					t.Fatal("scan changed files")
				}
				return
			}
			q.worker(t)
			q.post(t)
			q.worker(t)
			if stage == "save-edit" {
				eds[0].ReplaceAllText("newer")
			} else if stage == "save-cancel" {
				cancelEditorBatchClose()
			}
			q.post(t)
			if stage == "close-cancel" {
				for i := 0; i < 4; i++ {
					q.post(t)
				}
				q.post(t)
				cancelEditorBatchClose()
			}
			q.drain(t)
			want := 3
			if stage == "close-cancel" {
				want = 2
			}
			if w.tabs.Count() != want || diskText(t, paths[0]) != "captured" || activeEditorClose != nil {
				t.Fatal("late cancellation state")
			}
			if stage == "save-edit" && (eds[0].Text() != "newer" || !eds[0].Modified()) {
				t.Fatal("newer buffer marked saved")
			}
		})
	}
}
func TestEditorBatchCloseReservationAndStaleMenu(t *testing.T) {
	w := testWorkbench(t)
	q := batchQueue(t)
	paths, eds := batchFiles(t, w)
	menu := tabContext(t, w.tabs, 1)
	startEditorBatchClose(w.tabs, eds[1], closeOtherEditors)
	if saveEditorToDisk(eds[0], paths[0]) || closeEditorTab(w.tabs, 0) || canQuit(w.canvas) {
		t.Fatal("reserved file changed")
	}
	if _, err := prepareWorkspaceReplacement(w.tabs, nil, []ged.FileReplacement{{Path: paths[0], Before: "A.txt", After: "replacement"}}); err == nil {
		t.Fatal("replacement overlapped")
	}
	if _, err := prepareEditorRename(w.tabs, nil, paths[0], paths[0]+".renamed"); err == nil {
		t.Fatal("rename overlapped")
	}
	cancelEditorBatchClose()
	q.drain(t)
	if !closeEditorTab(w.tabs, 1) {
		t.Fatal("close anchor")
	}
	openFileInEditor(w.tabs, paths[1])
	menuCommandButton(menu, "tab.closeAll").Action().Trigger(nil)
	if activeEditorClose != nil || w.tabs.Count() != 3 {
		t.Fatal("stale menu targeted replacement")
	}
}
func TestEditorBatchCloseRealBlockedWorkerLeavesUIFree(t *testing.T) {
	w := testWorkbench(t)
	q := batchQueue(t)
	_, eds := batchFiles(t, w)
	entered, release, posted := make(chan struct{}), make(chan struct{}), make(chan func(), 1)
	editorCloseLaunch = func(f func()) { go f() }
	editorClosePost = func(f func()) { posted <- f }
	editorCloseRead = func(path string, limit int64) (filetxn.Snapshot, error) {
		close(entered)
		<-release
		return filetxn.ReadLimit(path, limit)
	}
	startEditorBatchClose(w.tabs, nil, closeAllEditors)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker not entered")
	}
	eds[0].ReplaceAllText("typed while disk blocked")
	cancelEditorBatchClose()
	if startEditorBatchClose(w.tabs, nil, closeAllEditors) {
		t.Fatal("second blocked worker")
	}
	close(release)
	select {
	case f := <-posted:
		f()
	case <-time.After(5 * time.Second):
		t.Fatal("worker not retired")
	}
	if w.tabs.Count() != 3 || eds[0].Text() != "typed while disk blocked" || activeEditorClose != nil || len(q.workers) != 0 {
		t.Fatal("UI lost input")
	}
}
func TestEditorBatchCloseSkipsUntrackedAndBoundsPaths(t *testing.T) {
	w := testWorkbench(t)
	q := batchQueue(t)
	_, eds := batchFiles(t, w)
	unknown := gui.NewCodeEditor()
	unknown.SetText("only copy")
	w.tabs.AddTab(unknown, "Transient", nil)
	startEditorBatchClose(w.tabs, eds[1], closeAllEditors)
	q.drain(t)
	if w.tabs.Count() != 1 || w.tabs.Stack().Page(0) != unknown || unknown.Text() != "only copy" {
		t.Fatal("untracked work lost")
	}
	text := strings.Repeat("空间🙂/", 100) + "file.go"
	short := closeEditorPathLabel(text)
	if !utf8.ValidString(short) || len([]rune(short)) != 72 || !strings.HasSuffix(short, "file.go") {
		t.Fatal("unbounded/invalid path label")
	}
}
func TestIndividualCloseRevalidatesPageAfterModalMutation(t *testing.T) {
	w := testWorkbench(t)
	batchQueue(t)
	paths, eds := batchFiles(t, w)
	eds[1].ReplaceAllText("dirty B")
	old := askCloseDirtyEditor
	defer func() { askCloseDirtyEditor = old }()
	askCloseDirtyEditor = func(*gui.CodeEditor, string) gui.DialogResult {
		if !closeEditorTab(w.tabs, 0) {
			t.Fatal("nested earlier close")
		}
		return gui.DialogNo
	}
	if !closeEditorTab(w.tabs, 1) || w.tabs.Count() != 1 || activeEditor(w.tabs) != eds[2] || openEditors[paths[1]] != nil {
		t.Fatal("stale numeric index closed wrong page")
	}
}

func TestEditorBatchCloseFormatsGoAndPreservesNewerText(t *testing.T) {
	w := testWorkbench(t)
	q := batchQueue(t)
	path := writeFile(t, t.TempDir(), "main.go", "package main\n")
	openFileInEditor(w.tabs, path)
	ed := activeEditor(w.tabs)
	ed.ReplaceAllText("package main\nfunc f( ){ }\n")
	expected, err := gofmtSource(ed.Text())
	if err != nil {
		t.Skip("gofmt unavailable: ", err)
	}
	askCloseEditors = func(*gui.TabWidget, []string) gui.DialogResult { return gui.DialogOK }
	startEditorBatchClose(w.tabs, nil, closeAllEditors)
	q.drain(t)
	if diskText(t, path) != expected || w.tabs.Count() != 0 {
		t.Fatal("Go save was not formatted")
	}
	if !reopenClosedEditor(w.tabs) || activeEditor(w.tabs).Text() != expected {
		t.Fatal("formatted state not reopened")
	}
}
func TestEditorBatchCloseRejectsMutationInsidePromptAndOwnerReplacement(t *testing.T) {
	for _, mutate := range []string{"prompt-edit", "owner"} {
		t.Run(mutate, func(t *testing.T) {
			w := testWorkbench(t)
			q := batchQueue(t)
			paths, eds := batchFiles(t, w)
			eds[0].ReplaceAllText("before")
			askCloseEditors = func(*gui.TabWidget, []string) gui.DialogResult {
				if mutate == "prompt-edit" {
					eds[0].ReplaceAllText("during prompt")
				}
				return gui.DialogOK
			}
			startEditorBatchClose(w.tabs, nil, closeAllEditors)
			q.worker(t)
			q.post(t)
			if mutate == "owner" {
				q.worker(t)
				globalWorkbench = &workbench{}
				defer func() { globalWorkbench = w }()
			}
			q.drain(t)
			if w.tabs.Count() != 3 || activeEditorClose != nil {
				t.Fatal("obsolete batch closed pages")
			}
			if mutate == "prompt-edit" && diskText(t, paths[0]) != "A.txt" {
				t.Fatal("changed prompt state saved")
			}
			if mutate == "owner" && !eds[0].Modified() {
				t.Fatal("old owner mutated by completion")
			}
		})
	}
}
func TestEditorBatchCloseFileAndPaletteRoutes(t *testing.T) {
	for _, entry := range []string{"file", "palette"} {
		t.Run(entry, func(t *testing.T) {
			w := testWorkbench(t)
			q := batchQueue(t)
			batchFiles(t, w)
			if entry == "file" {
				menuCommandButton(buildFileMenu(w.tabs, w.canvas), "file.closeAll").Action().Trigger(nil)
			} else {
				old := paletteCommands
				defer func() { paletteCommands = old }()
				registerPaletteCommands(w.tabs, w.canvas)
				for _, command := range paletteCommands {
					if command.Name == "Close All Editors" {
						command.Fn()
						break
					}
				}
			}
			if activeEditorClose == nil {
				t.Fatal("command did not start batch")
			}
			cancel := menuCommandButton(buildFileMenu(w.tabs, w.canvas), "file.cancelClose")
			if cancel == nil || !cancel.IsEnabled() {
				t.Fatal("cancel unavailable")
			}
			cancel.Action().Trigger(nil)
			q.drain(t)
			if w.tabs.Count() != 3 {
				t.Fatal("cancel command did not preserve pages")
			}
		})
	}
}

func TestIndividualCloseModalReplacementDoesNotSaveOldBuffer(t *testing.T) {
	w := testWorkbench(t)
	batchQueue(t)
	paths, eds := batchFiles(t, w)
	eds[1].ReplaceAllText("old unsaved")
	old := askCloseDirtyEditor
	defer func() { askCloseDirtyEditor = old }()
	var replacement *gui.CodeEditor
	askCloseDirtyEditor = func(*gui.CodeEditor, string) gui.DialogResult {
		w.tabs.RemoveTab(1)
		delete(openEditors, paths[1])
		openFileInEditor(w.tabs, paths[1])
		replacement = activeEditor(w.tabs)
		replacement.ReplaceAllText("replacement unsaved")
		return gui.DialogOK
	}
	if closeEditorTab(w.tabs, 1) || diskText(t, paths[1]) != "B.txt" || replacement.Text() != "replacement unsaved" {
		t.Fatal("obsolete close wrote over replacement")
	}
}

func BenchmarkEditorBatchCloseIdentityCheck(b *testing.B) {
	old := openEditors
	defer func() { openEditors = old }()
	openEditors = map[string]*gui.CodeEditor{}
	tabs := gui.NewTabWidget()
	job := &editorCloseJob{tabs: tabs, owner: globalWorkbench}
	oldJob := activeEditorClose
	activeEditorClose = job
	defer func() { activeEditorClose = oldJob }()
	for i := 0; i < 1000; i++ {
		path := filepath.Join("project", string(rune(0x4e00+i))+".txt")
		ed := gui.NewCodeEditor()
		ed.SetFilePath(path)
		tabs.AddTab(ed, path, nil)
		openEditors[path] = ed
		job.targets = append(job.targets, editorCloseTarget{editor: ed, path: path, revision: ed.TextRevision()})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !job.allCurrent() {
			b.Fatal("identity mismatch")
		}
	}
}
