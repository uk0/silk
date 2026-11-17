package hmi_test

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/point"
	pointsql "github.com/uk0/silk/point/sqlite"
)

func livePointProject() hmi.Project {
	return hmi.Project{Version: hmi.Version, Title: "Live point QA", Width: 640, Height: 480, Points: []hmi.Point{
		{Definition: point.Definition{Name: "DI11", Kind: point.Bool, Category: "DI"}, Initial: false},
		{Definition: point.Definition{Name: "AI11", Kind: point.Float, Category: "AI", Unit: "bar"}, Initial: 12.5},
		{Definition: point.Definition{Name: "BI11", Kind: point.Int, Category: "BI"}, Initial: int64(1)},
		{Definition: point.Definition{Name: "TEXT", Kind: point.String}, Initial: "ready"},
	}}
}

type livePointStore struct {
	mu      sync.Mutex
	values  []core.Value
	block   <-chan struct{}
	entered chan struct{}
	err     error
}

func (*livePointStore) Define(context.Context, point.Definition) error { return nil }
func (s *livePointStore) Publish(ctx context.Context, _ string, v core.Value) error {
	if s.entered != nil {
		select {
		case s.entered <- struct{}{}:
		default:
		}
	}
	if s.block != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.block:
		}
	}
	if s.err != nil {
		return s.err
	}
	s.mu.Lock()
	s.values = append(s.values, v)
	s.mu.Unlock()
	return nil
}

func TestRuntimePointSnapshotsConditionalWritesAndABA(t *testing.T) {
	ctx := context.Background()
	store := new(livePointStore)
	rt, err := hmi.NewRuntime(ctx, livePointProject(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop()
	states := rt.PointStates()
	if len(states) != 4 || states[2].Definition.Name != "BI11" || !states[2].Writable || states[2].Revision == 0 {
		t.Fatal("incorrect initial snapshots")
	}
	states[2].Definition.Name = "local"
	states[2].Value.Raw = "local"
	a, _ := rt.PointState("BI11")
	b, err := rt.WritePoint(ctx, "BI11", a.Token, "9223372036854775807")
	if err != nil || b.Value.Raw != int64(math.MaxInt64) || b.Revision <= a.Revision {
		t.Fatal("typed write lost precision/revision", b, err)
	}
	if _, err = rt.WritePoint(ctx, "BI11", a.Token, 2); !errors.Is(err, hmi.ErrPointConflict) {
		t.Fatal("stale token accepted", err)
	}
	other, _ := rt.PointState("DI11")
	if _, err = rt.WritePoint(ctx, "BI11", other.Token, 2); !errors.Is(err, hmi.ErrPointConflict) {
		t.Fatal("cross-point token accepted", err)
	}
	rt2, err := hmi.NewRuntime(ctx, livePointProject(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt2.Stop()
	foreign, _ := rt2.PointState("BI11")
	if _, err = rt.WritePoint(ctx, "BI11", foreign.Token, 2); !errors.Is(err, hmi.ErrPointConflict) {
		t.Fatal("cross-runtime token accepted", err)
	}
	if _, err = rt.WritePoint(nil, "BI11", b.Token, 2); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err = rt.PointState("missing"); !errors.Is(err, hmi.ErrPointNotFound) {
		t.Fatal("unknown point accepted")
	}
	if _, err = rt.WritePoint(ctx, "BI11", b.Token, "not-a-number"); err == nil {
		t.Fatal("invalid value accepted")
	}
	cur, _ := rt.PointState("BI11")
	if cur.Revision != b.Revision || cur.Value.Raw != b.Value.Raw {
		t.Fatal("failed write changed state")
	}
	if err = rt.Publish(ctx, "BI11", core.Value{Raw: int64(2), Quality: core.QualityGood, Time: a.Value.Time}); err != nil {
		t.Fatal(err)
	}
	if err = rt.Publish(ctx, "BI11", a.Value); err != nil {
		t.Fatal(err)
	}
	if _, err = rt.WritePoint(ctx, "BI11", a.Token, 3); !errors.Is(err, hmi.ErrPointConflict) {
		t.Fatal("ABA bypassed token", err)
	}
	cur, _ = rt.PointState("BI11")
	var wg sync.WaitGroup
	var wins, conflicts atomic.Int32
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := rt.WritePoint(ctx, "BI11", cur.Token, i+10)
			if err == nil {
				wins.Add(1)
			} else if errors.Is(err, hmi.ErrPointConflict) {
				conflicts.Add(1)
			} else {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if wins.Load() != 1 || conflicts.Load() != 1 {
		t.Fatal("competing runtime writers both won", wins.Load(), conflicts.Load())
	}
	if rt.SupportsHistory() {
		t.Fatal("non-historian store advertised history")
	}
	if _, err = rt.RecentHistory(ctx, "BI11", 10); !errors.Is(err, hmi.ErrHistoryUnavailable) {
		t.Fatal(err)
	}
}

func TestRuntimePointReadDoesNotWaitForPersistenceAndCancelDoesNotPublish(t *testing.T) {
	release := make(chan struct{})
	store := &livePointStore{block: release, entered: make(chan struct{}, 1)}
	rt, err := hmi.NewRuntime(context.Background(), livePointProject(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop()
	before, _ := rt.PointState("DI11")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := rt.WritePoint(ctx, "DI11", before.Token, true); done <- err }()
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("write did not reach store")
	}
	read := make(chan hmi.PointState, 1)
	go func() { v, _ := rt.PointState("DI11"); read <- v }()
	select {
	case v := <-read:
		if v.Revision != before.Revision || v.Value.Raw != false {
			t.Fatal("published before persistence")
		}
	case <-time.After(time.Second):
		t.Fatal("live read blocked behind store I/O")
	}
	cancel()
	select {
	case err = <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("write did not cancel")
	}
	after, _ := rt.PointState("DI11")
	if after.Token != before.Token || after.Value.Raw != false {
		t.Fatal("canceled write published")
	}
	close(release)
}

func TestRuntimePointObserverHeartbeatStaleAndHTTPOwnership(t *testing.T) {
	for _, initial := range []string{"fresh", "stale-before-heartbeat"} {
		t.Run(initial, func(t *testing.T) {
			testRuntimePointObserverHeartbeat(t, initial == "stale-before-heartbeat")
		})
	}
}

func testRuntimePointObserverHeartbeat(t *testing.T, initiallyStale bool) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	p := livePointProject()
	p.Points[0].StaleMS = 1
	p.Sources = []hmi.Source{{ID: "acquisition", URL: server.URL, Method: "GET", IntervalMS: 1000, TimeoutMS: 100, Enabled: true, Mappings: []point.Mapping{{Name: "AI11", Pointer: "/pressure", Kind: point.Float}}}}
	rt, err := hmi.NewRuntime(context.Background(), p, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop()
	// Construction can already age a 1ms point on a loaded Windows host.
	// Establish an explicit sample/clock before subscribing, instead of assuming
	// NewRuntime finishes within the stale threshold. No sleeps or retries.
	sample := core.Value{Raw: false, Quality: core.QualityGood, Time: time.Unix(1_700_000_000, 0)}
	if err = rt.Publish(context.Background(), "DI11", sample); err != nil {
		t.Fatal(err)
	}
	if initiallyStale {
		rt.Tick(sample.Time.Add(time.Millisecond + time.Nanosecond))
		aged, _ := rt.PointState("DI11")
		if aged.Value.Quality != core.QualityUncertain {
			t.Fatal("initial stale control did not age")
		}
		if err = rt.Publish(context.Background(), "DI11", sample); err != nil {
			t.Fatal(err)
		}
	}
	var observed atomic.Int32
	stop := rt.SubscribePoints(func(s hmi.PointState) {
		if s.Definition.Name == "DI11" {
			observed.Add(1)
			if current, _ := rt.PointState("DI11"); current.Revision < s.Revision {
				t.Error("non-atomic callback state")
			}
		}
	})
	defer stop()
	if observed.Load() != 1 {
		t.Fatal("subscription did not prime")
	}
	before, _ := rt.PointState("DI11")
	if err = rt.Publish(context.Background(), "DI11", before.Value); err != nil {
		t.Fatal(err)
	}
	after, _ := rt.PointState("DI11")
	if observed.Load() != 1 || after.Revision <= before.Revision {
		t.Fatal("heartbeat notified or did not advance token")
	}
	boundary := after.Value.Time.Add(time.Millisecond)
	rt.Tick(boundary)
	if at, _ := rt.PointState("DI11"); at.Value.Quality != core.QualityGood || observed.Load() != 1 {
		t.Fatal("point aged at exact boundary")
	}
	rt.Tick(boundary.Add(time.Nanosecond))
	after, _ = rt.PointState("DI11")
	if after.Value.Quality != core.QualityUncertain || observed.Load() != 2 {
		t.Fatalf("stale quality missing: quality=%v notifications=%d", after.Value.Quality, observed.Load())
	}
	rt.Tick(boundary.Add(time.Second))
	if observed.Load() != 2 {
		t.Fatal("repeated stale tick notified")
	}
	if _, err = rt.WritePoint(context.Background(), "DI11", before.Token, true); !errors.Is(err, hmi.ErrPointConflict) {
		t.Fatal("pre-stale token accepted")
	}
	owned, _ := rt.PointState("AI11")
	if owned.Writable || owned.Writer != "acquisition" {
		t.Fatal("HTTP point not marked read only")
	}
	if _, err = rt.WritePoint(context.Background(), "AI11", owned.Token, 5); !errors.Is(err, hmi.ErrPointReadOnly) {
		t.Fatal("second writer accepted", err)
	}
	stop()
	_, err = rt.WritePoint(context.Background(), "DI11", after.Token, true)
	if err != nil || observed.Load() != 2 {
		t.Fatal("canceled subscription delivered", err)
	}
	if calls.Load() != 0 {
		t.Fatal("snapshot/subscription/write started HTTP")
	}
	rt.Stop()
	stopped, _ := rt.PointState("DI11")
	if stopped.Writable {
		t.Fatal("stopped runtime writable")
	}
	if _, err = rt.WritePoint(context.Background(), "DI11", stopped.Token, false); err == nil {
		t.Fatal("write after stop")
	}
}

func TestRuntimePointSQLiteHistoryOrderExactTypesAndReopen(t *testing.T) {
	if !pointsql.StoreAvailable() {
		t.Skip("SQLite requires cgo")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "points.db")
	store, err := pointsql.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := hmi.NewRuntime(ctx, livePointProject(), store, nil)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	at := time.Now().UTC()
	for _, v := range []int64{math.MaxInt64 - 1, math.MaxInt64} {
		if err = rt.Publish(ctx, "BI11", core.Value{Raw: v, Quality: core.QualityGood, Time: at}); err != nil {
			t.Fatal(err)
		}
	}
	if !rt.SupportsHistory() {
		t.Fatal("SQLite history unavailable")
	}
	recent, err := rt.RecentHistory(ctx, "BI11", 1)
	if err != nil || len(recent) != 1 || recent[0].Value.Raw != int64(math.MaxInt64) {
		t.Fatal("recent precision/order", recent, err)
	}
	history, err := rt.History(ctx, "BI11", at.Add(-time.Second), at.Add(time.Second), 10)
	if err != nil || len(history) != 2 || history[0].Value.Raw != int64(math.MaxInt64-1) {
		t.Fatal("range insertion order", history, err)
	}
	for _, n := range []int{0, 10001} {
		if _, err = rt.RecentHistory(ctx, "BI11", n); err == nil {
			t.Fatal("unbounded query accepted")
		}
	}
	if _, err = rt.RecentHistory(nil, "BI11", 1); err == nil {
		t.Fatal("nil history context")
	}
	if _, err = rt.RecentHistory(ctx, "unknown", 1); !errors.Is(err, hmi.ErrPointNotFound) {
		t.Fatal(err)
	}
	rt.Stop()
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = pointsql.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stored, err := store.Latest(ctx, "BI11")
	if err != nil || stored.Value.Raw != int64(math.MaxInt64) || stored.Value.Quality != core.QualityGood || !stored.Value.Time.Equal(at) {
		t.Fatal("independent reopened state", stored, err)
	}
}

type pointHistoryStore struct {
	livePointStore
	rows []point.SampleRecord
}

func (s *pointHistoryStore) ReadHistory(context.Context, string, time.Time, time.Time, int) ([]point.SampleRecord, error) {
	return s.rows, nil
}
func (s *pointHistoryStore) RecentHistory(context.Context, string, int) ([]point.SampleRecord, error) {
	return s.rows, nil
}

type pointRestoreStore struct {
	livePointStore
	value core.Value
}

func (s *pointRestoreStore) LoadSample(_ context.Context, name string) (core.Value, bool, error) {
	return s.value, name == "BI11", nil
}

func TestRuntimePointHistoryRejectsBrokenReaderAndRestoreContracts(t *testing.T) {
	ctx := context.Background()
	at := time.Now().UTC()
	good := point.SampleRecord{Name: "BI11", Value: core.Value{Raw: int64(1), Quality: core.QualityGood, Time: at}}
	store := &pointHistoryStore{}
	rt, err := hmi.NewRuntime(ctx, livePointProject(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop()
	for _, rows := range [][]point.SampleRecord{
		{good, good},
		{{Name: "DI11", Value: good.Value}},
		{{Name: "BI11", Value: core.Value{Raw: int64(1), Quality: core.Quality(255), Time: at}}},
		{{Name: "BI11", Value: core.Value{Raw: map[string]int{"bad": 1}, Quality: core.QualityGood, Time: at}}},
		{{Name: "BI11", Value: core.Value{Raw: int64(1), Quality: core.QualityGood}}},
	} {
		store.rows = rows
		if _, err = rt.RecentHistory(ctx, "BI11", 1); err == nil {
			t.Fatal("invalid/unbounded store history accepted", rows)
		}
	}
	store.rows = []point.SampleRecord{good, {Name: "BI11", Value: core.Value{Raw: int64(2), Quality: core.QualityGood, Time: at.Add(time.Second)}}}
	if _, err = rt.RecentHistory(ctx, "BI11", 2); err == nil {
		t.Fatal("unordered recent history accepted")
	}
	if _, err = rt.History(ctx, "BI11", at.Add(-time.Second), at, 2); err == nil {
		t.Fatal("history outside requested range accepted")
	}
	for _, sample := range []core.Value{{Raw: map[string]int{"bad": 1}, Quality: core.QualityGood, Time: at}, {Raw: int64(1), Quality: core.QualityGood}, {Raw: int64(1), Quality: core.Quality(255), Time: at}} {
		if _, err = hmi.NewRuntime(ctx, livePointProject(), &pointRestoreStore{value: sample}, nil); err == nil {
			t.Fatal("invalid restored sample accepted", sample)
		}
	}
}
