package hmiapp

import (
	"context"
	"github.com/uk0/silk/hmi"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGeneratedProjectCompilesAndDocumentRoundTrips(t *testing.T) {
	p := hmi.Template()
	b, e := Generate(p)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "plant.silkhmi")
	if e = hmi.Save(path, p); e != nil {
		t.Fatal(e)
	}
	if _, e = hmi.Load(path); e != nil {
		t.Fatal(e)
	}
	if testing.Short() {
		return
	}
	dir := t.TempDir()
	root, e := filepath.Abs("..")
	if e != nil {
		t.Fatal(e)
	}
	module := "module generatedhmi\n\ngo 1.26.10\n\nrequire github.com/uk0/silk v0.0.0\nreplace github.com/uk0/silk => " + filepath.ToSlash(root) + "\n"
	if e = os.WriteFile(filepath.Join(dir, "go.mod"), []byte(module), 0600); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(dir, "main.go"), b, 0600)
	cmd := exec.CommandContext(context.Background(), "go", "build", "-mod=mod", "-p", "1", ".")
	cmd.Dir = dir
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("generated Go does not build: %v\n%s", e, out)
	}
}

func TestWorkbenchCloseJoinsStartupAndRemainsIdempotent(t *testing.T) {
	for i := 0; i < 5; i++ {
		f, err := New(Config{DatabasePath: filepath.Join(t.TempDir(), "points.db")})
		if err != nil {
			t.Fatal(err)
		}
		f.StartRuntime()
		session := f.session
		if session == nil {
			t.Fatal("run not started")
		}
		f.Close()
		f.Close()
		select {
		case <-session.done:
		case <-time.After(10 * time.Second):
			t.Fatal("startup worker survived Close")
		}
		f.Join()
		f.StartRuntime()
		if f.session != session {
			t.Fatal("closed workbench restarted")
		}
	}
}

func TestWorkbenchControlsRepresentIOAndRuntimeAvailability(t *testing.T) {
	f, err := New(Config{ProjectPath: filepath.Join(t.TempDir(), "plant.silkhmi"), DatabasePath: filepath.Join(t.TempDir(), "points.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Join()
	defer f.Close()
	if !f.buttons[0].IsEnabled() || f.buttons[1].IsEnabled() || !f.buttons[2].IsEnabled() {
		t.Fatal("initial actions inconsistent")
	}
	f.busy = true
	f.refreshControls()
	if f.buttons[0].IsEnabled() || f.buttons[2].IsEnabled() || !f.Editor.Diagram.IsEditing() || !f.Editor.Diagram.IsReadOnly() {
		t.Fatal("IO lock failed or entered live preview")
	}
	f.busy = false
	f.refreshControls()
	if !f.Editor.Diagram.IsEditing() || f.Editor.Diagram.IsReadOnly() {
		t.Fatal("IO lock did not restore editing")
	}
	f.Save()
	if !f.busy || !f.Editor.Diagram.IsReadOnly() {
		t.Fatal("save not represented synchronously")
	}
	f.Close()
	f.Join()
	if _, err = hmi.Load(f.cfg.ProjectPath); err != nil {
		t.Fatal("joined save did not persist", err)
	}
	for _, b := range f.buttons {
		if b.IsEnabled() {
			t.Fatal("closed workbench has enabled actions")
		}
	}
}

func TestWorkbenchPointPanelToggleIsInertAndLiveLayoutDoesNotOverlap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "points.db")
	f, err := New(Config{DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Join()
	defer f.Close()
	f.SetSize(640, 560)
	f.TogglePoints()
	if !f.Points.IsVisible() || len(f.Points.PointStates()) != 0 || f.session != nil {
		t.Fatal("view toggle started a runtime")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("inert panel opened database", err)
	}
	// A synthetic active session exercises the actual live viewport policy
	// without introducing a timing-only startup/pump assertion into this test.
	f.session = &session{cancel: func() {}, done: make(chan struct{})}
	close(f.session.done)
	f.refreshControls()
	f.OnResize()
	if f.Editor.Y()+f.Editor.Height() > f.Points.Y() || f.Points.Y()+f.Points.Height() > f.status.Y() {
		t.Fatal("runtime panel overlaps canvas/status")
	}
	if f.Editor.Diagram.Height() < 150 {
		t.Fatal("disabled authoring toolbar obscures live diagram")
	}
	f.TogglePoints()
	if f.Points.IsVisible() || len(f.Points.PointStates()) != 0 {
		t.Fatal("hidden panel kept active view")
	}
}

func TestWorkbenchRuntimeAvailabilityEventsAreOnceAndReentrantStopSafe(t *testing.T) {
	var events []*hmi.Runtime
	f, err := New(Config{RuntimeChanged: func(rt *hmi.Runtime) { events = append(events, rt) }})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Join()
	defer f.Close()
	if f.Runtime() != nil {
		t.Fatal("inert workbench has runtime")
	}
	rt, err := hmi.NewRuntime(context.Background(), hmi.Template(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop()
	s := &session{runtime: rt, cancel: func() {}, done: make(chan struct{})}
	close(s.done)
	f.session = s
	f.notifyRuntime(s, rt)
	f.notifyRuntime(s, rt)
	if f.Runtime() != rt || len(events) != 1 || events[0] != rt {
		t.Fatal("ready event missing/duplicated")
	}
	f.cfg.RuntimeChanged = func(rt *hmi.Runtime) {
		events = append(events, rt)
		if rt == nil {
			f.StopRuntime()
		}
	}
	f.StopRuntime()
	f.StopRuntime()
	if f.Runtime() != nil || len(events) != 2 || events[1] != nil {
		t.Fatal("stop did not revoke availability once")
	}
	f.Close()
	if len(events) != 2 {
		t.Fatal("close repeated unavailable event")
	}
}

func TestWorkbenchPointPanelCannotRebindUnavailableRuntime(t *testing.T) {
	f, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Join()
	defer f.Close()
	rt, err := hmi.NewRuntime(context.Background(), hmi.Template(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop()
	s := &session{runtime: rt, cancel: func() {}, done: make(chan struct{})}
	close(s.done)
	f.session = s
	f.TogglePoints()
	if len(f.Points.PointStates()) != 0 {
		t.Fatal("panel bound a runtime before its availability event")
	}
	f.TogglePoints()
	f.notifyRuntime(s, rt)
	f.TogglePoints()
	if len(f.Points.PointStates()) == 0 {
		t.Fatal("available runtime did not bind")
	}
	f.StopRuntime()
	// The session may remain while its workers/store drain. Showing the panel
	// again must not resurrect the revoked runtime or its subscriptions.
	f.TogglePoints()
	f.TogglePoints()
	if !f.Points.IsVisible() || len(f.Points.PointStates()) != 0 {
		t.Fatal("panel rebound a stopping runtime")
	}
}
