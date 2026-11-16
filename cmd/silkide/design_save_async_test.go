package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

type ownedSaveTitleCommand struct {
	scene         *ged.GedScene
	before, after string
}

func TestOwnedDesignSaveCapturesTypedCompanionWithSamePrimary(t *testing.T) {
	w, c, posts := testOwnedDesignSaver(t)
	s := w.canvas.GedScene()
	s.SetFormTitle("Captured form")
	b, err := ged.NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatal(err)
	}
	b.SetWidgetName("Action")
	b.SetParent(s)
	b.Widget().(*gui.Button).SetText("captured caption")
	path := filepath.Join(t.TempDir(), "saved.silkui")
	pair, err := s.GenerateCodeFile(path, ged.CodeGenOptions{PackageName: "panels", TypeName: "StableUI"})
	if err != nil {
		t.Fatal(err)
	}
	entered, unblock := blockOwnedWriter(t, c)
	var out *designSaveOutcome
	if ok, err := c.request(path, func(r designSaveOutcome) { out = &r }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	<-entered
	s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "newer form"})
	b.Widget().(*gui.Button).SetText("newer caption")
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return out != nil })
	if !out.Written || out.Adopted || out.Err != nil || out.CompanionErr != nil {
		t.Fatal(out)
	}
	machine, err := os.ReadFile(pair.MachineFile)
	if err != nil || !strings.Contains(string(machine), "captured caption") || strings.Contains(string(machine), "newer caption") {
		t.Fatal("companion did not use the primary's captured state", err)
	}
	doc, err := core.LoadTDocFile(path)
	if err != nil || !strings.Contains(doc.String(), "captured caption") || strings.Contains(doc.String(), "newer caption") {
		t.Fatal("primary did not use captured state", err)
	}
}

func TestOwnedDesignSaveCompanionFailureDoesNotErasePrimarySuccess(t *testing.T) {
	for _, panicCompanion := range []bool{false, true} {
		t.Run(fmt.Sprint("panic=", panicCompanion), func(t *testing.T) {
			w, c, posts := testOwnedDesignSaver(t)
			c.settings.companion = func(string, *core.TDoc, *ged.CodeSnapshot) error {
				if panicCompanion {
					panic("broken companion")
				}
				return errors.New("broken companion")
			}
			path := filepath.Join(t.TempDir(), "saved.silkui")
			var out *designSaveOutcome
			if ok, err := c.request(path, func(r designSaveOutcome) { out = &r }); !ok || err != nil {
				t.Fatal(ok, err)
			}
			pumpOwnedSaves(t, posts, func() bool { return out != nil })
			if !out.Written || !out.Adopted || out.Err != nil || out.CompanionErr == nil || !w.canvas.GedScene().IsClean() {
				t.Fatal("companion warning erased committed primary outcome", out)
			}
			if _, err := core.LoadTDocFile(path); err != nil {
				t.Fatal("successful primary not readable", err)
			}
		})
	}
}

func TestOwnedDesignSavePrimaryFailureNeverRunsCompanion(t *testing.T) {
	_, c, posts := testOwnedDesignSaver(t)
	var calls atomic.Int32
	c.settings.companion = func(string, *core.TDoc, *ged.CodeSnapshot) error { calls.Add(1); return nil }
	var out *designSaveOutcome
	if ok, err := c.request(filepath.Join(t.TempDir(), "missing", "saved"), func(r designSaveOutcome) { out = &r }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	pumpOwnedSaves(t, posts, func() bool { return out != nil })
	if calls.Load() != 0 || out.Written || out.CompanionErr != nil || out.Err == nil {
		t.Fatal("companion ran after failed primary", calls.Load(), out)
	}
}

func TestOwnedDesignSaveCompanionRemainsInsidePhysicalSlotAfterRetirement(t *testing.T) {
	w, c, posts := testOwnedDesignSaver(t)
	s := w.canvas.GedScene()
	originalPath := s.Filename()
	s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "first capture"})
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var calls atomic.Int32
	c.settings.companion = func(path string, doc *core.TDoc, code *ged.CodeSnapshot) error {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return regenerateCompanionSnapshot(path, doc, code)
	}
	first := filepath.Join(t.TempDir(), "first.silkui")
	firstDone := 0
	if ok, err := c.request(first, func(r designSaveOutcome) {
		firstDone++
		if !errors.Is(r.Err, async.ErrSuperseded) {
			t.Error("first continuation not explicitly retired", r)
		}
	}); !ok || err != nil {
		t.Fatal(ok, err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("companion not started")
	}
	if firstDone != 0 || s.IsClean() || s.Filename() != originalPath {
		t.Fatal("UI claimed completion before companion returned")
	}
	if _, err := core.LoadTDocFile(first); err != nil {
		t.Fatal("primary was not actually committed before companion", err)
	}
	var latest *designSaveOutcome
	last := filepath.Join(t.TempDir(), "last.silkui")
	if ok, err := c.request(last, func(r designSaveOutcome) { latest = &r }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if stats := c.settings.queue.Stats(); !stats.Active || !stats.Queued || calls.Load() != 1 || firstDone != 1 {
		t.Fatal("retirement freed the active physical companion slot", stats, calls.Load(), firstDone)
	}
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return latest != nil })
	if calls.Load() != 2 || firstDone != 1 || !latest.Written || !latest.Adopted || latest.CompanionErr != nil || latest.Err != nil || s.Filename() != last {
		t.Fatal("latest save did not finish after retired companion", calls.Load(), latest)
	}
}

func (c *ownedSaveTitleCommand) Redo()        { c.scene.SetFormTitle(c.after) }
func (c *ownedSaveTitleCommand) Undo()        { c.scene.SetFormTitle(c.before) }
func (c *ownedSaveTitleCommand) Text() string { return "edit form title" }

func testOwnedDesignSaver(t *testing.T) (*workbench, *ownedDesignSaver, chan func()) {
	t.Helper()
	w := testWorkbench(t)
	posts := make(chan func(), 128)
	queue := async.NewWriteQueue[designWriteResult]()
	c := newOwnedDesignSaver(w, ownedDesignSaveSettings{queue: queue, post: func(f func()) { posts <- f }})
	t.Cleanup(func() {
		c.Close()
		queue.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := queue.Wait(ctx); err != nil {
			t.Error("physical writer did not finish", err)
		}
	})
	return w, c, posts
}
func pumpOwnedSaves(t *testing.T, posts chan func(), done func() bool) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for !done() {
		select {
		case f := <-posts:
			f()
		case <-timer.C:
			t.Fatal("owned save UI delivery timed out")
		}
	}
}
func blockOwnedWriter(t *testing.T, c *ownedDesignSaver) (<-chan struct{}, func()) {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	c.settings.write = func(_ context.Context, snapshot *ged.DesignWriteSnapshot, expected core.FileFingerprint) error {
		close(entered)
		<-release
		return snapshot.WriteAgainst(expected)
	}
	return entered, unblock
}

func TestOwnedDesignSaveAdmissionReturnsBeforePhysicalWriter(t *testing.T) {
	w, c, posts := testOwnedDesignSaver(t)
	entered, unblock := blockOwnedWriter(t, c)
	s := w.canvas.GedScene()
	original := s.Filename()
	path := filepath.Join(t.TempDir(), "saved")
	completed := 0
	var out designSaveOutcome
	admitted, err := requestOwnedDesignSave(path, func(r designSaveOutcome) { completed++; out = r })
	if err != nil || !admitted {
		t.Fatal(admitted, err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("writer not started")
	}
	if completed != 0 || s.Filename() != original {
		t.Fatal("admission claimed durable/UI success")
	}
	if _, err := os.Stat(path + ".silkui"); !os.IsNotExist(err) {
		t.Fatal("held writer unexpectedly saved", err)
	}
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return completed == 1 })
	if !out.Written || !out.Adopted || out.Err != nil || s.Filename() != path+".silkui" || !s.IsClean() {
		t.Fatal("actual completion not adopted", out)
	}
	t.Log("owned_save_api_return_before_writer_release=true; internal API check, production command tested separately")
}

func TestOwnedDesignSaveNewInputDoesNotBecomeFalselyClean(t *testing.T) {
	w, c, posts := testOwnedDesignSaver(t)
	entered, unblock := blockOwnedWriter(t, c)
	s := w.canvas.GedScene()
	original := s.Filename()
	var out *designSaveOutcome
	if ok, err := c.request(filepath.Join(t.TempDir(), "saved"), func(v designSaveOutcome) { out = &v }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	<-entered
	s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "newer UI input"})
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return out != nil })
	if !out.Written || out.Adopted || out.Err != nil || s.IsClean() || s.Filename() != original || s.FormTitle() != "newer UI input" {
		t.Fatal("late save overwrote current UI", out)
	}
}

func TestOwnedDesignSaveLatestQueuedAndExplicitRetirement(t *testing.T) {
	w, c, posts := testOwnedDesignSaver(t)
	s := w.canvas.GedScene()
	firstEntered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var calls atomic.Int32
	c.settings.write = func(_ context.Context, snapshot *ged.DesignWriteSnapshot, expected core.FileFingerprint) error {
		if calls.Add(1) == 1 {
			close(firstEntered)
			<-release
		}
		return snapshot.WriteAgainst(expected)
	}
	results := map[int]designSaveOutcome{}
	root := t.TempDir()
	for i := 0; i < 50; i++ {
		n := i
		path := filepath.Join(root, fmt.Sprintf("saved-%02d", i))
		if ok, err := c.request(path, func(v designSaveOutcome) {
			if _, exists := results[n]; exists {
				t.Error("duplicate continuation")
			}
			results[n] = v
		}); !ok || err != nil {
			t.Fatal(ok, err)
		}
		if i == 0 {
			<-firstEntered
		}
	}
	if len(results) != 49 || calls.Load() != 1 || c.pending == nil {
		t.Fatal("not one current UI receipt and one blocked physical writer", len(results), calls.Load())
	}
	for i := 0; i < 49; i++ {
		if !errors.Is(results[i].Err, async.ErrSuperseded) {
			t.Fatal("retired continuation lost explicit result", i, results[i])
		}
	}
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return len(results) == 50 })
	if calls.Load() != 2 || !results[49].Written || !results[49].Adopted || results[49].Err != nil || !s.IsClean() {
		t.Fatal("latest queued result not adopted", calls.Load(), results[49])
	}
}

func TestOwnedDesignSaveCloseSeversUIButRetainsPhysicalSlotForNewOwner(t *testing.T) {
	w, c, posts := testOwnedDesignSaver(t)
	entered, unblock := blockOwnedWriter(t, c)
	oldScene := w.canvas.GedScene()
	oldName := oldScene.Filename()
	oldPath := filepath.Join(t.TempDir(), "old")
	oldCalls := 0
	var oldResult designSaveOutcome
	if ok, err := c.request(oldPath, func(v designSaveOutcome) { oldCalls++; oldResult = v }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	<-entered
	box := c.pending.box
	c.Close()
	if box.owner.Load() != nil || c.pending != nil || c.owner != nil || oldCalls != 1 || !errors.Is(oldResult.Err, errSaveOwnerClosed) {
		t.Fatal("close retained UI receipt or lost continuation")
	}
	w2 := testWorkbench(t)
	newOwner := newOwnedDesignSaver(w2, ownedDesignSaveSettings{queue: c.settings.queue, post: func(f func()) { posts <- f }})
	t.Cleanup(newOwner.Close)
	newDone := false
	if ok, err := newOwner.request(filepath.Join(t.TempDir(), "new"), func(out designSaveOutcome) {
		if out.Err != nil || !out.Adopted {
			t.Error(out)
		}
		newDone = true
	}); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if stats := c.settings.queue.Stats(); !stats.Active || !stats.Queued {
		t.Fatal("new owner replaced blocked physical slot", stats)
	}
	unblock()
	pumpOwnedSaves(t, posts, func() bool { return newDone })
	if oldCalls != 1 || oldScene.Filename() != oldName {
		t.Fatal("retired owner received late UI completion")
	}
	if _, err := os.Stat(oldPath + ".silkui"); err != nil {
		t.Fatal("logical close incorrectly promised no physical write", err)
	}
}

func TestOwnedDesignSaveFailureLeavesIdentityAndRejectsWithoutCallback(t *testing.T) {
	w, c, posts := testOwnedDesignSaver(t)
	name := w.canvas.GedScene().Filename()
	calls := 0
	if ok, err := c.request("", func(designSaveOutcome) { calls++ }); ok || err == nil || calls != 0 {
		t.Fatal("invalid admission was ambiguous", ok, err, calls)
	}
	path := filepath.Join(t.TempDir(), "missing-parent", "bad")
	var out *designSaveOutcome
	if ok, err := c.request(path, func(v designSaveOutcome) { out = &v }); !ok || err != nil {
		t.Fatal(ok, err)
	}
	pumpOwnedSaves(t, posts, func() bool { return out != nil })
	if out.Written || out.Adopted || out.Err == nil || w.canvas.GedScene().Filename() != name {
		t.Fatal("failed worker changed identity", out)
	}
}
