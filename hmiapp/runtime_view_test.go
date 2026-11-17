package hmiapp

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/point"
	pointsql "github.com/uk0/silk/point/sqlite"
)

func runtimeViewProject() hmi.Project {
	return hmi.Project{Version: hmi.Version, Title: "View QA", Width: 640, Height: 360, Points: []hmi.Point{
		{Definition: point.Definition{Name: "DI11", Kind: point.Bool}, Initial: false},
		{Definition: point.Definition{Name: "AI11", Kind: point.Float}, Initial: 0.0},
	}, Nodes: []hmi.Node{{ID: "di", Kind: "valve", X: 20, Y: 20, Width: 100, Height: 80, Tag: "DI11"}, {ID: "ai", Kind: "value", X: 180, Y: 20, Width: 180, Height: 80, Tag: "AI11"}}}
}
func runtimeViewTest(t *testing.T, options RuntimeViewOptions) (*RuntimeView, chan func()) {
	t.Helper()
	v := NewRuntimeView(options)
	posts := make(chan func(), 128)
	v.post = func(f func()) { posts <- f }
	v.postContext = func(ctx context.Context, f func()) error {
		select {
		case posts <- f:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	t.Cleanup(func() {
		v.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := v.Wait(ctx); err != nil {
			t.Error(err)
		}
		for {
			select {
			case f := <-posts:
				f()
			default:
				return
			}
		}
	})
	return v, posts
}
func pumpRuntimeView(t *testing.T, posts chan func(), done func() bool) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !done() {
		select {
		case f := <-posts:
			f()
		case <-time.After(5 * time.Millisecond):
		case <-deadline.C:
			t.Fatal("runtime view callback timeout")
		}
	}
}
func TestRuntimeViewInertSnapshotOwnedModesAndCompactLayout(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-created")
	path := filepath.Join(dir, "points.db")
	v, _ := runtimeViewTest(t, RuntimeViewOptions{EnsureDatabaseDirectory: true})
	project := runtimeViewProject()
	if err := v.SetSnapshot(project, path); err != nil {
		t.Fatal(err)
	}
	project.Points[0].Initial = true
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("staging did filesystem I/O", err)
	}
	if v.Runtime() != nil || v.Busy() || len(v.Points.PointStates()) != 0 || v.Diagram.Project().Points[0].Initial != false {
		t.Fatal("snapshot not inert/owned")
	}
	for _, size := range [][2]float64{{320, 240}, {640, 360}, {1200, 440}} {
		v.SetSize(size[0], size[1])
		for _, b := range v.buttons {
			if b.X() < 0 || b.X()+b.Width() > v.Width()+0.01 {
				t.Fatal("toolbar escaped")
			}
		}
		if v.Points.Y()+v.Points.Height() > v.status.Y() || v.Diagram.Y()+v.Diagram.Height() > v.status.Y() {
			t.Fatal("body/status overlap")
		}
	}
	v.SetPointsMode(false)
	if v.Points.IsVisible() || !v.Diagram.IsVisible() {
		t.Fatal("diagram mode failed")
	}
	v.SetPointsMode(true)
	if !v.Points.IsVisible() || v.Diagram.IsVisible() {
		t.Fatal("points mode failed")
	}
	v.SetStartGuard(func() bool { return false })
	v.Start()
	if v.Busy() {
		t.Fatal("host guard bypassed")
	}
	v.Invalidate()
	v.SetStartGuard(nil)
	v.Start()
	if v.Busy() {
		t.Fatal("invalid snapshot restarted")
	}
}

func TestRuntimeViewSnapshotFitsNewProjectRatherThanTemplate(t *testing.T) {
	v, _ := runtimeViewTest(t, RuntimeViewOptions{})
	v.SetSize(1200, 600)
	project := runtimeViewProject()
	if err := v.SetSnapshot(project, filepath.Join(t.TempDir(), "points.db")); err != nil {
		t.Fatal(err)
	}
	want := math.Min(v.Diagram.Width()/project.Width, v.Diagram.Height()/project.Height)
	if math.Abs(v.Diagram.Zoom()-want) > 1e-9 {
		t.Fatalf("new project retained template camera: zoom=%v want=%v", v.Diagram.Zoom(), want)
	}
}

func TestRuntimeViewSourceButtonRoutesActiveSelectionWithoutIO(t *testing.T) {
	v, posts := runtimeViewTest(t, RuntimeViewOptions{EnsureDatabaseDirectory: true})
	v.SetSize(1000, 500)
	p := runtimeViewProject()
	p.Links = []hmi.Link{{ID: "flow", From: hmi.Endpoint{Node: "di", Port: "east"}, To: hmi.Endpoint{Node: "ai", Port: "west"}}}
	db := filepath.Join(t.TempDir(), "new", "points.db")
	if err := v.SetSnapshot(p, db); err != nil {
		t.Fatal(err)
	}
	var events []hmi.SourceTarget
	v.SetSourceAction(func() { events = append(events, hmi.SourceTarget{}) })
	v.SetSourceTargetAction(func(s hmi.SourceTarget) { events = append(events, s) })
	action := v.buttons[5].Action()
	v.SetPointsMode(false)
	before := v.Diagram.ProjectJSON()
	z := v.Diagram.Zoom()
	v.Diagram.OnLeftDown(70*z, 65*z)
	action.Trigger(nil)
	v.Diagram.OnLeftDown(150*z, 60*z)
	action.Trigger(nil)
	if v.Busy() || v.Diagram.ProjectJSON() != before {
		t.Fatal("navigation starts acquisition or edits canvas")
	}
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		t.Fatal("navigation touched database", err)
	}
	v.SetPointsMode(true)
	v.Start()
	pumpRuntimeView(t, posts, func() bool { return v.Runtime() != nil })
	if !v.Points.SelectPoint("DI11") {
		t.Fatal("real live point missing")
	}
	action.Trigger(nil)
	v.Invalidate()
	action.Trigger(nil) // only the original source document, no stale entity
	v.SetVisible(false)
	action.Trigger(nil) // a queued dispatch must respect a hidden owner
	want := []hmi.SourceTarget{
		{Kind: hmi.SourceNode, ID: "di"}, {Kind: hmi.SourceLink, ID: "flow"},
		{Kind: hmi.SourcePoint, ID: "DI11"}, {},
	}
	if !slices.Equal(events, want) {
		t.Fatalf("source action: got %v want %v", events, want)
	}
}

func TestRuntimeViewAlarmPaneKeepsRuntimeAndSourceIdentity(t *testing.T) {
	v, posts := runtimeViewTest(t, RuntimeViewOptions{EnsureDatabaseDirectory: true})
	v.SetSize(950, 370)
	button := v.buttons[runtimeAlarmsButton]
	if button.IsVisible() {
		t.Fatal("toolbar reserved a slot without alarm rules")
	}
	p := runtimeViewProject()
	p.Points[0].Initial = true
	p.Alarms = []hmi.AlarmRule{{ID: "limit", Point: "DI11", Condition: hmi.AlarmMatch, Priority: hmi.AlarmCritical, Threshold: true, Latch: true}}
	db := filepath.Join(t.TempDir(), "store", "points.db")
	if err := v.SetSnapshot(p, db); err != nil {
		t.Fatal(err)
	}
	button.Action().Trigger(nil)
	if !button.IsEnabled() || !v.Alarms.IsVisible() || v.Points.IsVisible() || v.Diagram.IsVisible() || v.Busy() {
		t.Fatal("alarm mode did not remain an inert exclusive view")
	}
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		t.Fatal("switching to alarm mode opened the database", err)
	}
	for _, width := range []float64{320, 640, 950} {
		v.SetSize(width, 270)
		if v.Alarms.Y()+v.Alarms.Height() > v.status.Y() || button.X()+button.Width() > width || button.Y()+button.Height() > v.Alarms.Y() {
			t.Fatalf("alarm panel bounds at %.0f", width)
		}
	}
	if !pointsql.StoreAvailable() {
		t.Skip("runtime alarm session requires cgo for SQLite")
	}
	v.Start()
	pumpRuntimeView(t, posts, func() bool { return v.Runtime() != nil })
	rt := v.Runtime()
	states := rt.AlarmStates()
	if len(states) != 1 || !states[0].Active || !v.Alarms.SelectAlarm("limit") {
		t.Fatalf("expected active latch bound to the dock: %+v", states)
	}
	var events []hmi.SourceTarget
	v.SetSourceAction(func() { events = append(events, hmi.SourceTarget{}) })
	v.SetSourceTargetAction(func(s hmi.SourceTarget) { events = append(events, s) })
	action := v.buttons[runtimeSourceButton].Action()
	action.Trigger(nil)
	v.SetPointsMode(true)
	if v.Runtime() != rt || v.Alarms.SelectedAlarm() != "" || v.Alarms.IsVisible() || !v.Points.SelectPoint("DI11") {
		t.Fatal("presentation switch restarted acquisition or retained an alarm selection")
	}
	action.Trigger(nil)
	v.SetAlarmMode()
	if v.Runtime() != rt || !v.Alarms.SelectAlarm("limit") || len(rt.AlarmEvents(0)) != 1 {
		t.Fatal("return to alarm tab replaced the live session or its history")
	}
	v.Invalidate()
	action.Trigger(nil)
	if v.Alarms.IsVisible() || button.IsVisible() || v.Alarms.SelectedAlarm() != "" || v.Runtime() != nil || v.CanStart() {
		t.Fatal("invalidated snapshot retained alarm permissions or geometry")
	}
	want := []hmi.SourceTarget{{Kind: hmi.SourceAlarm, ID: "limit"}, {Kind: hmi.SourcePoint, ID: "DI11"}, {}}
	if !slices.Equal(events, want) {
		t.Fatalf("wrong authored target: got %v want %v", events, want)
	}
}
func TestRuntimeViewHTTPManualPersistenceQuietStopAndRestart(t *testing.T) {
	if !pointsql.StoreAvailable() {
		t.Skip("SQLite requires cgo")
	}
	var calls, bad atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.Header.Get("X-View-QA") != "yes" {
			bad.Add(1)
		}
		_, _ = w.Write([]byte(`{"pressure":826}`))
	}))
	defer server.Close()
	var events []*hmi.Runtime
	v, posts := runtimeViewTest(t, RuntimeViewOptions{EnsureDatabaseDirectory: true, RequestHeaders: func(string) http.Header { return http.Header{"X-View-QA": []string{"yes"}} }, RuntimeChanged: func(rt *hmi.Runtime) { events = append(events, rt) }})
	project := runtimeViewProject()
	project.Sources = []hmi.Source{{ID: "qa", URL: server.URL, Method: "POST", Body: `{"plant":"qa"}`, RepeatWrites: true, Enabled: true, IntervalMS: 250, TimeoutMS: 500, Mappings: []point.Mapping{{Name: "AI11", Pointer: "/pressure", Kind: point.Float, Steps: []point.Step{{Expr: "x / 10"}}}}}}
	path := filepath.Join(t.TempDir(), "store", "points.db")
	if err := v.SetSnapshot(project, path); err != nil {
		t.Fatal(err)
	}
	v.Start()
	pumpRuntimeView(t, posts, func() bool {
		rt := v.Runtime()
		if rt == nil {
			return false
		}
		s, _ := rt.PointState("AI11")
		return s.Value.Raw == 82.6 && s.Value.Quality == core.QualityGood
	})
	if bad.Load() != 0 || calls.Load() == 0 || len(events) != 1 || len(v.Points.PointStates()) != 2 {
		t.Fatal("acquisition/ready failed")
	}
	v.Points.Refresh()
	if v.Points.PointStates()[1].Value.Raw != 82.6 {
		t.Fatal("monitor not bound")
	}
	rt := v.Runtime()
	state, _ := rt.PointState("DI11")
	result := make(chan error, 1)
	go func() { _, err := rt.WritePoint(context.Background(), "DI11", state.Token, true); result <- err }()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	v.SetPointsMode(false)
	v.SetPointsMode(true)
	v.Points.Refresh()
	if v.Points.PointStates()[0].Value.Raw != true {
		t.Fatal("mode lost runtime binding")
	}
	v.Stop()
	if v.Runtime() != nil || len(v.Points.PointStates()) != 0 || len(events) != 2 || events[1] != nil {
		t.Fatal("stop did not revoke")
	}
	v.SetPointsMode(false)
	v.SetPointsMode(true)
	if len(v.Points.PointStates()) != 0 {
		t.Fatal("mode rebound stopping runtime")
	}
	pumpRuntimeView(t, posts, func() bool { return !v.Busy() })
	n := calls.Load()
	time.Sleep(300 * time.Millisecond)
	if calls.Load() != n {
		t.Fatal("stopped HTTP still running")
	}
	store, err := pointsql.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	sample, err := store.Latest(context.Background(), "DI11")
	_ = store.Close()
	if err != nil || sample.Value.Raw != true {
		t.Fatal("durable write missing", err)
	}
	v.Start()
	pumpRuntimeView(t, posts, func() bool { return v.Runtime() != nil })
	s, _ := v.Runtime().PointState("DI11")
	if s.Value.Raw != true {
		t.Fatal("restart did not restore")
	}
	container := new(gui.Widget)
	container.Init(container)
	v.SetParent(container)
	container.SetVisible(false)
	if v.Runtime() != nil {
		t.Fatal("ancestor hiding did not revoke")
	}
	pumpRuntimeView(t, posts, func() bool { return !v.Busy() })
}
func TestRuntimeViewLateReadyDispatchFailureAndReentrantStop(t *testing.T) {
	if !pointsql.StoreAvailable() {
		t.Skip("SQLite requires cgo")
	}
	v, posts := runtimeViewTest(t, RuntimeViewOptions{})
	if err := v.SetSnapshot(runtimeViewProject(), filepath.Join(t.TempDir(), "points.db")); err != nil {
		t.Fatal(err)
	}
	v.Start()
	var stale func()
	select {
	case stale = <-posts:
	case <-time.After(5 * time.Second):
		t.Fatal("no ready")
	}
	v.Stop()
	pumpRuntimeView(t, posts, func() bool { return !v.Busy() })
	stale()
	if v.Runtime() != nil || len(v.Points.PointStates()) != 0 {
		t.Fatal("late ready rebound")
	}
	v.postContext = func(context.Context, func()) error { return errors.New("fixture dispatcher unavailable") }
	v.Start()
	pumpRuntimeView(t, posts, func() bool { return !v.Busy() })
	if v.LastError() == nil {
		t.Fatal("lost dispatch error")
	}
	v.postContext = func(ctx context.Context, f func()) error { posts <- f; return nil }
	v.options.RuntimeChanged = func(rt *hmi.Runtime) {
		if rt != nil {
			v.Stop()
		}
	}
	v.Start()
	pumpRuntimeView(t, posts, func() bool { return !v.Busy() })
	if v.Runtime() != nil {
		t.Fatal("reentrant stop lost")
	}
	v.Close()
	v.Start()
	if v.CanStart() {
		t.Fatal("closed view restartable")
	}
	if err := v.Wait(nil); err == nil {
		t.Fatal("nil wait context")
	}
}
