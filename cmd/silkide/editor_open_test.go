package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func pumpManualOpenUntil(t *testing.T, w *workbench, done func() bool) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for !done() {
		select {
		case f := <-testEditorOpenPosts[w]:
			f()
		case <-timer.C:
			t.Fatal("manual-open UI delivery timeout")
		}
	}
}
func TestEditorOpenPublicFlowReturnsBeforePhysicalReader(t *testing.T) {
	w, prefs := recentOpenWorkbench(t)
	path := writeFile(t, t.TempDir(), "regular.txt", "source 中文 😀")
	entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	result := make(chan bool, 1)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	w.opens.settings.read = func(ctx context.Context, p string) ([]byte, error) {
		close(entered)
		<-release
		b, _, e := readRestoreSnapshot(ctx, p)
		return b, e
	}
	go func() {
		select {
		case <-entered:
			select {
			case <-returned:
				result <- true
			case <-time.After(time.Second):
				result <- false
			}
		case <-returned:
			result <- true
		}
		unblock()
	}()
	accepted := openFileInEditor(w.tabs, path)
	close(returned)
	if !accepted || !<-result {
		t.Fatal("UI open waited for physical reader")
	}
	if activeEditor(w.tabs) != nil || w.tabs.Count() != 1 || len(prefs.RecentFiles()) != 0 {
		t.Fatal("accepted request pretended source was ready")
	}
	if pending, ok := w.tabs.Stack().CurrentPage().(*restorePage); !ok || pending.saved.Path != path || !pending.HasFocus() {
		t.Fatal("no owned inert focusable loading page")
	}
	pumpTestEditorOpens(t, w.tabs)
	ed := activeEditor(w.tabs)
	if ed == nil || ed.Text() != "source 中文 😀" || ed.Modified() || !ed.HasFocus() || !reflect.DeepEqual(prefs.RecentFiles(), []string{path}) {
		t.Fatal("ready source/focus/MRU not committed")
	}
	t.Log("public_open_accepted=true return_before_reader_released=true; ready UI and MRU delivered later")
}

func TestEditorOpenDeduplicatesLatestLocationAndRetainsDirtyBuffer(t *testing.T) {
	w := testWorkbench(t)
	root := t.TempDir()
	origin := writeFile(t, root, "origin.txt", "origin\n")
	if !openTestEditorReady(t, w.tabs, origin) {
		t.Fatal("origin")
	}
	ed := activeEditor(w.tabs)
	ed.ReplaceAllText("dirty origin\n")
	ed.NavigateToLineCol(0, 3)
	target := writeFile(t, root, "target.txt", "one\ntwo\nthree\n")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var reads atomic.Int32
	w.opens.settings.read = func(ctx context.Context, p string) ([]byte, error) {
		reads.Add(1)
		close(entered)
		<-release
		b, _, e := readRestoreSnapshot(ctx, p)
		return b, e
	}
	calls := 0
	for line := 0; line < 50; line++ {
		n := line % 3
		if !navigateEditor(w.tabs, target, func(target *gui.CodeEditor) { calls++; target.NavigateToLineCol(n, 1) }) {
			t.Fatal("request")
		}
	}
	<-entered
	if w.tabs.Count() != 2 || len(w.opens.all) != 1 || reads.Load() != 1 || w.navigation.Len() != 0 {
		t.Fatal("duplicate read/page or premature history")
	}
	unblock()
	pumpTestEditorOpens(t, w.tabs)
	if calls != 1 || activeEditorPath(w.tabs) != target || activeEditor(w.tabs).CursorLine() != 1 || !w.navigation.CanGoBack() {
		t.Fatal("latest ready location not committed")
	}
	navigateEditorHistory(w.tabs, false)
	if activeEditor(w.tabs) != ed || ed.Text() != "dirty origin\n" || !ed.CanUndo() || ed.CursorCol() != 3 {
		t.Fatal("back overwrote dirty source or origin")
	}
}

func TestEditorOpenCompletionDoesNotStealAnotherFieldFocus(t *testing.T) {
	for _, duringReplace := range []bool{false, true} {
		t.Run(map[bool]string{false: "before", true: "during-replace"}[duringReplace], func(t *testing.T) {
			w := testWorkbench(t)
			path := writeFile(t, t.TempDir(), "new.txt", "new source")
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			w.opens.settings.read = func(ctx context.Context, p string) ([]byte, error) {
				close(entered)
				<-release
				b, _, e := readRestoreSnapshot(ctx, p)
				return b, e
			}
			ready := 0
			requestEditorOpen(w.tabs, path, editorOpenOptions{ready: func(*gui.CodeEditor) bool { ready++; return true }})
			<-entered
			field := gui.NewEdit()
			field.SetParent(w.frame)
			if duringReplace {
				w.tabs.SetCurrentChangedCallback(func(_ interface{}, _ int) {
					if activeEditor(w.tabs) != nil {
						field.SetFocus()
					}
				})
			} else {
				field.SetFocus()
			}
			unblock()
			pumpTestEditorOpens(t, w.tabs)
			if !field.HasFocus() || ready != 0 || openEditors[path] == nil {
				t.Fatalf("late source stole input focus or executed navigation: field=%v ready=%d loaded=%v focus=%T", field.HasFocus(), ready, openEditors[path] != nil, gui.CurrentFocusWidget())
			}
		})
	}
}

func TestEditorOpenCancelKeepsPhysicalSlotsAndBoundsAdmission(t *testing.T) {
	w := testWorkbench(t)
	w.opens.settings.limit = 3
	root := t.TempDir()
	var paths []string
	for _, name := range []string{"a.txt", "b.txt", "c.txt", "d.txt"} {
		paths = append(paths, writeFile(t, root, name, name))
	}
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	started := make(chan string, 4)
	var physical atomic.Int32
	w.opens.settings.read = func(_ context.Context, p string) ([]byte, error) {
		physical.Add(1)
		started <- p
		<-release
		return os.ReadFile(p)
	}
	for _, p := range paths[:3] {
		if !openFileInEditor(w.tabs, p) {
			t.Fatal("admission")
		}
	}
	<-started
	<-started
	if !closeEditorTab(w.tabs, 0) || w.opens.active != 2 || !w.opens.Busy() {
		t.Fatal("logical close freed physical slot")
	}
	if openFileInEditor(w.tabs, paths[3]) || physical.Load() != 2 {
		t.Fatal("admission grew replacement workers")
	}
	if !cancelEditorFileOpens(w.tabs) || editorFileOpensPending(w.tabs) || w.tabs.Count() != 0 || physical.Load() != 2 {
		t.Fatal("cancel changed buffers or started queued read")
	}
	unblock()
	pumpTestEditorOpens(t, w.tabs)
	if len(openEditors) != 0 || physical.Load() != 2 || w.opens.active != 0 {
		t.Fatal("late canceled result or unbounded read")
	}
}

func TestEditorOpenSoftTimeoutCannotSpawnReplacementReaders(t *testing.T) {
	w := testWorkbench(t)
	w.opens.settings.timeout = 40 * time.Millisecond
	root := t.TempDir()
	a := writeFile(t, root, "a.txt", "a")
	b := writeFile(t, root, "b.txt", "b")
	c := writeFile(t, root, "c.txt", "c")
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var physical atomic.Int32
	w.opens.settings.read = func(_ context.Context, p string) ([]byte, error) { physical.Add(1); <-release; return os.ReadFile(p) }
	openFileInEditor(w.tabs, a)
	openFileInEditor(w.tabs, b)
	pumpManualOpenUntil(t, w, func() bool { return !editorFileOpensPending(w.tabs) })
	if w.opens.active != 2 || physical.Load() != 2 || !w.opens.Busy() || len(openEditors) != 0 {
		t.Fatal("deadline joined or freed a physical call")
	}
	w.opens.settings.timeout = time.Second
	if !openFileInEditor(w.tabs, c) || physical.Load() != 2 {
		t.Fatal("blocked-slot retry started a replacement")
	}
	cancelEditorFileOpens(w.tabs)
	unblock()
	pumpTestEditorOpens(t, w.tabs)
	if openEditors[a] != nil || openEditors[b] != nil || openEditors[c] != nil {
		t.Fatal("late timeout data installed")
	}
}

func TestEditorOpenHistorySkipsMissingOnlyAfterActualFailure(t *testing.T) {
	w := testWorkbench(t)
	root := t.TempDir()
	a := writeFile(t, root, "a.txt", "a\n")
	b := writeFile(t, root, "b.txt", "b\n")
	missing := root + "/missing.txt"
	openTestEditorReady(t, w.tabs, a)
	origin := activeEditor(w.tabs)
	origin.NavigateToLineCol(0, 1)
	w.navigation.Push(gui.NavPosition{FilePath: a, Column: 1})
	w.navigation.Push(gui.NavPosition{FilePath: missing})
	w.navigation.Push(gui.NavPosition{FilePath: b})
	openTestEditorReady(t, w.tabs, b)
	live := activeEditor(w.tabs).NavigationPosition()
	navigateEditorHistory(w.tabs, false)
	if pos, _ := w.navigation.CurrentPosition(); pos.FilePath != b || activeEditor(w.tabs) != nil {
		t.Fatal("history committed before failed read")
	}
	pumpTestEditorOpens(t, w.tabs)
	if activeEditor(w.tabs) != origin || origin.CursorCol() != 1 || !w.navigation.CanGoForward() {
		t.Fatal("sequential history fallback failed")
	}
	next := w.navigation.PrepareNavigation(origin.NavigationPosition(), true)
	if len(next) != 2 || next[1].Destination() != live {
		t.Fatal("live return position lost")
	}
}

func TestEditorOpenCloseDetachesWorkerMailboxesAndHeldPage(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "a.txt", "source")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	w.opens.settings.read = func(_ context.Context, p string) ([]byte, error) { close(entered); <-release; return os.ReadFile(p) }
	openFileInEditor(w.tabs, path)
	<-entered
	var job *editorOpenJob
	for _, j := range w.opens.all {
		job = j
	}
	page := job.page
	box := job.box
	w.opens.Close()
	if box.owner.Load() != nil || page.cancel != nil || job.options.ready != nil || job.page != nil || w.opens.owner != nil || w.opens.tabs != nil {
		t.Fatal("retired worker retains UI owner")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if !errors.Is(w.opens.jobs.Wait(ctx), context.DeadlineExceeded) {
		t.Fatal("close pretended reader had ended")
	}
	unblock()
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := w.opens.jobs.Wait(ctx2); err != nil {
		t.Fatal(err)
	}
	select {
	case f := <-testEditorOpenPosts[w]:
		f()
	default:
	}
	if len(openEditors) != 0 {
		t.Fatal("closed owner accepted late source")
	}
}

func TestEditorOpenRepeatedQueuedCancellationDoesNotRetainJobs(t *testing.T) {
	w := testWorkbench(t)
	w.opens.settings.limit = 4
	root := t.TempDir()
	a := writeFile(t, root, "a.txt", "a")
	b := writeFile(t, root, "b.txt", "b")
	queued := writeFile(t, root, "queued.txt", "q")
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	entered := make(chan struct{}, 2)
	var physical atomic.Int32
	w.opens.settings.read = func(_ context.Context, p string) ([]byte, error) {
		physical.Add(1)
		entered <- struct{}{}
		<-release
		return os.ReadFile(p)
	}
	openFileInEditor(w.tabs, a)
	openFileInEditor(w.tabs, b)
	<-entered
	<-entered
	for i := 0; i < 100; i++ {
		if !openFileInEditor(w.tabs, queued) {
			t.Fatal("queued admission")
		}
		idx := w.tabs.CurrentIndex()
		if !closeEditorTab(w.tabs, idx) {
			t.Fatal("close queued")
		}
		if len(w.opens.queue) != 0 || len(w.opens.all) != 2 || physical.Load() != 2 {
			t.Fatal("retained canceled queue or replacement thread", len(w.opens.queue), len(w.opens.all), physical.Load())
		}
	}
	cancelEditorFileOpens(w.tabs)
	unblock()
	pumpTestEditorOpens(t, w.tabs)
}

func TestEditorOpenOutOfOrderCompletionKeepsLatestIntent(t *testing.T) {
	w, prefs := recentOpenWorkbench(t)
	root := t.TempDir()
	origin := writeFile(t, root, "origin.txt", "origin")
	openTestEditorReady(t, w.tabs, origin)
	a := writeFile(t, root, "a.txt", "a\nsecond")
	b := writeFile(t, root, "b.txt", "b\nsecond")
	releases := map[string]chan struct{}{a: make(chan struct{}), b: make(chan struct{})}
	var onceA, onceB sync.Once
	unblockA := func() { onceA.Do(func() { close(releases[a]) }) }
	unblockB := func() { onceB.Do(func() { close(releases[b]) }) }
	t.Cleanup(unblockA)
	t.Cleanup(unblockB)
	entered := make(chan string, 2)
	w.opens.settings.read = func(_ context.Context, p string) ([]byte, error) { entered <- p; <-releases[p]; return os.ReadFile(p) }
	calledA, calledB := 0, 0
	navigateEditor(w.tabs, a, func(ed *gui.CodeEditor) { calledA++; ed.NavigateToLineCol(1, 1) })
	navigateEditor(w.tabs, b, func(ed *gui.CodeEditor) { calledB++; ed.NavigateToLineCol(1, 2) })
	<-entered
	<-entered
	unblockB()
	pumpManualOpenUntil(t, w, func() bool { return openEditors[b] != nil })
	unblockA()
	pumpTestEditorOpens(t, w.tabs)
	if calledA != 0 || calledB != 1 || activeEditorPath(w.tabs) != b || !openEditors[b].HasFocus() || openEditors[b].CursorCol() != 2 || openEditors[a] == nil || w.navigation.Len() != 2 {
		t.Fatal("obsolete completion changed current location/history/focus")
	}
	if !reflect.DeepEqual(prefs.RecentFiles(), []string{b, origin}) {
		t.Fatal("premature MRU", prefs.RecentFiles())
	}
}

func TestEditorOpenHistoryMutationRevokesPendingLocation(t *testing.T) {
	w := testWorkbench(t)
	root := t.TempDir()
	origin := writeFile(t, root, "origin.txt", "origin")
	openTestEditorReady(t, w.tabs, origin)
	path := writeFile(t, root, "target.txt", "one\ntwo")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	w.opens.settings.read = func(_ context.Context, p string) ([]byte, error) { close(entered); <-release; return os.ReadFile(p) }
	called := 0
	navigateEditor(w.tabs, path, func(ed *gui.CodeEditor) { called++; ed.NavigateToLineCol(1, 1) })
	<-entered
	w.navigation.Push(gui.NavPosition{FilePath: "newer-history"})
	w.navigation.Clear()
	unblock()
	pumpTestEditorOpens(t, w.tabs)
	if called != 0 || w.navigation.Len() != 0 || openEditors[path] == nil || openEditors[path].CursorLine() != 0 {
		t.Fatal("pending location resurrected cleared history")
	}
}

func TestEditorOpenFailureNeverCreatesEditableEmptyFile(t *testing.T) {
	for _, kind := range []string{"missing", "directory", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			w, prefs := recentOpenWorkbench(t)
			path := filepath.Join(t.TempDir(), kind+".txt")
			switch kind {
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				err = f.Truncate(33 << 20)
				_ = f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			if !openFileInEditor(w.tabs, path) {
				t.Fatal("request not admitted")
			}
			pumpTestEditorOpens(t, w.tabs)
			if activeEditor(w.tabs) != nil || len(openEditors) != 0 || len(prefs.RecentFiles()) != 0 {
				t.Fatal("failed read fabricated a ready editor")
			}
			if kind == "oversized" {
				p, ok := w.tabs.Stack().CurrentPage().(*restorePage)
				if !ok || p.label.Text() == "" || !closeEditorTab(w.tabs, w.tabs.CurrentIndex()) {
					t.Fatal("error page not closeable")
				}
			} else if w.tabs.Count() != 0 {
				t.Fatal("unavailable path retained a tab")
			}
		})
	}
}

func TestEditorOpenPendingSessionCapturesRequestedPosition(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "pending.txt", "one\ntwo")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	w.opens.settings.read = func(_ context.Context, p string) ([]byte, error) { close(entered); <-release; return os.ReadFile(p) }
	saved := editorSession{Path: path, Line: 1, Column: 2, ScrollY: 0}
	if !requestEditorOpen(w.tabs, path, editorOpenOptions{saved: &saved}) {
		t.Fatal("request")
	}
	<-entered
	state := captureWorkspaceSession(w.tabs, w.canvas)
	if !reflect.DeepEqual(state.Editors, []editorSession{saved}) || state.ActiveFile != path {
		t.Fatal("pending session identity lost", state)
	}
	unblock()
	pumpTestEditorOpens(t, w.tabs)
	if ed := activeEditor(w.tabs); ed == nil || ed.CursorLine() != 1 || ed.CursorCol() != 2 {
		t.Fatal("requested position lost")
	}
}

func TestEditorOpenCancelActionsCannotTargetReplacementController(t *testing.T) {
	w := testWorkbench(t)
	oldPalette := paletteCommands
	t.Cleanup(func() { paletteCommands = oldPalette })
	root := t.TempDir()
	a := writeFile(t, root, "a.txt", "a")
	b := writeFile(t, root, "b.txt", "b")
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	read := func(_ context.Context, p string) ([]byte, error) { <-release; return os.ReadFile(p) }
	w.opens.settings.read = read
	openFileInEditor(w.tabs, a)
	menu := buildFileMenu(w.tabs, w.canvas)
	defer releaseWorkbenchMenu(menu)
	button := menuCommandButton(menu, "file.cancelOpens")
	if button == nil || !button.IsEnabled() {
		t.Fatal("cancel action unavailable")
	}
	registerPaletteCommands(w.tabs, w.canvas)
	var bound paletteCommand
	for _, command := range paletteCommands {
		if command.Name == i18n.T("Cancel File Opens") {
			bound = command.Bind()
		}
	}
	if bound.Available == nil || !bound.Available() {
		t.Fatal("bound palette unavailable")
	}
	old := w.opens
	old.Close()
	installTestEditorOpener(t, w)
	w.opens.settings.read = read
	openFileInEditor(w.tabs, b)
	button.Action().Trigger(nil)
	bound.Fn()
	if bound.Available() || len(w.opens.pending) != 1 || !editorFileOpensPending(w.tabs) {
		t.Fatal("stale cancel targeted replacement owner")
	}
	if !cancelEditorFileOpens(w.tabs) {
		t.Fatal("current cancel failed")
	}
	unblock()
	pumpTestEditorOpens(t, w.tabs)
}
