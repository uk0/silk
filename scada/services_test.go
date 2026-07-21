package scada

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/eventlog"
	"github.com/uk0/silk/gui"
)

// TestMain neutralises the gui Post wakeup hook. gui's init installs
// glfw.PostEmptyEvent so off-thread Post callers can wake the event loop; firing
// it from a headless test traps, so nil it — gui.Post then just enqueues, which
// is all these GL-free tests need (mirroring the gui package's own headless tests).
func TestMain(m *testing.M) {
	gui.SetUIWakeup(nil)
	os.Exit(m.Run())
}

// newTestServices builds a started container with headless stores under a temp
// dir and a no-op Notifier (never reaching the OS). Stop is registered as a
// cleanup and is idempotent, so a test may also call it explicitly.
func newTestServices(t *testing.T) *Services {
	t.Helper()
	cfg := DefaultConfig(t.TempDir())
	cfg.Notifier = func(string, string) error { return nil }
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

// TestNewAllocatesOneTagDB checks New wires a single tag registry shared by the
// collector, registers cfg.TagMeta before anything else, and opens every store.
func TestNewAllocatesOneTagDB(t *testing.T) {
	cfg := DefaultConfig(t.TempDir())
	cfg.Notifier = func(string, string) error { return nil }
	cfg.TagMeta = map[string]core.Meta{"TIC-101": {Hi: 100}}

	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(s.Stop)

	for name, got := range map[string]any{
		"Tags": s.Tags, "Alarms": s.Alarms, "Hist": s.Hist,
		"Events": s.Events, "Recipes": s.Recipes, "Stats": s.Stats,
	} {
		if got == nil {
			t.Fatalf("New left %s nil", name)
		}
	}
	if s.Tags == core.Default {
		t.Fatal("New must allocate its own TagDB, not the global Default")
	}

	// Meta registered up front.
	tag, ok := s.Tags.Get("TIC-101")
	if !ok {
		t.Fatal("TagMeta tag TIC-101 not registered")
	}
	if tag.Meta().Hi != 100 {
		t.Fatalf("TIC-101 Hi = %v, want 100", tag.Meta().Hi)
	}

	// Identity: the collector reads the SAME registry New allocated.
	s.Tags.GetOrCreate("x", core.Meta{}).SetValue(5.0)
	s.Stats.Track("x")
	if st, ok := s.Stats.Get("x"); !ok || st.Last != 5 {
		t.Fatalf("collector saw %+v ok=%v; Stats and Tags are not the same registry", st, ok)
	}
}

// TestStartWiresAlarmBridge raises a limit alarm and asserts the bridge Start
// installs both books a KindAlarm event and fires the injected Notifier.
func TestStartWiresAlarmBridge(t *testing.T) {
	cfg := DefaultConfig(t.TempDir())
	cfg.TagMeta = map[string]core.Meta{"TIC-101": {Hi: 100}}

	var notifs int
	var lastTitle string
	cfg.Notifier = func(title, body string) error {
		notifs++
		lastTitle = title
		return nil
	}

	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(s.Stop)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Drive TIC-101 past its Hi limit. The tag -> alarm -> bridge -> event/notify
	// chain is synchronous on this goroutine, so both side effects are done when
	// SetValue returns.
	tag, ok := s.Tags.Get("TIC-101")
	if !ok {
		t.Fatal("TIC-101 missing")
	}
	tag.SetValue(150.0)

	if notifs != 1 {
		t.Fatalf("Notifier fired %d times, want 1", notifs)
	}
	if !strings.Contains(lastTitle, "TIC-101") {
		t.Fatalf("notification title %q missing tag", lastTitle)
	}

	events, err := s.Events.Query(time.Now().Add(-time.Hour), time.Now().Add(time.Hour), eventlog.KindAlarm)
	if err != nil {
		t.Fatalf("event query: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Source == "TIC-101" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no KindAlarm event booked for TIC-101; got %d events", len(events))
	}
}

// TestStopIdempotentAndClosesStores checks Stop can run twice and that it closes
// both SQLite stores.
func TestStopIdempotentAndClosesStores(t *testing.T) {
	s := newTestServices(t)

	s.Stop()
	s.Stop() // second call must be a no-op, not a panic

	if err := s.Events.Record(eventlog.KindSystem, "x", "y"); err == nil {
		t.Fatal("event log still writable after Stop")
	}
	if _, err := s.Hist.Query("t", time.Time{}, time.Now()); err == nil {
		t.Fatal("historian still queryable after Stop")
	}
}

// TestNewRollsBackOnPartialFailure points the event log at a directory so its
// open fails after the historian has already opened, and asserts New returns the
// error with no container (the historian must have been rolled back).
func TestNewRollsBackOnPartialFailure(t *testing.T) {
	dir := t.TempDir()
	histPath := filepath.Join(dir, "history.db")
	badEvents := filepath.Join(dir, "events-as-dir")
	if err := os.Mkdir(badEvents, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	s, err := New(Config{HistorianPath: histPath, EventLogPath: badEvents})
	if err == nil {
		t.Fatal("New succeeded with an unopenable event log path")
	}
	if s != nil {
		t.Fatalf("New returned a container on failure: %+v", s)
	}
	// The historian opened (its file exists) before the event log failed, so the
	// rollback branch ran to close it.
	if _, statErr := os.Stat(histPath); statErr != nil {
		t.Fatalf("historian file missing; open never reached the rollback path: %v", statErr)
	}
}

// TestStartHookFailureRollsBack registers a passing hook then a failing one and
// asserts Start returns the error, runs the whole cleanup list (the passing
// hook's cancel included), and stays not-started.
func TestStartHookFailureRollsBack(t *testing.T) {
	cfg := DefaultConfig(t.TempDir())
	cfg.Notifier = func(string, string) error { return nil }
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(s.Stop)

	var canceled bool
	s.AddStartupHook(func(*Services) (func(), error) {
		return func() { canceled = true }, nil
	})
	boom := errors.New("boom")
	s.AddStartupHook(func(*Services) (func(), error) {
		return nil, boom
	})

	if err := s.Start(); !errors.Is(err, boom) {
		t.Fatalf("Start error = %v, want boom", err)
	}
	if !canceled {
		t.Fatal("passing hook's cancel was not run during rollback")
	}
	if s.started {
		t.Fatal("Start marked started despite a hook failure")
	}
	if len(s.cleanup) != 0 {
		t.Fatalf("cleanup not emptied after rollback: %d entries", len(s.cleanup))
	}
}
