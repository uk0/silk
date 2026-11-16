package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/uk0/silk/core"
)

func testSaveBatch(t *testing.T) (*workbench, *ownedSourceSaver, chan func(), []string) {
	t.Helper()
	w := testWorkbench(t)
	sources, posts := installTestSourceSaver(t, w)
	designs := newOwnedDesignSaver(w, ownedDesignSaveSettings{queue: sources.settings.queue, post: func(f func()) { posts <- f }})
	t.Cleanup(designs.Close)
	old := saveBatchPost
	saveBatchPost = func(f func()) { posts <- f }
	t.Cleanup(func() { saveBatchPost = old })
	var paths []string
	for _, name := range []string{"first.txt", "second.txt"} {
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, []byte("original"), 0644); err != nil {
			t.Fatal(err)
		}
		if !openTestEditorReady(t, w.tabs, p) {
			t.Fatal("open source")
		}
		openEditors[p].ReplaceAllText("changed " + name)
		paths = append(paths, p)
	}
	return w, sources, posts, paths
}

func TestAsyncSaveAllSequencesSourcesThenDesign(t *testing.T) {
	w, _, posts, paths := testSaveBatch(t)
	s := w.canvas.GedScene()
	s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "batch design"})
	completed, calls := false, 0
	if !requestSaveAllDocuments(w.tabs, w.canvas, func(ok bool) {
		calls++
		if !ok {
			t.Error("save all failed")
		}
		completed = true
	}) {
		t.Fatal("batch not admitted")
	}
	if !saveBatchBusy() || canQuit(w.canvas) {
		t.Fatal("batch UI interval not guarded")
	}
	pumpOwnedSaves(t, posts, func() bool { return completed })
	for _, p := range paths {
		if openEditors[p].Modified() || diskText(t, p) != "changed "+filepath.Base(p) {
			t.Fatal("source not saved", p)
		}
	}
	if _, err := core.LoadTDocFile(s.Filename()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !s.IsClean() || saveBatchBusy() || anySaveBusy() {
		t.Fatal("batch completion/cleanup", calls)
	}
}

func TestAsyncSaveAllKeepsNewerInputAndStopsRemainingTargets(t *testing.T) {
	w, c, posts, paths := testSaveBatch(t)
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
	completed := false
	if !requestSaveAllDocuments(w.tabs, w.canvas, func(ok bool) {
		if ok {
			t.Error("new input declared fully saved")
		}
		completed = true
	}) {
		t.Fatal("batch not admitted")
	}
	// Start the admitted first target while retaining control of UI delivery.
	select {
	case f := <-posts:
		f()
	case <-time.After(5 * time.Second):
		t.Fatal("no batch step")
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("writer not entered")
	}
	openEditors[paths[0]].ReplaceAllText("newer input")
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return completed })
	if openEditors[paths[0]].Text() != "newer input" || !openEditors[paths[0]].Modified() {
		t.Fatal("newer input lost")
	}
	if diskText(t, paths[1]) != "original" || !openEditors[paths[1]].Modified() {
		t.Fatal("remaining target was unexpectedly written")
	}
}

func TestAsyncSaveAllPhysicalFailureDoesNotAnnounceSuccess(t *testing.T) {
	w, c, posts, paths := testSaveBatch(t)
	c.settings.format = func(context.Context, string) (string, error) { t.Error("txt unexpectedly formatted"); return "", nil }
	if err := os.WriteFile(paths[0], []byte("external"), 0644); err != nil {
		t.Fatal(err)
	}
	completed := false
	if !requestSaveAllDocuments(w.tabs, w.canvas, func(ok bool) {
		if ok {
			t.Error("conflict treated as all saved")
		}
		completed = true
	}) {
		t.Fatal("batch not admitted")
	}
	pumpOwnedSaves(t, posts, func() bool { return completed })
	if diskText(t, paths[0]) != "external" || diskText(t, paths[1]) != "original" || anySaveBusy() || saveBatchBusy() {
		t.Fatal("conflict or batch cleanup broken")
	}
}
