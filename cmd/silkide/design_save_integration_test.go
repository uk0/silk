package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
)

func TestDesignSavePublicCommandReturnsBeforePhysicalWriter(t *testing.T) {
	w, c, posts := testOwnedDesignSaver(t)
	w.activate(modeDesign)
	s := w.canvas.GedScene()
	s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "public save"})
	entered, unblock := blockOwnedWriter(t, c)
	completed := false
	if !requestCurrentDocumentSave(w.tabs, w.canvas, func(ok bool) {
		if !ok {
			t.Error("not adopted")
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
	if completed || s.IsClean() {
		t.Fatal("admission claimed physical completion")
	}
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return completed })
	if !s.IsClean() {
		t.Fatal("actual save not adopted")
	}
	t.Log("production_design_save_command_return_before_writer_release=true")
}

func TestDesignSaveQueuedSamePathUsesActualPredecessorBaseline(t *testing.T) {
	w, c, posts := testOwnedDesignSaver(t)
	s := w.canvas.GedScene()
	path := s.Filename()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var calls atomic.Int32
	c.settings.write = func(_ context.Context, snapshot *ged.DesignWriteSnapshot, v core.FileFingerprint) error {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return snapshot.WriteAgainst(v)
	}
	s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "first"})
	firstDone := 0
	if ok, err := c.request(path, func(r designSaveOutcome) {
		firstDone++
		if !errors.Is(r.Err, async.ErrSuperseded) {
			t.Error(r)
		}
	}); !ok || err != nil {
		t.Fatal(ok, err)
	}
	<-entered
	s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "latest"})
	var out *designSaveOutcome
	if ok, err := c.request(path, func(r designSaveOutcome) { out = &r }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return out != nil })
	if firstDone != 1 || out.Err != nil || !out.Written || !out.Adopted {
		t.Fatal(firstDone, out)
	}
	doc, err := core.LoadTDocFile(path)
	if err != nil || !strings.Contains(doc.String(), "latest") {
		t.Fatal("latest own write conflicted with predecessor", err)
	}
}

func TestDesignSaveExternalQueuedChangeIsNotOverwritten(t *testing.T) {
	w, c, posts := testOwnedDesignSaver(t)
	s := w.canvas.GedScene()
	path := s.Filename()
	entered, unblock := blockOwnedWriter(t, c)
	s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "ours"})
	var out *designSaveOutcome
	if ok, err := c.request(path, func(r designSaveOutcome) { out = &r }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	<-entered
	if err := os.WriteFile(path, []byte("external replacement"), 0644); err != nil {
		t.Fatal(err)
	}
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return out != nil })
	if !errors.Is(out.Err, core.ErrSaveConflict) || out.Written || out.Adopted || s.IsClean() || diskText(t, path) != "external replacement" {
		t.Fatal(out)
	}
}

func TestDesignRecoveryAndManualWriteSharePhysicalOrder(t *testing.T) {
	w, c, posts := testOwnedDesignSaver(t)
	s := w.canvas.GedScene()
	path := s.Filename()
	s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "recovery"})
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	c.settings.recovery = func(_ context.Context, doc *core.TDoc, path string) error {
		close(entered)
		<-release
		return doc.SaveFileAtomic(path)
	}
	requestDesignRecovery(s)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("recovery not started")
	}
	if !anySaveBusy() || !savePathBusy(path+".autosave") {
		t.Fatal("recovery did not retain physical reservation")
	}
	s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "manual latest"})
	var out *designSaveOutcome
	if ok, err := c.request(path, func(r designSaveOutcome) { out = &r }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	// Another timer tick cannot replace the queued manual save.
	requestDesignRecovery(s)
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return out != nil })
	if !out.Written || !out.Adopted || out.Err != nil || out.RecoveryErr != nil {
		t.Fatal(out)
	}
	if _, err := os.Stat(path + ".autosave"); !os.IsNotExist(err) {
		t.Fatal("old recovery survived ordered manual cleanup", err)
	}
	if anySaveBusy() {
		t.Fatal("physical recovery/manual reservation leaked")
	}
}

func TestDesignSavePreservesRecoveryChangedDuringPrimaryWrite(t *testing.T) {
	w, c, posts := testOwnedDesignSaver(t)
	s := w.canvas.GedScene()
	path := s.Filename()
	if err := os.WriteFile(path+".autosave", []byte("old recovery"), 0644); err != nil {
		t.Fatal(err)
	}
	entered, unblock := blockOwnedWriter(t, c)
	var out *designSaveOutcome
	if ok, err := c.request(path, func(r designSaveOutcome) { out = &r }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	<-entered
	if err := os.WriteFile(path+".autosave", []byte("external recovery update"), 0644); err != nil {
		t.Fatal(err)
	}
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return out != nil })
	if out.Err != nil || !out.Written || !out.Adopted || !errors.Is(out.RecoveryErr, core.ErrSaveConflict) || diskText(t, path+".autosave") != "external recovery update" {
		t.Fatal("changed recovery deleted or primary success erased", out)
	}
}

func TestDesignSaveRecoveryCleanupPanicKeepsCommittedPrimaryResult(t *testing.T) {
	w, c, posts := testOwnedDesignSaver(t)
	c.settings.cleanup = func(string, core.FileFingerprint) error { panic("cleanup failure") }
	var out *designSaveOutcome
	if ok, err := c.request(w.canvas.GedScene().Filename(), func(r designSaveOutcome) { out = &r }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	pumpOwnedSaves(t, posts, func() bool { return out != nil })
	if out.Err != nil || !out.Written || !out.Adopted || out.RecoveryErr == nil {
		t.Fatal("cleanup panic erased primary success", out)
	}
}
