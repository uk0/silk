package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func testSourceSaver(t *testing.T, ext string) (*workbench, *ownedSourceSaver, *gui.CodeEditor, string, chan func()) {
	t.Helper()
	w := testWorkbench(t)
	path := filepath.Join(t.TempDir(), "source"+ext)
	if err := os.WriteFile(path, []byte("original\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !openTestEditorReady(t, w.tabs, path) {
		t.Fatal("open source")
	}
	ed := openEditors[path]
	c, posts := installTestSourceSaver(t, w)
	return w, c, ed, path, posts
}

func installTestSourceSaver(t *testing.T, w *workbench) (*ownedSourceSaver, chan func()) {
	t.Helper()
	posts := make(chan func(), 128)
	q := async.NewWriteQueue[designWriteResult]()
	c := newOwnedSourceSaver(w, sourceSaveSettings{queue: q, post: func(f func()) { posts <- f }})
	t.Cleanup(func() {
		c.Close()
		q.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := q.Wait(ctx); err != nil {
			t.Error(err)
		}
	})
	return c, posts
}

func holdSourceWriter(t *testing.T, c *ownedSourceSaver) (<-chan struct{}, func()) {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	write := c.settings.write
	c.settings.write = func(path string, data []byte, v core.FileFingerprint) (core.FileFingerprint, error) {
		close(entered)
		<-release
		return write(path, data, v)
	}
	return entered, unblock
}

func TestSourceSavePublicCommandReturnsBeforePhysicalWriter(t *testing.T) {
	w, c, ed, path, posts := testSourceSaver(t, ".txt")
	w.activate(modeEdit)
	ed.ReplaceAllText("captured😀\n")
	entered, unblock := holdSourceWriter(t, c)
	completed := false
	if !requestCurrentDocumentSave(w.tabs, w.canvas, func(ok bool) {
		if !ok {
			t.Error("save not adopted")
		}
		completed = true
	}) {
		t.Fatal("command rejected")
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("writer not entered")
	}
	if completed || !ed.Modified() {
		t.Fatal("admission treated as successful save")
	}
	field := gui.NewEdit()
	field.SetParent(w.frame)
	field.SetFocus()
	if data, _ := os.ReadFile(path); string(data) != "original\n" {
		t.Fatal("blocked writer changed disk")
	}
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return completed })
	if ed.Modified() || diskText(t, path) != "captured😀\n" {
		t.Fatal("actual completion not published")
	}
	if !field.HasFocus() || strings.Contains(w.frame.Title(), " *") {
		t.Fatal("save completion stole focus or left stale dirty window title", w.frame.Title())
	}
	t.Log("production_source_save_command_return_before_writer_release=true")
}

func TestSourceSaveNewerInputAndExternalConflict(t *testing.T) {
	for _, external := range []bool{false, true} {
		t.Run(map[bool]string{false: "newer-input", true: "external-conflict"}[external], func(t *testing.T) {
			_, c, ed, path, posts := testSourceSaver(t, ".txt")
			ed.ReplaceAllText("captured\n")
			entered, unblock := holdSourceWriter(t, c)
			var out *sourceSaveOutcome
			if ok, err := c.request(ed, path, func(r sourceSaveOutcome) { out = &r }); !ok || err != nil {
				t.Fatal(ok, err)
			}
			<-entered
			if external {
				if err := os.WriteFile(path, []byte("external\n"), 0644); err != nil {
					t.Fatal(err)
				}
			} else {
				ed.ReplaceAllText("new input\n")
			}
			unblock()
			pumpOwnedSaves(t, posts, func() bool { return out != nil })
			if external {
				if !errors.Is(out.Err, core.ErrSaveConflict) || out.Written || diskText(t, path) != "external\n" || !ed.Modified() {
					t.Fatal(out)
				}
			} else {
				if out.Err != nil || !out.Written || out.Adopted || ed.Text() != "new input\n" || ed.SavedTextSnapshot() != "captured\n" || !ed.Modified() {
					t.Fatal(out)
				}
			}
		})
	}
}

func TestSourceSaveReservationRetainsUIAndPhysicalLifetime(t *testing.T) {
	w, c, ed, path, posts := testSourceSaver(t, ".txt")
	ed.ReplaceAllText("save\n")
	entered, unblock := holdSourceWriter(t, c)
	var out *sourceSaveOutcome
	if ok, err := c.request(ed, path, func(r sourceSaveOutcome) { out = &r }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	<-entered
	if !savePathBusy(path) || closeEditorTab(w.tabs, editorTabIndex(w.tabs, ed)) || canQuit(w.canvas) {
		t.Fatal("pending writer did not reserve document")
	}
	unblock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.settings.queue.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if !savePathBusy(path) {
		t.Fatal("fast worker released before UI publication")
	}
	pumpOwnedSaves(t, posts, func() bool { return out != nil })
	if savePathBusy(path) || anySaveBusy() {
		t.Fatal("completed save leaked reservation")
	}
}

func TestSourceSaveLatestQueuedUsesActualPhysicalBaseline(t *testing.T) {
	_, c, ed, path, posts := testSourceSaver(t, ".txt")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	write := c.settings.write
	var calls atomic.Int32
	c.settings.write = func(path string, data []byte, v core.FileFingerprint) (core.FileFingerprint, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return write(path, data, v)
	}
	ed.ReplaceAllText("first")
	firstDone := 0
	if ok, err := c.request(ed, path, func(r sourceSaveOutcome) {
		firstDone++
		if !errors.Is(r.Err, async.ErrSuperseded) {
			t.Error(r)
		}
	}); !ok || err != nil {
		t.Fatal(ok, err)
	}
	<-entered
	ed.ReplaceAllText("latest")
	var out *sourceSaveOutcome
	if ok, err := c.request(ed, path, func(r sourceSaveOutcome) { out = &r }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return out != nil })
	if firstDone != 1 || out.Err != nil || !out.Adopted || diskText(t, path) != "latest" || ed.Modified() {
		t.Fatal(firstDone, out)
	}
}

func TestSourceSaveCloseSeversUIAndRetainsPhysicalReservation(t *testing.T) {
	_, c, ed, path, _ := testSourceSaver(t, ".txt")
	ed.ReplaceAllText("physically committed after retirement")
	entered, unblock := holdSourceWriter(t, c)
	calls := 0
	if ok, err := c.request(ed, path, func(r sourceSaveOutcome) {
		calls++
		if !errors.Is(r.Err, errSaveOwnerClosed) {
			t.Error(r)
		}
	}); !ok || err != nil {
		t.Fatal(ok, err)
	}
	<-entered
	box := c.pending.box
	c.Close()
	if calls != 1 || box.owner.Load() != nil || c.pending != nil || c.owner != nil || !savePathBusy(path) {
		t.Fatal("logical close lost physical reservation or retained UI")
	}
	unblock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.settings.queue.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || savePathBusy(path) || ed.SavedTextSnapshot() != "original\n" || diskText(t, path) != "physically committed after retirement" {
		t.Fatal("late result touched retired UI or disappeared")
	}
}

func TestSourceSaveFormatsOffUIAndHonorsCRLF(t *testing.T) {
	_, c, ed, path, posts := testSourceSaver(t, ".go")
	ed.ReplaceAllText("package main\nfunc f(){println(1)}\n")
	if err := ed.ConvertLineEndings(gui.LineEndingCRLF); err != nil {
		t.Fatal(err)
	}
	var out *sourceSaveOutcome
	if ok, err := c.request(ed, path, func(r sourceSaveOutcome) { out = &r }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	pumpOwnedSaves(t, posts, func() bool { return out != nil })
	if out.Err != nil || out.FormatErr != nil || !out.Adopted || diskText(t, path) != "package main\r\n\r\nfunc f() { println(1) }\r\n" {
		t.Fatal(out, diskText(t, path))
	}
}

func TestSourceSaveDefersBaselineHashUntilPhysicalWorker(t *testing.T) {
	_, c, ed, path, posts := testSourceSaver(t, ".txt")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	if _, err := c.settings.queue.Submit(context.Background(), func(context.Context) (designWriteResult, error) {
		close(entered)
		<-release
		return designWriteResult{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	<-entered
	ed.ReplaceAllText("new snapshot")
	var out *sourceSaveOutcome
	if ok, err := c.request(ed, path, func(r sourceSaveOutcome) { out = &r }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	state := c.bindings[ed].state
	if state.version.Load() != nil || state.initialText.Load() == nil {
		t.Fatal("source baseline was hashed on UI admission")
	}
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return out != nil })
	if out.Err != nil || !out.Adopted || state.version.Load() == nil || state.initialText.Load() != nil {
		t.Fatal("worker did not publish/release source baseline", out)
	}
}

func TestSourceSaveAdmissionSeparatesTypingWithoutBreakingNewerGroup(t *testing.T) {
	_, c, ed, path, posts := testSourceSaver(t, ".txt")
	ed.ReplaceAllText("")
	ed.SetFocus()
	ed.OnTextInput("a")
	ed.OnTextInput("b")
	entered, unblock := holdSourceWriter(t, c)
	var out *sourceSaveOutcome
	if ok, err := c.request(ed, path, func(r sourceSaveOutcome) { out = &r }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	<-entered
	ed.OnTextInput("c")
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return out != nil })
	if out.Err != nil || out.Adopted || ed.Text() != "abc" {
		t.Fatal(out, ed.Text())
	}
	ed.OnTextInput("d")
	ed.Undo()
	if ed.Text() != "ab" || ed.Modified() {
		t.Fatal("save admission or late completion broke typing groups", ed.Text())
	}
	ed.Redo()
	if ed.Text() != "abcd" || !ed.Modified() {
		t.Fatal("redo lost post-save typing", ed.Text())
	}
}
