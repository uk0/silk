package gui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/point"
)

type pointMonitorStore struct {
	mu             sync.Mutex
	values         []core.Value
	recent         atomic.Int32
	block          bool
	entered        chan struct{}
	cancelled      chan struct{}
	writeEntered   chan struct{}
	writeCancelled chan struct{}
}

func (*pointMonitorStore) Define(context.Context, point.Definition) error { return nil }
func (s *pointMonitorStore) Publish(ctx context.Context, _ string, v core.Value) error {
	if s.writeEntered != nil {
		close(s.writeEntered)
		<-ctx.Done()
		close(s.writeCancelled)
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.values = append(s.values, v)
	s.mu.Unlock()
	return nil
}
func (s *pointMonitorStore) ReadHistory(context.Context, string, time.Time, time.Time, int) ([]point.SampleRecord, error) {
	return nil, errors.New("range query was not requested")
}
func (s *pointMonitorStore) RecentHistory(ctx context.Context, name string, limit int) ([]point.SampleRecord, error) {
	s.recent.Add(1)
	if limit != 200 {
		return nil, fmt.Errorf("unbounded UI query %d", limit)
	}
	if s.block {
		close(s.entered)
		<-ctx.Done()
		close(s.cancelled)
		return nil, ctx.Err()
	}
	return []point.SampleRecord{{Name: name, Value: core.Value{Raw: int64(math.MaxInt64), Quality: core.QualityGood, Time: time.Now()}}, {Name: name, Value: core.Value{Quality: core.QualityBad, Time: time.Now().Add(-time.Second)}}}, nil
}
func monitorProject() hmi.Project {
	return hmi.Project{Version: hmi.Version, Title: "Monitor QA", Width: 640, Height: 480, Points: []hmi.Point{
		{Definition: point.Definition{Name: "DI11", Kind: point.Bool, Category: "DI"}, Initial: false},
		{Definition: point.Definition{Name: "BI11", Kind: point.Int, Category: "BI"}, Initial: int64(math.MaxInt64 - 1)},
		{Definition: point.Definition{Name: "BI12", Kind: point.Int, Category: "BI"}, Initial: int64(math.MaxInt64)},
	}}
}
func newMonitorTest(t *testing.T, store hmi.Store) (*ProcessPointMonitor, *hmi.Runtime, chan func()) {
	t.Helper()
	rt, err := hmi.NewRuntime(context.Background(), monitorProject(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	v := NewProcessPointMonitor()
	posts := make(chan func(), 64)
	v.post = func(f func()) { posts <- f }
	v.SetRuntime(rt)
	t.Cleanup(func() {
		v.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := v.Wait(ctx); err != nil {
			t.Error(err)
		}
		rt.Stop()
		drainPointPosts(posts)
	})
	return v, rt, posts
}
func drainPointPosts(posts chan func()) {
	for {
		select {
		case f := <-posts:
			f()
		default:
			return
		}
	}
}
func pumpPointPosts(t *testing.T, posts chan func(), done func() bool) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !done() {
		select {
		case f := <-posts:
			f()
		case <-deadline.C:
			t.Fatal("point UI completion did not arrive")
		}
	}
}

func TestProcessPointMonitorCoalescesCurrentSamplesAndRetiresOldConnections(t *testing.T) {
	v, rt, posts := newMonitorTest(t, nil)
	if len(posts) != 1 {
		t.Fatal("point priming queued one message per point", len(posts))
	}
	drainPointPosts(posts)
	for i := 0; i < 1000; i++ {
		if err := rt.Publish(context.Background(), "BI11", core.Value{Raw: int64(i), Quality: core.QualityGood, Time: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	if len(posts) != 1 {
		t.Fatal("unbounded UI posts", len(posts))
	}
	old := v.connection
	drainPointPosts(posts)
	if v.model.byName["BI11"].state.Value.Raw != int64(999) {
		t.Fatal("burst did not deliver latest")
	}
	before, _ := rt.PointState("BI11")
	if err := rt.Publish(context.Background(), "BI11", before.Value); err != nil {
		t.Fatal(err)
	}
	if len(posts) != 0 {
		t.Fatal("unchanged observation woke UI")
	}
	v.Refresh()
	if v.model.byName["BI11"].state.Revision <= before.Revision {
		t.Fatal("explicit refresh lost heartbeat")
	}
	queuedState, _ := rt.PointState("BI11")
	queuedState.Revision = 1
	queuedState.Value.Raw = int64(-1)
	old.publish(queuedState)
	drainPointPosts(posts)
	if v.model.byName["BI11"].state.Value.Raw != int64(999) {
		t.Fatal("old sample rolled back live row")
	}
	old.publish(before)
	v.SetRuntime(nil)
	drainPointPosts(posts)
	if len(v.PointStates()) != 0 || v.writeButton.IsEnabled() || v.historyButton.IsEnabled() {
		t.Fatal("detached view retained live state")
	}
	old.publish(before)
	if len(posts) != 0 {
		t.Fatal("retired connection posted")
	}
	v.SetRuntime(rt)
	drainPointPosts(posts)
	if v.model.byName["BI11"].state.Value.Raw != int64(999) {
		t.Fatal("rebind did not prime current state")
	}
	v.Close()
	v.SetRuntime(rt)
	drainPointPosts(posts)
	if v.connection != nil || len(v.PointStates()) != 0 {
		t.Fatal("closed view rebound")
	}
}

func TestProcessPointMonitorSortingUsesExactIntegersAndKeepsSelection(t *testing.T) {
	v, rt, posts := newMonitorTest(t, nil)
	drainPointPosts(posts)
	v.SelectPoint("BI12")
	v.table.sortByColumn(3)
	if v.SelectedPoint() != "BI12" {
		t.Fatal("sort retargeted selection")
	}
	i11, i12 := -1, -1
	for i, r := range v.model.rows {
		if r.state.Definition.Name == "BI11" {
			i11 = i
		}
		if r.state.Definition.Name == "BI12" {
			i12 = i
		}
	}
	if i11 >= i12 {
		t.Fatal("integer values were rounded through float64 during sorting")
	}
	if err := rt.Publish(context.Background(), "BI12", core.Value{Raw: int64(-1), Quality: core.QualityGood, Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
	drainPointPosts(posts)
	if v.SelectedPoint() != "BI12" || v.model.rows[v.table.SelectedRow()].state.Definition.Name != "BI12" {
		t.Fatal("live sort lost selected identity")
	}
	v.setError(errors.New("sticky action failure"))
	rt.Publish(context.Background(), "BI12", core.Value{Raw: int64(2), Quality: core.QualityGood, Time: time.Now()})
	drainPointPosts(posts)
	if v.LastError() == nil {
		t.Fatal("background sample hid an action failure")
	}
}

func TestProcessPointWriteDialogCancelValidationSuccessAndStaleBaseline(t *testing.T) {
	oldShow := showProcessPointDialog
	t.Cleanup(func() { showProcessPointDialog = oldShow })
	store := new(pointMonitorStore)
	v, rt, posts := newMonitorTest(t, store)
	drainPointPosts(posts)
	v.SelectPoint("DI11")
	showProcessPointDialog = func(d *Dialog) DialogResult { return DialogCancel }
	v.ShowWrite()
	if len(store.values) != 0 {
		t.Fatal("cancel performed I/O")
	}
	showProcessPointDialog = func(d *Dialog) DialogResult {
		body := d.Content().(*processPointDialogBody)
		body.input.SetText("not-bool")
		d.buttons[1].Action().Trigger(nil)
		if d.closed || !strings.Contains(body.status.Text(), "bool") {
			t.Fatal("invalid value closed dialog or hid rejection", body.status.Text())
		}
		return DialogCancel
	}
	v.ShowWrite()
	if len(store.values) != 0 {
		t.Fatal("invalid value wrote")
	}
	showProcessPointDialog = func(d *Dialog) DialogResult {
		body := d.Content().(*processPointDialogBody)
		body.input.SetText("true")
		d.buttons[1].Action().Trigger(nil)
		pumpPointPosts(t, posts, func() bool { return d.closed })
		return DialogOK
	}
	v.ShowWrite()
	state, _ := rt.PointState("DI11")
	if state.Value.Raw != true || len(store.values) != 1 {
		t.Fatal("confirmed typed write missing", state)
	}
	showProcessPointDialog = func(d *Dialog) DialogResult {
		body := d.Content().(*processPointDialogBody)
		if err := rt.Publish(context.Background(), "DI11", core.Value{Raw: false, Quality: core.QualityGood, Time: time.Now()}); err != nil {
			t.Fatal(err)
		}
		body.input.SetText("true")
		d.buttons[1].Action().Trigger(nil)
		pumpPointPosts(t, posts, func() bool { return strings.Contains(body.status.Text(), "Write failed") })
		if d.closed || !errors.Is(v.LastError(), hmi.ErrPointConflict) {
			t.Fatal("stale write accepted or failure hidden")
		}
		d.buttons[1].Action().Trigger(nil)
		return DialogCancel
	}
	v.ShowWrite()
	if len(store.values) != 2 {
		t.Fatal("failed write retried or changed persistence", len(store.values))
	}
}

func TestProcessPointHistoryIsExplicitBoundedAndCancelledWithOwner(t *testing.T) {
	oldShow := showProcessPointDialog
	t.Cleanup(func() { showProcessPointDialog = oldShow })
	store := new(pointMonitorStore)
	v, _, posts := newMonitorTest(t, store)
	drainPointPosts(posts)
	v.SelectPoint("BI11")
	if store.recent.Load() != 0 {
		t.Fatal("binding queried storage")
	}
	showProcessPointDialog = func(d *Dialog) DialogResult {
		body := d.Content().(*processPointDialogBody)
		pumpPointPosts(t, posts, func() bool { return body.table.Model().RowCount() == 2 })
		if body.table.Model().CellText(0, 1) != "9223372036854775807" || body.table.Model().CellText(1, 1) != "—" {
			t.Fatal("history lost exact type/quality semantics")
		}
		return DialogCancel
	}
	v.ShowHistory()
	if store.recent.Load() != 1 {
		t.Fatal("history not explicit")
	}
	store.block = true
	store.entered = make(chan struct{})
	store.cancelled = make(chan struct{})
	showProcessPointDialog = func(d *Dialog) DialogResult {
		select {
		case <-store.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("history worker did not enter")
		}
		v.SetRuntime(nil)
		if !d.closed {
			t.Fatal("runtime detach left floating history dialog")
		}
		select {
		case <-store.cancelled:
		case <-time.After(5 * time.Second):
			t.Fatal("history did not cancel")
		}
		return DialogCancel
	}
	v.ShowHistory()
	drainPointPosts(posts)
	if v.dialog != nil || v.closeDialog != nil || len(v.PointStates()) != 0 {
		t.Fatal("closed modal resurrected view")
	}
}

func TestProcessPointWriteDetachesAndCancelsWithoutLateDialogMutation(t *testing.T) {
	oldShow := showProcessPointDialog
	t.Cleanup(func() { showProcessPointDialog = oldShow })
	store := &pointMonitorStore{writeEntered: make(chan struct{}), writeCancelled: make(chan struct{})}
	v, rt, posts := newMonitorTest(t, store)
	drainPointPosts(posts)
	v.SelectPoint("DI11")
	showProcessPointDialog = func(d *Dialog) DialogResult {
		body := d.Content().(*processPointDialogBody)
		body.input.SetText("true")
		d.buttons[1].Action().Trigger(nil)
		select {
		case <-store.writeEntered:
		case <-time.After(5 * time.Second):
			t.Fatal("write worker did not enter")
		}
		v.SetRuntime(nil)
		if !d.closed {
			t.Fatal("detached write dialog floats without its runtime")
		}
		select {
		case <-store.writeCancelled:
		case <-time.After(5 * time.Second):
			t.Fatal("detached write did not cancel")
		}
		return DialogCancel
	}
	v.ShowWrite()
	drainPointPosts(posts)
	state, _ := rt.PointState("DI11")
	if state.Value.Raw != false || len(store.values) != 0 || v.connection != nil || v.dialog != nil {
		t.Fatal("cancelled write published or resurrected dialog")
	}
}

func TestProcessPointMonitorLongStringsAndCompositePainting(t *testing.T) {
	x := float64(826)
	if processPointDisplay(x*0.1) != "82.6" {
		t.Fatal("float display exposes scaling noise")
	}
	text := strings.Repeat("世界\n", 200000)
	display := processPointDisplay(text)
	if utf8.RuneCountInString(display) > 161 || !utf8.ValidString(display) || strings.Contains(display, "\n") {
		t.Fatal("unbounded/malformed display")
	}
	if _, err := processPointInlineText(hmi.PointState{Value: core.Value{Raw: text}}); err == nil {
		t.Fatal("oversized inline field silently truncated")
	}
	oldMode := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(oldMode) })
	for _, mode := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(mode)
		v := NewProcessPointMonitor()
		v.SetSize(96, 80)
		pm := paint.NewPixmap(104, 88)
		g := pm.NewPainter()
		v.Draw(g)
		paint.ClosePainter(g)
		im, err := pm.Image()
		if err != nil {
			t.Fatal(err)
		}
		want := Theme().FormColor
		r, green, b, a := im.At(48, 40).RGBA()
		if uint8(r>>8) != want.R || uint8(green>>8) != want.G || uint8(b>>8) != want.B || uint8(a>>8) != want.A {
			t.Fatal("composite leaks placeholder")
		}
		_, _, _, outside := im.At(100, 84).RGBA()
		if outside != 0 {
			t.Fatal("composite escapes bounds")
		}
		paint.CloseSurface(pm)
		v.Close()
	}
	for _, size := range [][2]float64{{640, 360}, {320, 200}, {96, 80}} {
		v := NewProcessPointMonitor()
		v.SetSize(size[0], size[1])
		if v.table.Y() < v.refreshButton.Y()+v.refreshButton.Height() || v.status.Y() < v.table.Y()+v.table.Height() {
			t.Fatal("monitor layout overlaps")
		}
		v.Close()
	}
}

func TestProcessEditorLiveModeDoesNotWasteViewportOnDisabledTools(t *testing.T) {
	v := NewProcessEditor()
	defer v.Close()
	v.SetSize(640, 280)
	v.SetRunning(true)
	if v.Diagram.Y() != 0 || v.Diagram.Height() < 200 {
		t.Fatal("live viewport covered by disabled authoring toolbar")
	}
	for _, b := range v.buttons {
		if b.IsVisible() || b.IsEnabled() {
			t.Fatal("live authoring button remains visible/enabled")
		}
	}
	v.SetRunning(false)
	if v.Diagram.Y() <= 0 {
		t.Fatal("authoring toolbar layout not restored")
	}
}

func BenchmarkProcessPointLargeValueDisplay(b *testing.B) {
	text := strings.Repeat("x", 1<<20)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(processPointDisplay(text)) > 643 {
			b.Fatal("display not bounded")
		}
	}
}
