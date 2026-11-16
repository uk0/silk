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

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func blockDesignReader(t *testing.T, w *workbench) (<-chan string, func()) {
	t.Helper()
	entered, release := make(chan string, 8), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	w.designOpen.settings.read = func(ctx context.Context, p string) (designOpenSnapshot, error) {
		entered <- p
		<-release
		return readManualDesign(ctx, p)
	}
	return entered, unblock
}
func designOpenPath(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name+".silkui")
	writeRecoveryDesign(t, p, name, "button "+name)
	return p
}
func TestManualDesignOpenPublicFlowReturnsBeforePhysicalReader(t *testing.T) {
	w, prefs := recentOpenWorkbench(t)
	path := designOpenPath(t, "目标 😀")
	scene := w.canvas.GedScene()
	before := scene.SaveDesign().String()
	oldPath := scene.Filename()
	entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	result := make(chan bool, 1)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	w.designOpen.settings.read = func(ctx context.Context, p string) (designOpenSnapshot, error) {
		close(entered)
		<-release
		return readManualDesign(ctx, p)
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
	openDocument(path, w.tabs, w.canvas, w.center, true)
	close(returned)
	if !<-result {
		t.Fatal("design open waited on the physical reader")
	}
	if scene.SaveDesign().String() != before || scene.Filename() != oldPath || len(prefs.RecentFiles()) != 0 || !designOpenPending(w.tabs) {
		t.Fatal("accepted read pretended to be an installed design")
	}
	if state := captureWorkspaceSession(w.tabs, w.canvas); state.Design != oldPath {
		t.Fatal("pending target replaced current session identity")
	}
	pumpTestDesignOpens(t, w)
	if w.canvas.GedScene() != scene || scene.Filename() != path || scene.FormTitle() != "目标 😀" || recoveryButtonText(t, scene) != "button 目标 😀" || !scene.IsClean() || !reflect.DeepEqual(prefs.RecentFiles(), []string{path}) {
		t.Fatal("prepared design was not installed exactly once")
	}
	t.Log("design_open_return_before_reader_released=true; original scene/MRU preserved until owned installation")
}
func TestManualDesignOpenDeduplicatesAndClearsOwnedStatus(t *testing.T) {
	w, prefs := recentOpenWorkbench(t)
	path := designOpenPath(t, "shared")
	bar := gui.NewStatusBar()
	w.frame.SetStatusBar(bar)
	entered, release := blockDesignReader(t, w)
	for i := 0; i < 50; i++ {
		if !requestDesignOpen(path, w.tabs, w.canvas, true) {
			t.Fatal("admission")
		}
	}
	<-entered
	if len(w.designOpen.all) != 1 || w.designOpen.active != 1 || w.designOpen.queued != nil || bar.Message() == "" {
		t.Fatal("same-path request created extra physical reads or lost status")
	}
	// Another producer owns this later status; completion must not clear it.
	bar.SetMessage("build owns this message")
	release()
	pumpTestDesignOpens(t, w)
	if bar.Message() != "build owns this message" || w.designOpen.status != "" || len(prefs.RecentFiles()) != 1 {
		t.Fatal("completion stole a newer status/MRU")
	}
}
func TestManualDesignOpenSupersedesBoundedQueuedWork(t *testing.T) {
	w := testWorkbench(t)
	a, b, c := designOpenPath(t, "a"), designOpenPath(t, "b"), designOpenPath(t, "c")
	entered := make(chan string, 4)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var physical atomic.Int32
	w.designOpen.settings.read = func(_ context.Context, p string) (designOpenSnapshot, error) {
		physical.Add(1)
		entered <- p
		<-release
		return readManualDesign(context.Background(), p)
	}
	requestDesignOpen(a, w.tabs, w.canvas, true)
	if <-entered != a {
		t.Fatal("first reader")
	}
	for i := 0; i < 100; i++ {
		path := b
		if i%2 == 1 {
			path = c
		}
		if !requestDesignOpen(path, w.tabs, w.canvas, true) {
			t.Fatal("queued admission")
		}
		if len(w.designOpen.all) != 2 || w.designOpen.active != 1 || physical.Load() != 1 {
			t.Fatal("logical supersession grew physical/metadata capacity")
		}
	}
	unblock()
	pumpTestDesignOpens(t, w)
	if physical.Load() != 2 || w.canvas.GedScene().Filename() != c || w.designOpen.active != 0 || len(w.designOpen.all) != 0 {
		t.Fatal("obsolete target or canceled queued read was installed")
	}
}
func TestManualDesignOpenTimeoutDoesNotFreePhysicalSlot(t *testing.T) {
	w := testWorkbench(t)
	w.designOpen.settings.timeout = 100 * time.Millisecond
	a, b := designOpenPath(t, "a"), designOpenPath(t, "b")
	before := w.canvas.GedScene().SaveDesign().String()
	entered, unblock := blockDesignReader(t, w)
	requestDesignOpen(a, w.tabs, w.canvas, true)
	<-entered
	pumpTestDesignOpenUntil(t, w, func() bool { return !designOpenPending(w.tabs) })
	if w.designOpen.active != 1 || len(w.designOpen.all) != 1 || w.canvas.GedScene().SaveDesign().String() != before {
		t.Fatal("timeout freed a physical reader or replaced the scene")
	}
	w.designOpen.settings.timeout = 5 * time.Second
	requestDesignOpen(b, w.tabs, w.canvas, true)
	if len(w.designOpen.all) != 2 || w.designOpen.active != 1 || w.designOpen.queued == nil {
		t.Fatal("new request bypassed blocked slot")
	}
	select {
	case <-entered:
		t.Fatal("replacement physical reader started")
	default:
	}
	cancelOwnedDesignOpen(w.designOpen)
	unblock()
	pumpTestDesignOpens(t, w)
	if w.canvas.GedScene().SaveDesign().String() != before {
		t.Fatal("late canceled result installed")
	}
}
func TestManualDesignOpenRejectsInterveningState(t *testing.T) {
	for _, action := range []string{"edit-undo", "draft", "filename", "scene", "focus", "capture", "mode-aba", "source-open"} {
		t.Run(action, func(t *testing.T) {
			w, prefs := recentOpenWorkbench(t)
			path := designOpenPath(t, "target")
			scene := w.canvas.GedScene()
			entered, unblock := blockDesignReader(t, w)
			requestDesignOpen(path, w.tabs, w.canvas, true)
			<-entered
			switch action {
			case "edit-undo":
				scene.PushCommand(recoveryTitleEdit{scene, scene.FormTitle(), "edited"})
				scene.UndoStack().Undo()
			case "draft":
				scene.AddPendingEditor(&restoreDraft{pending: true})
			case "filename":
				scene.SetFilename(filepath.Join(t.TempDir(), "new.silkui"))
			case "scene":
				w.canvas.SetScene(ged.NewGedScene())
			case "focus":
				f := gui.NewEdit()
				f.SetParent(w.frame)
				f.SetFocus()
			case "capture":
				w.canvas.PushCapture()
				defer w.canvas.PopCapture()
			case "mode-aba":
				w.activate(modeWelcome)
				w.activate(modeDesign)
			case "source-open":
				p := writeFile(t, t.TempDir(), "notes.txt", "notes")
				openFileInEditor(w.tabs, p)
				pumpTestEditorOpens(t, w.tabs)
			}
			current := w.canvas.GedScene()
			before := current.SaveDesign().String()
			name := current.Filename()
			count := current.UndoStack().Count()
			focus := gui.CurrentFocusWidget()
			unblock()
			pumpTestDesignOpens(t, w)
			if w.canvas.GedScene() != current || current.SaveDesign().String() != before || current.Filename() != name || current.UndoStack().Count() != count || gui.CurrentFocusWidget() != focus {
				t.Fatal("late preparation overwrote a changed scene/draft/input")
			}
			for _, p := range prefs.RecentFiles() {
				if p == path {
					t.Fatal("stale preparation updated MRU")
				}
			}
		})
	}
}
func TestManualDesignRecoveryChecksAgainAfterModal(t *testing.T) {
	for _, mutate := range []bool{false, true} {
		t.Run(map[bool]string{false: "recovered", true: "modal-edit"}[mutate], func(t *testing.T) {
			w, prefs := recentOpenWorkbench(t)
			path := designOpenPath(t, "original")
			autosave := path + ".autosave"
			writeRecoveryDesign(t, autosave, "Recovered", "Pending")
			future := time.Now().Add(time.Hour)
			if err := os.Chtimes(autosave, future, future); err != nil {
				t.Fatal(err)
			}
			beforeOriginal, beforeRecovery := diskText(t, path), diskText(t, autosave)
			asks := 0
			scene := w.canvas.GedScene()
			w.designOpen.settings.confirmRecovery = func(gui.IWidget, string, string) bool {
				asks++
				if mutate {
					scene.PushCommand(recoveryTitleEdit{scene, scene.FormTitle(), "Keep latest"})
				}
				return true
			}
			requestDesignOpen(path, w.tabs, w.canvas, true)
			pumpTestDesignOpens(t, w)
			if asks != 1 {
				t.Fatal("wrong recovery questions", asks)
			}
			if mutate {
				if scene.FormTitle() != "Keep latest" || scene.Filename() == path || !scene.UndoStack().CanUndo() || len(prefs.RecentFiles()) != 0 {
					t.Fatal("modal input was overwritten")
				}
			} else {
				if scene.Filename() != path || scene.IsClean() || recoveryButtonText(t, scene) != "Pending" || scene.UndoStack().Count() != 0 {
					t.Fatal("recovery lost original identity/dirty state")
				}
			}
			if diskText(t, path) != beforeOriginal || diskText(t, autosave) != beforeRecovery {
				t.Fatal("opening recovery changed disk")
			}
		})
	}
}
func TestManualDesignRecoveryCanRestoreMissingOriginal(t *testing.T) {
	for _, accept := range []bool{false, true} {
		t.Run(map[bool]string{false: "reject", true: "accept"}[accept], func(t *testing.T) {
			w := testWorkbench(t)
			path := filepath.Join(t.TempDir(), "missing.silkui")
			writeRecoveryDesign(t, path+".autosave", "Recovered", "Keep")
			scene := w.canvas.GedScene()
			before := scene.SaveDesign().String()
			w.designOpen.settings.confirmRecovery = func(gui.IWidget, string, string) bool { return accept }
			requestDesignOpen(path, w.tabs, w.canvas, true)
			pumpTestDesignOpens(t, w)
			if accept {
				if scene.Filename() != path || scene.IsClean() || recoveryButtonText(t, scene) != "Keep" {
					t.Fatal("valid recovery not offered for missing original")
				}
			} else if scene.SaveDesign().String() != before || scene.Filename() == path {
				t.Fatal("rejected recovery installed an empty design")
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("original created before Save")
			}
		})
	}
}
func TestManualDesignCancelMenusBindOwnerAndReleaseView(t *testing.T) {
	w := testWorkbench(t)
	path := designOpenPath(t, "target")
	entered, unblock := blockDesignReader(t, w)
	requestDesignOpen(path, w.tabs, w.canvas, true)
	<-entered
	menu := buildFileMenu(w.tabs, w.canvas)
	defer releaseWorkbenchMenu(menu)
	b := menuCommandButton(menu, "file.cancelDesignOpen")
	if b == nil || !b.IsEnabled() || b.Icon() == nil {
		t.Fatal("missing cancel action/icon")
	}
	oldPalette := paletteCommands
	t.Cleanup(func() { paletteCommands = oldPalette })
	registerPaletteCommands(w.tabs, w.canvas)
	var bound paletteCommand
	for _, cmd := range paletteCommands {
		if cmd.Name == i18n.T("Cancel Design Open") {
			bound = cmd.Bind()
		}
	}
	if bound.Available == nil || !bound.Available() {
		t.Fatal("missing bound palette cancel")
	}
	old := w.designOpen
	var j *designOpenJob
	for _, job := range old.all {
		j = job
	}
	box := j.box
	old.Close()
	if box.owner.Load() != nil || j.scene != nil || j.focus != nil || old.owner != nil || old.canvas != nil {
		t.Fatal("retired mailbox/handle retained UI")
	}
	installTestDesignOpener(t, w)
	requestDesignOpen(path, w.tabs, w.canvas, true)
	b.Action().Trigger(nil)
	bound.Fn()
	if !designOpenPending(w.tabs) || bound.Available() {
		t.Fatal("stale menu canceled replacement controller")
	}
	w.canvas.Close()
	if !w.designOpen.closed || w.designOpen.owner != nil {
		t.Fatal("view close did not retire pending I/O")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if !errors.Is(old.jobs.Wait(ctx), context.DeadlineExceeded) {
		t.Fatal("Close joined or pretended blocked read ended")
	}
	unblock()
}

func TestManualDesignPreparedSnapshotIsRetiredByRenameCommit(t *testing.T) {
	w, prefs := recentOpenWorkbench(t)
	path := designOpenPath(t, "before")
	next := filepath.Join(filepath.Dir(path), "after.silkui")
	captured, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	w.designOpen.settings.read = func(ctx context.Context, p string) (designOpenSnapshot, error) {
		value, err := readManualDesign(ctx, p)
		close(captured)
		<-release
		return value, err
	}
	scene := w.canvas.GedScene()
	before, filename := scene.SaveDesign().String(), scene.Filename()
	requestDesignOpen(path, w.tabs, w.canvas, true)
	<-captured
	plan, err := prepareEditorRename(w.tabs, w.canvas, path, next)
	if err != nil {
		t.Fatal(err)
	}
	if err = plan.Wait(context.Background()); err != nil {
		plan.Complete(err)
		t.Fatal(err)
	}
	err = os.Rename(path, next)
	plan.Complete(err)
	if err != nil {
		t.Fatal(err)
	}
	if designOpenPending(w.tabs) {
		t.Fatal("rename commit left an old-path snapshot admitted")
	}
	unblock()
	pumpTestDesignOpens(t, w)
	if scene.SaveDesign().String() != before || scene.Filename() != filename || len(prefs.RecentFiles()) != 0 {
		t.Fatal("old-path snapshot survived completed rename")
	}
	if _, err = os.Stat(next); err != nil {
		t.Fatal("rename did not actually commit", err)
	}
}

func TestManualDesignNewAndStartupRestorationKeepLatestScene(t *testing.T) {
	w := testWorkbench(t)
	startup := designOpenPath(t, "startup")
	manual := designOpenPath(t, "manual")
	restorePosts := make(chan func(), 16)
	captured, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	r := newWorkspaceRestore(workspaceSession{Design: startup, Mode: modeDesign}, w.tabs, w.canvas, workspaceRestoreOptions{post: func(f func()) { restorePosts <- f }, read: func(ctx context.Context, p string, d bool) (restoredFile, error) {
		value, err := readRestoredFile(ctx, p, d)
		close(captured)
		<-release
		return value, err
	}})
	t.Cleanup(r.Close)
	<-captured
	requestDesignOpen(manual, w.tabs, w.canvas, true)
	pumpTestDesignOpens(t, w)
	if r.designPending || r.deferredRecovery || r.designListener != 0 {
		t.Fatal("manual intent did not retire the startup design branch")
	}
	unblock()
	pumpRestore(t, restorePosts, func() bool { return !r.Busy() })
	if w.canvas.GedScene().Filename() != manual {
		t.Fatal("startup completion overwrote manually opened design")
	}
	// New invalidates a second prepared request even if its read ignores cancel.
	entered, unblock2 := blockDesignReader(t, w)
	requestDesignOpen(startup, w.tabs, w.canvas, true)
	<-entered
	newDesignCanvas(w.canvas)
	fresh := w.canvas.GedScene()
	before := fresh.SaveDesign().String()
	unblock2()
	pumpTestDesignOpens(t, w)
	if fresh == nil || fresh.Filename() != "" || w.canvas.GedScene() != fresh || fresh.SaveDesign().String() != before {
		t.Fatal("late open replaced New")
	}
}
