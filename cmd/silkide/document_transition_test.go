package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func transitionSourceFixture(t *testing.T) (*workbench, *gui.CodeEditor, string) {
	t.Helper()
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "source.txt", "original")
	if !openTestEditorReady(t, w.tabs, path) {
		t.Fatal("open source")
	}
	return w, openEditors[path], path
}
func answerTransitionQuit(t *testing.T, choice gui.DialogResult) {
	t.Helper()
	old := askQuitDirtyEditors
	askQuitDirtyEditors = func([]string) gui.DialogResult { return choice }
	t.Cleanup(func() { askQuitDirtyEditors = old })
}

func TestAsyncTransitionQuitSaveDiscardCancel(t *testing.T) {
	for _, choice := range []gui.DialogResult{gui.DialogOK, gui.DialogNo, gui.DialogCancel} {
		t.Run(string(rune('A'+choice)), func(t *testing.T) {
			w, ed, path := transitionSourceFixture(t)
			ed.ReplaceAllText("edited😀")
			answerTransitionQuit(t, choice)
			exits := 0
			if !requestQuitDocuments(w.tabs, w.canvas, func() { exits++ }) {
				t.Fatal("quit not admitted")
			}
			if exits != 0 {
				t.Fatal("quit proceeded on read/save admission")
			}
			pumpTestTransitions(t, w)
			if choice == gui.DialogCancel {
				if exits != 0 || ed.Text() != "edited😀" || diskText(t, path) != "original" {
					t.Fatal("cancel lost edits")
				}
				return
			}
			if exits != 1 {
				t.Fatal("quit continuation not exactly once", exits)
			}
			want := "original"
			if choice == gui.DialogOK {
				want = "edited😀"
			}
			if diskText(t, path) != want {
				t.Fatal("wrong durable decision", diskText(t, path))
			}
		})
	}
}

func TestAsyncTransitionQuitInspectsDiskDriftAndRefusesLaterConflict(t *testing.T) {
	w, ed, path := transitionSourceFixture(t)
	if err := os.WriteFile(path, []byte("external observed"), 0644); err != nil {
		t.Fatal(err)
	}
	if ed.Modified() {
		t.Fatal("fixture must be UI clean")
	}
	old := askQuitDirtyEditors
	asks := 0
	askQuitDirtyEditors = func(paths []string) gui.DialogResult {
		asks++
		if len(paths) != 1 || paths[0] != path {
			t.Error("disk drift ignored", paths)
		}
		if err := os.WriteFile(path, []byte("external after question"), 0644); err != nil {
			t.Error(err)
		}
		return gui.DialogOK
	}
	t.Cleanup(func() { askQuitDirtyEditors = old })
	exits := 0
	requestQuitDocuments(w.tabs, w.canvas, func() { exits++ })
	pumpTestTransitions(t, w)
	if asks != 1 || exits != 0 || diskText(t, path) != "external after question" || ed.Text() != "original" {
		t.Fatal("observed conflict overwritten or quit allowed", asks, exits)
	}
}

func TestAsyncTransitionCloseReturnsBeforePhysicalReadAndKeepsNewInput(t *testing.T) {
	w, ed, path := transitionSourceFixture(t)
	c := w.transitions
	read := c.settings.read
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	c.settings.read = func(ctx context.Context, path string, limit int64) ([]byte, bool, error) {
		close(entered)
		<-release
		return read(ctx, path, limit)
	}
	old := askCloseDirtyEditor
	askCloseDirtyEditor = func(*gui.CodeEditor, string) gui.DialogResult {
		t.Error("newer edit reached old prompt")
		return gui.DialogNo
	}
	t.Cleanup(func() { askCloseDirtyEditor = old })
	if requestCloseEditorTab(w.tabs, 0) {
		t.Fatal("admission reported a removed page")
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("reader not entered")
	}
	if w.tabs.Count() != 1 || !anySaveBusy() {
		t.Fatal("read admission dropped page or physical slot")
	}
	ed.ReplaceAllText("new input")
	unblock()
	pumpTestTransitions(t, w)
	if w.tabs.Count() != 1 || openEditors[path] != ed || ed.Text() != "new input" || anySaveBusy() {
		t.Fatal("late close lost current input")
	}
	t.Log("production_tab_close_return_before_physical_read_release=true")
}

func TestAsyncTransitionNativeClosePostsOnlyAfterDurableSave(t *testing.T) {
	w, ed, path := transitionSourceFixture(t)
	ed.ReplaceAllText("native close saved")
	answerTransitionQuit(t, gui.DialogOK)
	exits := 0
	w.frame.SetClosedCallback(func(*gui.Frame) { exits++ })
	installQuitGuard(w.frame, w.canvas)
	if w.frame.CanClose() || w.frame.CanClose() || exits != 0 {
		t.Fatal("native CanClose admitted teardown or duplicate gesture")
	}
	pumpTestTransitions(t, w)
	if exits != 1 || diskText(t, path) != "native close saved" {
		t.Fatal("native close did not wait for actual save", exits)
	}
}

func TestAsyncTransitionNewDesignDraftSaveAndCancel(t *testing.T) {
	for _, choice := range []gui.DialogResult{gui.DialogOK, gui.DialogNo, gui.DialogCancel} {
		t.Run(string(rune('A'+choice)), func(t *testing.T) {
			w, p, _ := designerDraftFixture(t)
			s := w.canvas.GedScene()
			path := s.Filename()
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			bindDesignDisk(s, path, core.FingerprintBytes(data))
			oldAsk := askDiscardDirtyDesign
			askDiscardDirtyDesign = func(*ged.GedView) gui.DialogResult { return choice }
			t.Cleanup(func() { askDiscardDirtyDesign = oldAsk })
			oldAuto := globalAutoSaver
			t.Cleanup(func() {
				if globalAutoSaver != nil && globalAutoSaver != oldAuto {
					globalAutoSaver.Stop()
				}
				globalAutoSaver = oldAuto
			})
			newDesignCanvas(w.canvas)
			if w.canvas.GedScene() != s {
				t.Fatal("new scene installed before dirty decision completion")
			}
			pumpTestTransitions(t, w)
			if choice == gui.DialogCancel {
				if w.canvas.GedScene() != s || !p.HasPendingChanges() {
					t.Fatal("cancel committed/dropped draft")
				}
				return
			}
			if w.canvas.GedScene() == s || w.canvas.GedScene().Filename() != "" {
				t.Fatal("new continuation missing")
			}
			doc, err := core.LoadTDocFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(doc.String(), "println(42)") != (choice == gui.DialogOK) {
				t.Fatal("wrong persisted draft decision")
			}
		})
	}
}

func TestAsyncTransitionOwnerRetirementKeepsPhysicalReadSlot(t *testing.T) {
	w, ed, path := transitionSourceFixture(t)
	c := w.transitions
	read := c.settings.read
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	c.settings.read = func(ctx context.Context, path string, limit int64) ([]byte, bool, error) {
		close(entered)
		<-release
		return read(ctx, path, limit)
	}
	calls := 0
	if !c.begin("close:"+path, nil, []transitionSource{{editor: ed, path: path}}, false, func(ok bool) {
		calls++
		if ok {
			t.Error("retired transition continued")
		}
	}) {
		t.Fatal("admission")
	}
	<-entered
	box := c.active.box
	c.Close()
	if calls != 1 || box.owner.Load() != nil || c.active != nil || c.owner != nil || !anySaveBusy() {
		t.Fatal("UI retirement released blocked IO or retained owner")
	}
	unblock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.settings.queue.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || anySaveBusy() || openEditors[path] != ed {
		t.Fatal("late read changed retired UI")
	}
}

func TestAsyncTransitionCloseSaveKeepsNewTypingDuringPhysicalWrite(t *testing.T) {
	w, ed, path := transitionSourceFixture(t)
	c := w.transitions
	saver := newOwnedSourceSaver(w, sourceSaveSettings{queue: c.settings.queue, post: c.settings.post})
	entered, unblock := holdSourceWriter(t, saver)
	old := askCloseDirtyEditor
	askCloseDirtyEditor = func(*gui.CodeEditor, string) gui.DialogResult { return gui.DialogOK }
	t.Cleanup(func() { askCloseDirtyEditor = old })
	ed.ReplaceAllText("captured close save")
	requestCloseEditorTab(w.tabs, 0)
	pumpTestTransitionUntil(t, w, func() bool { return saver.pending != nil })
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("writer not entered")
	}
	ed.ReplaceAllText("newer input during close save")
	unblock()
	pumpTestTransitions(t, w)
	if openEditors[path] != ed || w.tabs.Count() != 1 || ed.Text() != "newer input during close save" || !ed.Modified() || diskText(t, path) != "captured close save" {
		t.Fatal("close lost a newer buffer")
	}
}

func TestAsyncTransitionQuitRevalidatesFinalPostedPermit(t *testing.T) {
	w, ed, path := transitionSourceFixture(t)
	ed.ReplaceAllText("saved before final post")
	answerTransitionQuit(t, gui.DialogOK)
	exits := 0
	requestQuitDocuments(w.tabs, w.canvas, func() { exits++ })
	pumpTestTransitionUntil(t, w, func() bool {
		return w.transitions.active != nil && w.transitions.active.next == 1 && w.sourceSaves != nil && w.sourceSaves.pending == nil
	})
	ed.ReplaceAllText("typed before posted teardown")
	pumpTestTransitions(t, w)
	if exits != 0 || ed.Text() != "typed before posted teardown" || !ed.Modified() || diskText(t, path) != "saved before final post" {
		t.Fatal("posted close ignored newer input", exits)
	}
}

func TestAsyncTransitionChangedDraftCannotReuseDiscardDecision(t *testing.T) {
	w, p, _ := designerDraftFixture(t)
	s := w.canvas.GedScene()
	old := askDiscardDirtyDesign
	askDiscardDirtyDesign = func(*ged.GedView) gui.DialogResult {
		p.HandlerEditor().ReplaceAllText("func latest(){println(99)}")
		return gui.DialogNo
	}
	t.Cleanup(func() { askDiscardDirtyDesign = old })
	newDesignCanvas(w.canvas)
	pumpTestTransitions(t, w)
	if w.canvas.GedScene() != s || !p.HasPendingChanges() || !strings.Contains(p.HandlerEditor().Text(), "latest") {
		t.Fatal("old discard decision dropped a newer draft")
	}
}

func TestAsyncTransitionCancelOpenDuringSaveChooserPreventsLateWrite(t *testing.T) {
	w := testWorkbench(t)
	s := w.canvas.GedScene()
	s.SetFilename("")
	s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "dirty chooser"})
	oldAsk, oldChoose, oldAfter := askDiscardDirtyDesign, chooseDesignSavePath, afterDesignSaveChooser
	t.Cleanup(func() {
		askDiscardDirtyDesign = oldAsk
		chooseDesignSavePath = oldChoose
		afterDesignSaveChooser = oldAfter
	})
	askDiscardDirtyDesign = func(*ged.GedView) gui.DialogResult { return gui.DialogOK }
	saved := filepath.Join(t.TempDir(), "must-not-save.silkui")
	chooseDesignSavePath = func() string {
		if !cancelOwnedDesignOpen(w.designOpen) {
			t.Error("guarded open not canceled")
		}
		return saved
	}
	afterDesignSaveChooser = w.transitions.settings.post
	target := designOpenPath(t, "target")
	if !requestDesignOpen(target, w.tabs, w.canvas, true) {
		t.Fatal("open not admitted")
	}
	select {
	case f := <-testTransitionPosts[w]:
		f()
	case <-time.After(5 * time.Second):
		t.Fatal("no save decision callback")
	}
	select {
	case f := <-testTransitionPosts[w]:
		f()
	case <-time.After(5 * time.Second):
		t.Fatal("no native chooser cleanup callback")
	}
	if w.canvas.GedScene() != s || s.Filename() != "" || !s.HasPendingEdits() && s.IsClean() || designOpenPending(w.tabs) || anySaveBusy() {
		t.Fatal("retired chooser changed current design")
	}
	if _, err := os.Stat(saved); !os.IsNotExist(err) {
		t.Fatal("retired chooser dispatched late physical save", err)
	}
}

func TestAsyncTransitionOpenWaitsForSaveAndRejectsNewerDesignEdits(t *testing.T) {
	for _, editDuring := range []bool{false, true} {
		t.Run(fmt.Sprint(editDuring), func(t *testing.T) {
			w := testWorkbench(t)
			s := w.canvas.GedScene()
			before := s.Filename()
			s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "captured design"})
			c := newOwnedDesignSaver(w, ownedDesignSaveSettings{queue: w.transitions.settings.queue, post: w.transitions.settings.post})
			entered, unblock := blockOwnedWriter(t, c)
			old := askDiscardDirtyDesign
			askDiscardDirtyDesign = func(*ged.GedView) gui.DialogResult { return gui.DialogOK }
			t.Cleanup(func() { askDiscardDirtyDesign = old })
			target := designOpenPath(t, "opened target")
			if !requestDesignOpen(target, w.tabs, w.canvas, true) {
				t.Fatal("open not admitted")
			}
			pumpTestTransitionUntil(t, w, func() bool { return c.pending != nil })
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("writer not entered")
			}
			if w.designOpen.pending != nil || s.Filename() != before {
				t.Fatal("open read/install happened before save completion")
			}
			if editDuring {
				s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "newer design"})
			}
			unblock()
			pumpTestDesignOpens(t, w)
			if editDuring {
				if s.Filename() != before || s.FormTitle() != "newer design" || s.IsClean() {
					t.Fatal("late open discarded new design input")
				}
			} else if s.Filename() != target || s.FormTitle() != "opened target" {
				t.Fatal("successful save did not continue owned open")
			}
		})
	}
}

type transitionCancelDraft struct {
	pending bool
	cancel  func()
}

func (d *transitionCancelDraft) HasPendingChanges() bool { return d.pending }
func (d *transitionCancelDraft) CommitPendingChanges() error {
	d.pending = false
	d.cancel()
	return nil
}
func TestAsyncTransitionCancelDuringDraftCommitPreventsOldWriterAdmission(t *testing.T) {
	w := testWorkbench(t)
	s := w.canvas.GedScene()
	path := s.Filename()
	s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "dirty"})
	s.AddPendingEditor(&transitionCancelDraft{true, func() { cancelOwnedDesignOpen(w.designOpen) }})
	old := askDiscardDirtyDesign
	askDiscardDirtyDesign = func(*ged.GedView) gui.DialogResult { return gui.DialogOK }
	t.Cleanup(func() { askDiscardDirtyDesign = old })
	if !requestDesignOpen(designOpenPath(t, "target"), w.tabs, w.canvas, true) {
		t.Fatal("open admission")
	}
	select {
	case f := <-testTransitionPosts[w]:
		f()
	case <-time.After(5 * time.Second):
		t.Fatal("no save decision")
	}
	if transitionBusy() || designOpenPending(w.tabs) || anySaveBusy() || w.designSaves.pending != nil {
		t.Fatal("retired draft callback admitted an old writer")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("canceled draft commit wrote the original", err)
	}
}

func TestAsyncTransitionChoiceWaitsForNestedPhysicalSaveAndPublication(t *testing.T) {
	for _, choice := range []gui.DialogResult{gui.DialogNo, gui.DialogOK} {
		for _, newInput := range []bool{false, true} {
			t.Run(fmt.Sprint(choice, "/", newInput), func(t *testing.T) {
				w, ed, path := transitionSourceFixture(t)
				ed.ReplaceAllText("pending manual save")
				c := w.transitions
				saver := newOwnedSourceSaver(w, sourceSaveSettings{queue: c.settings.queue, post: c.settings.post})
				entered, unblock := holdSourceWriter(t, saver)
				old := askQuitDirtyEditors
				askQuitDirtyEditors = func([]string) gui.DialogResult {
					if !requestSourceDocumentSave(ed, path, nil) {
						t.Error("nested explicit save not admitted")
					}
					return choice
				}
				t.Cleanup(func() { askQuitDirtyEditors = old })
				exits := 0
				requestQuitDocuments(w.tabs, w.canvas, func() { exits++ })
				pumpTestTransitionUntil(t, w, func() bool { return c.active != nil && c.active.waitingWrites })
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					t.Fatal("nested writer not entered")
				}
				if exits != 0 || c.settings.queue.Stats().Queued {
					t.Fatal("teardown bypassed write or a barrier replaced queued work")
				}
				if newInput {
					ed.ReplaceAllText("new input after discard question")
				}
				unblock()
				pumpTestTransitions(t, w)
				if diskText(t, path) != "pending manual save" {
					t.Fatal("in-flight save disappeared")
				}
				if newInput {
					if exits != 0 || ed.Text() != "new input after discard question" || !ed.Modified() {
						t.Fatal("newer input lost at queued teardown")
					}
				} else if exits != 1 {
					t.Fatal("quit failed to resume after physical and UI completion", exits)
				}
			})
		}
	}
}

func TestAsyncTransitionWaitsForAlreadyPendingSourceSave(t *testing.T) {
	for _, newInput := range []bool{false, true} {
		t.Run(fmt.Sprint(newInput), func(t *testing.T) {
			w := testWorkbench(t)
			p := writeFile(t, t.TempDir(), "saved.go", "package p\n")
			if !openTestEditorReady(t, w.tabs, p) {
				t.Fatal("open")
			}
			ed := openEditors[p]
			ed.ReplaceAllText("package p\nvar X=1\n")
			c := w.transitions
			saver := newOwnedSourceSaver(w, sourceSaveSettings{queue: c.settings.queue, post: c.settings.post})
			entered, unblock := holdSourceWriter(t, saver)
			if !requestSourceDocumentSave(ed, p, nil) {
				t.Fatal("save not admitted")
			}
			<-entered
			exits := 0
			if !requestQuitDocuments(w.tabs, w.canvas, func() { exits++ }) || c.active == nil || !c.active.initialWait || exits != 0 {
				t.Fatal("pending quit was dropped or proceeded")
			}
			if newInput {
				ed.ReplaceAllText("package p\nvar X=2\n")
			}
			unblock()
			pumpTestTransitions(t, w)
			if diskText(t, p) != "package p\n\nvar X = 1\n" {
				t.Fatal("pending formatted save lost", diskText(t, p))
			}
			if newInput {
				if exits != 0 || !ed.Modified() || ed.Text() != "package p\nvar X=2\n" {
					t.Fatal("later input discarded")
				}
			} else if exits != 1 || ed.Modified() {
				t.Fatal("quit did not resume after formatting", exits)
			}
		})
	}
}

func TestAsyncTransitionWaitsForAlreadyPendingDesignSave(t *testing.T) {
	for _, change := range []string{"none", "model", "draft", "undo-redo"} {
		t.Run(change, func(t *testing.T) {
			w := testWorkbench(t)
			c := w.transitions
			saver := newOwnedDesignSaver(w, ownedDesignSaveSettings{queue: c.settings.queue, post: c.settings.post})
			s := w.canvas.GedScene()
			s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "pending design"})
			entered, unblock := blockOwnedWriter(t, saver)
			if !requestDesignDocumentSave(w.canvas, nil) {
				t.Fatal("save not admitted")
			}
			<-entered
			exits := 0
			if !requestQuitDocuments(w.tabs, w.canvas, func() { exits++ }) || c.active == nil || !c.active.initialWait {
				t.Fatal("pending quit rejected")
			}
			switch change {
			case "model":
				s.NotifyDesignChanged()
			case "undo-redo":
				s.UndoStack().Undo()
				s.UndoStack().Redo()
			case "draft":
				s.AddPendingEditor(&checkpointTransitionDraft{pending: true})
			}
			unblock()
			pumpTestTransitions(t, w)
			if _, err := core.LoadTDocFile(s.Filename()); err != nil {
				t.Fatal("pending physical save missing", err)
			}
			if change == "none" && exits != 1 || change != "none" && exits != 0 {
				t.Fatal("wrong final permission", change, exits)
			}
		})
	}
}

type checkpointTransitionDraft struct{ pending bool }

func (d *checkpointTransitionDraft) HasPendingChanges() bool     { return d.pending }
func (d *checkpointTransitionDraft) CommitPendingChanges() error { d.pending = false; return nil }

func TestAsyncTransitionClosingOpenerRetiresOnlyItsGuardedSave(t *testing.T) {
	w := testWorkbench(t)
	s := w.canvas.GedScene()
	s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "guarded save"})
	saver := newOwnedDesignSaver(w, ownedDesignSaveSettings{queue: w.transitions.settings.queue, post: w.transitions.settings.post})
	entered, unblock := blockOwnedWriter(t, saver)
	old := askDiscardDirtyDesign
	askDiscardDirtyDesign = func(*ged.GedView) gui.DialogResult { return gui.DialogOK }
	t.Cleanup(func() { askDiscardDirtyDesign = old })
	if !requestDesignOpen(designOpenPath(t, "target"), w.tabs, w.canvas, true) {
		t.Fatal("open admission")
	}
	pumpTestTransitionUntil(t, w, func() bool { return saver.pending != nil })
	<-entered
	w.designOpen.Close()
	if transitionBusy() || saver.pending != nil || !anySaveBusy() || w.canvas.GedScene() != s {
		t.Fatal("opener retirement leaked guard or freed physical writer")
	}
	unblock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := saver.settings.queue.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if anySaveBusy() || s.IsClean() || w.canvas.GedScene() != s {
		t.Fatal("retired completion adopted or reservations leaked")
	}
	if _, err := core.LoadTDocFile(s.Filename()); err != nil {
		t.Fatal("retirement erased actual physical result", err)
	}
}

func TestAsyncTransitionWaitsAcrossAlreadyPendingSaveAllGaps(t *testing.T) {
	for _, newInput := range []bool{false, true} {
		t.Run(fmt.Sprint(newInput), func(t *testing.T) {
			w, saver, posts, paths := testSaveBatch(t)
			c := w.transitions
			c.settings.queue, c.settings.post, c.settings.afterModal = saver.settings.queue, saver.settings.post, saver.settings.post
			testTransitionPosts[w] = posts
			s := w.canvas.GedScene()
			s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "save all design"})
			finished := false
			if !requestSaveAllDocuments(w.tabs, w.canvas, func(bool) { finished = true }) {
				t.Fatal("batch not admitted")
			}
			if anySaveBusy() {
				t.Fatal("fixture did not capture pre-worker batch gap")
			}
			exits := 0
			if !requestQuitDocuments(w.tabs, w.canvas, func() { exits++ }) {
				t.Fatal("pending batch quit rejected")
			}
			if newInput {
				openEditors[paths[1]].ReplaceAllText("later input")
			}
			pumpTestTransitions(t, w)
			if !finished || saveBatchBusy() || anySaveBusy() {
				t.Fatal("quit bypassed batch completion")
			}
			if newInput {
				if exits != 0 || openEditors[paths[1]].Text() != "later input" || !openEditors[paths[1]].Modified() {
					t.Fatal("later input discarded")
				}
			} else {
				if exits != 1 || !s.IsClean() {
					t.Fatal("quit failed to resume", exits)
				}
				for _, p := range paths {
					if diskText(t, p) != "changed "+filepath.Base(p) {
						t.Fatal("batch target not saved", p)
					}
				}
			}
		})
	}
}
