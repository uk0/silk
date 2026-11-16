package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/hotreload"
)

func reloadWorkbench(t *testing.T) (*workbench, *hotreload.Reloader, string) {
	t.Helper()
	w := testWorkbench(t)
	path := designOpenPath(t, "original")
	if err := openDesignDocument(w.canvas.GedScene(), path, ""); err != nil {
		t.Fatal(err)
	}
	w.canvas.SetFocus()
	old, oldBridge, retired := globalReloader, globalDesignReloadBridge, retiredIDEReloaders
	globalReloader, globalDesignReloadBridge, retiredIDEReloaders = nil, nil, nil
	startReloader(w.canvas)
	owner := globalReloader
	if owner == nil || w.designOpen.reloadBridge == nil {
		t.Fatal("real IDE invalidation watcher not installed")
	}
	t.Cleanup(func() {
		closeIDEReloader()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := waitIDEReloaders(ctx); err != nil {
			t.Error(err)
		}
		globalReloader, globalDesignReloadBridge, retiredIDEReloaders = old, oldBridge, retired
	})
	return w, owner, path
}

func admitTestReload(t *testing.T, w *workbench, owner *hotreload.Reloader, path string) {
	t.Helper()
	if accepted, err := reloadOwnedCleanDesign(owner, w.canvas, path); err != nil || !accepted {
		t.Fatal("reload not admitted", accepted, err)
	}
}

func blockReloadReader(t *testing.T, w *workbench) (<-chan string, func()) {
	t.Helper()
	entered, release := make(chan string, 4), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	w.designOpen.settings.readReload = func(ctx context.Context, path string) (designOpenSnapshot, error) {
		entered <- path
		<-release
		return readReloadDesign(ctx, path)
	}
	return entered, unblock
}

func nextReloadPost(t *testing.T, w *workbench) func() {
	t.Helper()
	select {
	case f := <-testDesignOpenPosts[w]:
		return f
	case <-time.After(3 * time.Second):
		t.Fatal("reload UI post timeout")
		return nil
	}
}

func TestDesignReloadPublicFlowReturnsBeforePhysicalReader(t *testing.T) {
	w, owner, path := reloadWorkbench(t)
	scene := w.canvas.GedScene()
	before, originalWidget := scene.SaveDesign().String(), scene.Children()[0]
	writeRecoveryDesign(t, path, "External 😀", "Unicode 外部")
	entered, unblock := blockReloadReader(t, w)
	returned, result := make(chan struct{}), make(chan bool, 1)
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
	admitTestReload(t, w, owner, path)
	close(returned)
	if !<-result {
		t.Fatal("public reload admission waited for physical source I/O")
	}
	if scene.SaveDesign().String() != before || scene.Children()[0] != originalWidget {
		t.Fatal("admission pretended the source was already installed")
	}
	w.designOpen.settings.installed = func(*ged.GedView, string, string, designOpenSnapshot) {
		t.Fatal("reload used explicit-open MRU/autosaver/watcher host effects")
	}
	pumpTestDesignOpens(t, w)
	if scene.FormTitle() != "External 😀" || recoveryButtonText(t, scene) != "Unicode 外部" || !scene.IsClean() || scene.Filename() != path {
		t.Fatal("prepared Unicode snapshot was not installed exactly")
	}
	t.Log("reload_ui_return_before_physical_reader_released=true; original scene retained until owned UI completion")
}

func TestDesignReloadUsesPreparedBytesWithoutAnyUIReread(t *testing.T) {
	w, owner, path := reloadWorkbench(t)
	writeRecoveryDesign(t, path, "Prepared snapshot", "Captured before removal")
	admitTestReload(t, w, owner, path)
	completion := nextReloadPost(t, w)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	completion()
	pumpTestDesignOpens(t, w)
	if w.canvas.GedScene().FormTitle() != "Prepared snapshot" || recoveryButtonText(t, w.canvas.GedScene()) != "Captured before removal" {
		t.Fatal("UI apply reopened the path instead of consuming captured bytes")
	}
}

func TestDesignReloadPreservesDesignerMetadataAndHandlerSource(t *testing.T) {
	w, owner, path := reloadWorkbench(t)
	external := ged.NewGedScene()
	external.SetFormTitle("Complete designer snapshot")
	external.SetGrid(ged.GridModel{Pitch: 7, Visible: false, Snap: false})
	button, err := ged.NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}
	button.SetWidgetName("metadata_probe")
	button.SetBounds(17.25, 21.5, 35, 10)
	button.SetCode("// handler source 😀\nprintln(42)")
	button.Widget().(*gui.Button).SetText("Metadata value")
	button.SetParent(external)
	if err := external.SaveDesign().SaveFile(path); err != nil {
		t.Fatal(err)
	}
	admitTestReload(t, w, owner, path)
	pumpTestDesignOpens(t, w)
	scene := w.canvas.GedScene()
	loaded := scene.Children()[0].(*ged.FakeWidget)
	if scene.SaveDesign().String() != external.SaveDesign().String() || loaded.GetCode() != button.GetCode() || scene.Grid() != external.Grid() {
		t.Fatal("reload lost raw designer metadata/precision/handler source through an AST projection")
	}
}

func TestDesignReloadOwnSaveRetainsWidgetsAndUndoAndSkipsRecovery(t *testing.T) {
	w, owner, path := reloadWorkbench(t)
	scene := w.canvas.GedScene()
	scene.PushCommand(recoveryTitleEdit{scene, scene.FormTitle(), "Saved edit"})
	if !scene.Save() {
		t.Fatal("save fixture")
	}
	widget := scene.Children()[0]
	writeRecoveryDesign(t, path+".autosave", "Not an explicit recovery", "Must not install")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(path+".autosave", future, future); err != nil {
		t.Fatal(err)
	}
	w.designOpen.settings.confirmRecovery = func(gui.IWidget, string, string) bool { t.Fatal("reload prompted recovery"); return false }
	admitTestReload(t, w, owner, path)
	pumpTestDesignOpens(t, w)
	if scene.Children()[0] != widget || !scene.UndoStack().CanUndo() || !scene.IsClean() || scene.FormTitle() != "Saved edit" {
		t.Fatal("self-save invalidation rebuilt widgets, history or used an autosave sibling")
	}
	scene.UndoStack().Undo()
	if scene.FormTitle() != "original" || scene.IsClean() || scene.Children()[0] != widget {
		t.Fatal("saved edit can no longer undo against its original widgets")
	}
}

func TestDesignReloadRejectsInterveningState(t *testing.T) {
	for _, action := range []string{"edit-undo", "notification-aba", "draft", "filename", "scene", "focus", "capture", "mode-aba", "reloader", "close"} {
		t.Run(action, func(t *testing.T) {
			w, owner, path := reloadWorkbench(t)
			writeRecoveryDesign(t, path, "External stale", "Do not install")
			entered, unblock := blockReloadReader(t, w)
			admitTestReload(t, w, owner, path)
			<-entered
			scene := w.canvas.GedScene()
			switch action {
			case "edit-undo":
				scene.PushCommand(recoveryTitleEdit{scene, scene.FormTitle(), "Local edit"})
				scene.UndoStack().Undo()
			case "notification-aba":
				before := scene.FormTitle()
				scene.SetFormTitle("Local temporary")
				scene.NotifyDesignChanged()
				scene.SetFormTitle(before)
				scene.NotifyDesignChanged()
			case "draft":
				scene.AddPendingEditor(&restoreDraft{pending: true})
			case "filename":
				scene.SetFilename(filepath.Join(t.TempDir(), "retarget.silkui"))
			case "scene":
				w.canvas.SetScene(ged.NewGedScene())
			case "focus":
				field := gui.NewEdit()
				field.SetParent(w.frame)
				field.SetText("foreign input")
				field.SetFocus()
			case "capture":
				w.canvas.PushCapture()
				defer w.canvas.PopCapture()
			case "mode-aba":
				w.activate(modeWelcome)
				w.activate(modeDesign)
			case "reloader":
				closeIDEReloader()
				startReloader(w.canvas)
			case "close":
				w.canvas.Close()
			}
			current := w.canvas.GedScene()
			before, focus, count := current.SaveDesign().String(), gui.CurrentFocusWidget(), current.UndoStack().Count()
			unblock()
			// Close revokes Busy immediately, but the already-admitted physical
			// reader must still finish and its queued completion must be harmless.
			if action == "close" {
				nextReloadPost(t, w)()
			} else {
				pumpTestDesignOpens(t, w)
			}
			if w.canvas.GedScene() != current || current.SaveDesign().String() != before || current.UndoStack().Count() != count || gui.CurrentFocusWidget() != focus {
				t.Fatal("late reload overwrote new scene/edit/draft/input/lifetime state")
			}
		})
	}
}

func TestDesignReloadSharesPhysicalReaderAndNeverOverridesManualOpen(t *testing.T) {
	w, owner, path := reloadWorkbench(t)
	entered, unblock := blockReloadReader(t, w)
	admitTestReload(t, w, owner, path)
	<-entered
	next := designOpenPath(t, "explicit target")
	if !requestDesignOpen(next, w.tabs, w.canvas, false) {
		t.Fatal("explicit open not admitted")
	}
	intent := w.designOpen.intent
	for i := 0; i < 1000; i++ {
		if accepted, err := reloadOwnedCleanDesign(owner, w.canvas, path); err != nil || accepted {
			t.Fatal("automatic invalidation superseded explicit user intent")
		}
	}
	if w.designOpen.active != 1 || len(w.designOpen.all) != 2 || w.designOpen.intent != intent || w.designOpen.pending.path != next {
		t.Fatal("reload and explicit open escaped the shared physical/metadata budget")
	}
	select {
	case <-entered:
		t.Fatal("supersession started another physical reader")
	default:
	}
	unblock()
	pumpTestDesignOpens(t, w)
	if w.canvas.GedScene().Filename() != next || w.canvas.GedScene().FormTitle() != "explicit target" {
		t.Fatal("automatic old target displaced the explicit open")
	}
}

func TestDesignReloadCannotSupersedeManualOpenDuringAdmission(t *testing.T) {
	w, owner, path := reloadWorkbench(t)
	next := designOpenPath(t, "explicit after prompt")
	w.designOpen.settings.confirmDiscard = func(*ged.GedView) bool {
		if accepted, err := reloadOwnedCleanDesign(owner, w.canvas, path); err != nil || accepted {
			t.Error("nested automatic invalidation stole a manual admission reservation", accepted, err)
		}
		return true
	}
	if !requestDesignOpen(next, w.tabs, w.canvas, false) {
		t.Fatal("explicit open was lost during its guarded admission")
	}
	pumpTestDesignOpens(t, w)
	if w.canvas.GedScene().Filename() != next {
		t.Fatal("nested automatic invalidation replaced explicit target")
	}
}

func TestDesignReloadMailboxCoalescesAndReleasesClosedOwner(t *testing.T) {
	w, owner, path := reloadWorkbench(t)
	b := w.designOpen.reloadBridge
	writeRecoveryDesign(t, path, "Latest external", "Burst final")
	var signals sync.WaitGroup
	for i := 0; i < 1000; i++ {
		signals.Go(func() { _ = b.signal(path) })
	}
	signals.Wait()
	if len(testDesignOpenPosts[w]) != 1 || w.designOpen.Busy() {
		t.Fatal("watcher burst bypassed UI admission or queued one callback per event")
	}
	nextReloadPost(t, w)()
	pumpTestDesignOpens(t, w)
	if w.canvas.GedScene().FormTitle() != "Latest external" {
		t.Fatal("coalesced final snapshot not applied")
	}
	_ = b.signal(path)
	closeIDEReloader()
	nextReloadPost(t, w)()
	if b.controller.Load() != nil || b.reloader.Load() != nil || b.latest.Load() != nil || b.target.Load() != nil {
		t.Fatal("closed mailbox retained its old UI/source authority")
	}
	if accepted, _ := reloadOwnedCleanDesign(owner, w.canvas, path); accepted || w.designOpen.Busy() {
		t.Fatal("retired queued UI notification admitted another read")
	}
}

func TestDesignReloadHiddenCanvasNeverRevealsOrStealsEditorInput(t *testing.T) {
	w, owner, path := reloadWorkbench(t)
	source := writeFile(t, t.TempDir(), "notes.txt", "editor remains unchanged")
	openTestEditorReady(t, w.tabs, source)
	w.activate(modeEdit)
	ed := activeTestEditorReady(t, w.tabs)
	ed.SetFocus()
	writeRecoveryDesign(t, path, "Hidden external", "Background model")
	admitTestReload(t, w, owner, path)
	pumpTestDesignOpens(t, w)
	if w.canvas.GedScene().FormTitle() != "Hidden external" || w.center.ActiveView() != w.tabs || gui.CurrentFocusWidget() != ed || ed.Text() != "editor remains unchanged" {
		t.Fatal("automatic reload revealed a hidden canvas or changed source input")
	}
}

func TestDesignReloadFailuresPreserveSceneAndPhysicalSlots(t *testing.T) {
	for _, kind := range []string{"missing", "invalid", "timeout"} {
		t.Run(kind, func(t *testing.T) {
			w, owner, path := reloadWorkbench(t)
			before := w.canvas.GedScene().SaveDesign().String()
			var physical atomic.Int32
			switch kind {
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "invalid":
				if err := os.WriteFile(path, []byte("invalid-root"), 0600); err != nil {
					t.Fatal(err)
				}
			case "timeout":
				w.designOpen.settings.timeout = 100 * time.Millisecond
				entered, unblock := blockReloadReader(t, w)
				read := w.designOpen.settings.readReload
				w.designOpen.settings.readReload = func(ctx context.Context, path string) (designOpenSnapshot, error) {
					physical.Add(1)
					return read(ctx, path)
				}
				admitTestReload(t, w, owner, path)
				<-entered
				pumpTestDesignOpenUntil(t, w, func() bool { return w.designOpen.pending == nil })
				if w.designOpen.active != 1 || physical.Load() != 1 {
					t.Fatal("timeout freed a physically blocked slot")
				}
				w.designOpen.settings.timeout = time.Second
				admitTestReload(t, w, owner, path)
				if physical.Load() != 1 || w.designOpen.active != 1 || len(w.designOpen.all) != 2 {
					t.Fatal("reload replaced a still-blocked reader")
				}
				unblock()
			}
			if kind != "timeout" {
				admitTestReload(t, w, owner, path)
			}
			pumpTestDesignOpens(t, w)
			if w.canvas.GedScene().SaveDesign().String() != before || !w.canvas.GedScene().IsClean() {
				t.Fatal("read/error/timeout changed the existing scene")
			}
		})
	}
}
