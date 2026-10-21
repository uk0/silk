package main

import (
	"context"
	"errors"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/settings"
)

func testPreferences(t *testing.T) *preferences {
	t.Helper()
	p := newPreferences()
	t.Cleanup(func() {
		if err := p.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return p
}
func flushTestPreferences(t *testing.T, p *preferences) {
	t.Helper()
	if err := p.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func waitPreferenceTest(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("preference worker timeout")
	}
}
func preferenceTestFlight(p *preferences) *preferenceFlight {
	w := p.saveWriter()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flight
}

func TestPreferencesBackgroundSaveCoalescesWithoutBlockingEditing(t *testing.T) {
	p := &preferences{store: settings.New(filepath.Join(t.TempDir(), "prefs"))}
	p.WindowSize() // preload like main's Appearance
	w := p.saveWriter()
	posts := make(chan func(), 1)
	w.post = func(fn func()) { posts <- fn }
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }); _ = p.Shutdown(context.Background()) })
	var calls atomic.Int32
	w.save = func() error {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		return p.store.Sync()
	}
	p.SetRunArgs("first")
	waitPreferenceTest(t, started)
	responsive := make(chan struct{})
	go func() {
		defer close(responsive)
		for i := 0; i < 10000; i++ {
			p.SetRunArgs("latest")
			_ = p.RunArgs()
		}
	}()
	waitPreferenceTest(t, responsive)
	ed := gui.NewCodeEditor()
	ed.SetText("original")
	ed.ReplaceAllText("typing while save is stalled")
	ed.Undo()
	if ed.Text() != "original" {
		t.Fatal("pending preferences broke typing/undo")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := p.Flush(ctx); !errors.Is(err, context.DeadlineExceeded) || diagnostic.CodeOf(err) != diagnostic.SettingsWait {
		t.Fatalf("wait error %v", err)
	}
	f := preferenceTestFlight(p)
	if f == nil {
		t.Fatal("timeout falsely joined active writer")
	}
	once.Do(func() { close(release) })
	waitPreferenceTest(t, f.done)
	if calls.Load() != 2 || len(posts) != 1 {
		t.Fatalf("coalescing: saves=%d posts=%d", calls.Load(), len(posts))
	}
	if got := settings.New(p.store.Path()).String("run/args", ""); got != "latest" {
		t.Fatalf("saved %q", got)
	}
	if err := finishPreferenceSaves(p); err != nil {
		t.Fatal(err)
	}
	if preferenceTestFlight(p) != nil {
		t.Fatal("finalizer stranded writer")
	}
	(<-posts)() // retired callback cannot touch the closed host
	if err := p.requestSave(); diagnostic.CodeOf(err) != diagnostic.SettingsWrite {
		t.Fatal("closed writer accepted a new request", err)
	}
}

func TestPreferencesFailureFeedbackIsBoundedAndRetryable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs")
	p := &preferences{store: settings.New(path)}
	w := p.saveWriter()
	posts := make(chan func(), 1)
	w.post = func(fn func()) { posts <- fn }
	var reported []error
	w.report = func(err error) { reported = append(reported, err) }
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })
	p.SetRunArgs("old")
	flushTestPreferences(t, p)
	(<-posts)()
	reported = nil
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	backup := path + ".original"
	if err = os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		p.SetRunArgs("new")
		if err = p.Flush(context.Background()); diagnostic.CodeOf(err) != diagnostic.SettingsWrite {
			t.Fatal(err)
		}
	}
	if len(posts) != 1 || !p.store.IsDirty() || p.RunArgs() != "new" {
		t.Fatal("failure storm or lost in-memory state")
	}
	(<-posts)()
	if len(reported) != 1 || diagnostic.CodeOf(reported[0]) != diagnostic.SettingsWrite {
		t.Fatalf("feedback %v", reported)
	}
	message := preferenceErrorDetails(reported[0])
	if !strings.Contains(message, path) || !strings.Contains(message, "SILK-SET002") || !strings.Contains(message, "Pending settings remain dirty") {
		t.Fatal("missing path/code/hint", message)
	}
	saved, err := os.ReadFile(backup)
	if err != nil || string(saved) != string(original) {
		t.Fatal("original settings lost", err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(backup, path); err != nil {
		t.Fatal(err)
	}
	flushTestPreferences(t, p)
	(<-posts)()
	if len(reported) != 2 || reported[1] != nil || p.store.IsDirty() {
		t.Fatal("success did not retire previous failure")
	}
	if settings.New(path).String("run/args", "") != "new" {
		t.Fatal("retry did not persist runtime state")
	}
}

func TestPreferencesShutdownWaitsAndRetiresQueuedFeedback(t *testing.T) {
	p := &preferences{store: settings.NewMemory()}
	w := p.saveWriter()
	posts := make(chan func(), 1)
	w.post = func(fn func()) { posts <- fn }
	reported := 0
	w.report = func(error) { reported++ }
	p.SetRunArgs("first")
	flushTestPreferences(t, p)
	if len(posts) != 1 {
		t.Fatal("missing first notification")
	}
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }); _ = p.Shutdown(context.Background()) })
	w.save = func() error {
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
		return p.store.Sync()
	}
	p.SetRunArgs("last")
	waitPreferenceTest(t, started)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := p.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("shutdown did not join actual work", err)
	}
	(<-posts)()
	if reported != 0 {
		t.Fatal("queued callback survived shutdown")
	}
	f := preferenceTestFlight(p)
	if f == nil {
		t.Fatal("writer vanished while still blocked")
	}
	once.Do(func() { close(release) })
	waitPreferenceTest(t, f.done)
	if err := finishPreferenceSaves(p); err != nil {
		t.Fatal(err)
	}
	if len(posts) != 0 {
		t.Fatal("late UI wakeup after shutdown")
	}
}

func TestMainJoinsPreferencesAfterEventLoop(t *testing.T) {
	fn := parseMainFunc(t)
	prefs := boundBy(fn, "newPreferences", 0)
	loop, join := -1, -1
	arg := ""
	for i, stmt := range fn.Body.List {
		es, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := es.X.(*ast.CallExpr)
		if !ok {
			continue
		}
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if fun.Sel.Name == "EventLoop" {
				loop = i
			}
		case *ast.Ident:
			if fun.Name == "finishPreferenceSaves" {
				join = i
				if len(call.Args) == 1 {
					if id, ok := call.Args[0].(*ast.Ident); ok {
						arg = id.Name
					}
				}
			}
		}
	}
	if loop < 0 || join <= loop || prefs == "" || arg != prefs {
		t.Fatalf("main must join its actual preferences after dispatch: loop=%d join=%d prefs=%s arg=%s", loop, join, prefs, arg)
	}
}
