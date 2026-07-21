// Package scada is the headless runtime service container for a SCADA / 组态
// (HMI) application: it owns ONE of each backend resource — the real-time tag
// registry, the alarm engine, the historian, the event log, the recipe book
// and the live-statistics collector — and wires them together so a screen built
// from the backend-free gui panels has a single, consistent runtime to talk to.
//
// Import direction. scada sits ABOVE gui in the import graph: it may import
// core, gui, device, driver, historian, eventlog, recipe, stats, alarmbridge,
// notify, report and playback, and it feeds the gui panels through their plain
// SetX / SigY seams. gui MUST NOT import scada — the panels stay decoupled and
// unit-testable, driven by data and callbacks rather than by a Services handle.
// The tree-walking wiring that connects a live screen to a Services lives in
// wire.go (BindScreen); this file is the container and its lifecycle.
package scada

import (
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/uk0/silk/alarmbridge"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/eventlog"
	"github.com/uk0/silk/historian"
	"github.com/uk0/silk/notify"
	"github.com/uk0/silk/recipe"
	"github.com/uk0/silk/stats"
)

// Config parameterises New. The three paths locate the on-disk stores; TagMeta
// seeds the tag registry (and its alarm limits) before any store is touched;
// Notifier and OnError are the two side-effect sinks the container routes
// through so the host, not scada, owns notification delivery and error policy.
type Config struct {
	HistorianPath string                         // SQLite file for the sample historian
	EventLogPath  string                         // SQLite file for the event/audit log
	RecipePath    string                         // JSON file for the recipe book (loaded if present)
	TagMeta       map[string]core.Meta           // tags registered up front, with engineering + alarm limits
	Notifier      func(title, body string) error // user-facing notification sink (nil = silent)
	OnError       func(error)                    // best-effort error sink for background failures (nil = drop)
}

// DefaultConfig returns a Config whose stores live under dir and whose Notifier
// is the native desktop notifier. A headless caller (tests) overrides Notifier
// with its own sink; notify.Notify reaches the OS and needs a desktop session.
func DefaultConfig(dir string) Config {
	return Config{
		HistorianPath: filepath.Join(dir, "history.db"),
		EventLogPath:  filepath.Join(dir, "events.db"),
		RecipePath:    filepath.Join(dir, "recipes.json"),
		Notifier:      func(title, body string) error { return notify.Notify(title, body) },
	}
}

// StartupHook is a user-registered wiring step run by Start after the built-in
// wiring (alarm bridge, historian recording, stat tracking). It returns a
// cancel func kept in the container's cleanup list, or an error that aborts and
// rolls back Start. The Services is passed so a hook can reach the shared stores.
type StartupHook func(s *Services) (cancel func(), err error)

// Services owns one of each backend resource and the teardown list that unwinds
// everything Start installed. The five public store fields are the single shared
// instances the wiring (wire.go) and the host bind against.
type Services struct {
	Tags    *core.TagDB          // the one real-time tag registry
	Alarms  *core.AlarmDB        // the one limit-alarm engine
	Hist    *historian.Historian // the one sample historian
	Events  *eventlog.Log        // the one event/audit log
	Recipes *recipe.Book         // the one recipe book (in memory; Save persists it)
	Stats   *stats.Collector     // the one live-statistics collector

	cfg          Config
	startupHooks []StartupHook

	mu      sync.Mutex
	started bool
	stopped bool
	cleanup []func() // teardown steps, run in reverse by Stop
}

// New allocates the container: it creates the single TagDB + AlarmDB, registers
// cfg.TagMeta into the registry BEFORE opening any store (so alarm limits and
// engineering ranges are in place first), then opens the historian, event log
// and recipe book. If a later open fails, the resources already opened are
// rolled back (closed) before New returns the error, so a failed New leaks
// nothing. Start is NOT called here — the container is inert until Start.
func New(cfg Config) (*Services, error) {
	s := &Services{
		cfg:    cfg,
		Tags:   core.NewTagDB(),
		Alarms: core.NewAlarmDB(),
	}

	// Register the declared tags (and their meta) first: everything downstream —
	// alarm evaluation, historian recording, stat priming — reads this metadata.
	for name, meta := range cfg.TagMeta {
		s.Tags.GetOrCreate(name, meta)
	}

	hist, err := historian.NewHistorian(cfg.HistorianPath)
	if err != nil {
		return nil, err
	}
	s.Hist = hist

	events, err := eventlog.NewLog(cfg.EventLogPath)
	if err != nil {
		hist.Close() // roll back the historian opened just above
		return nil, err
	}
	s.Events = events

	book, err := loadRecipes(cfg.RecipePath)
	if err != nil {
		events.Close() // roll back in reverse: event log then historian
		hist.Close()
		return nil, err
	}
	s.Recipes = book

	s.Stats = stats.NewCollector(s.Tags)
	return s, nil
}

// loadRecipes returns the recipe book at path, an empty book when path is unset
// or the file does not yet exist, or an error when the file is present but
// unreadable / corrupt (which aborts New).
func loadRecipes(path string) (*recipe.Book, error) {
	if path == "" {
		return &recipe.Book{}, nil
	}
	book, err := recipe.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &recipe.Book{}, nil
		}
		return nil, err
	}
	return book, nil
}

// AddStartupHook registers h to run during Start. Call before Start; hooks
// registered after Start are ignored until a (currently unsupported) restart.
func (s *Services) AddStartupHook(h StartupHook) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startupHooks = append(s.startupHooks, h)
}

// Start installs the live runtime wiring and records every teardown in the
// cleanup list: the alarm bridge (alarm transition -> event + notification), an
// AlarmDB watch on each registered tag (so a limit violation raises), historian
// recording and stat tracking for the registered tags, then any registered
// startup hooks. It is idempotent — a second call is a no-op. A hook error rolls
// back everything installed so far and returns the error.
func (s *Services) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}

	// Alarm bridge: every raised alarm becomes an audit event and a notification.
	s.cleanup = append(s.cleanup, alarmbridge.Watch(s.Alarms, s.recordAlarmEvent, s.notify))

	// Evaluate each registered tag against its own limits, so a value crossing a
	// limit drives an AlarmDB transition the bridge above then reacts to.
	names := s.metaNames()
	for _, name := range names {
		if tag, ok := s.Tags.Get(name); ok {
			s.cleanup = append(s.cleanup, s.Alarms.Watch(tag))
		}
	}

	// Persist history and fold running statistics for the registered tags.
	s.cleanup = append(s.cleanup, s.Hist.Record(s.Tags, names))
	for _, name := range names {
		s.Stats.Track(name)
	}
	s.cleanup = append(s.cleanup, s.Stats.StopAll)

	// Host-registered startup hooks last; a failure unwinds the whole Start.
	for _, h := range s.startupHooks {
		cancel, err := h(s)
		if err != nil {
			s.runCleanupLocked()
			return err
		}
		if cancel != nil {
			s.cleanup = append(s.cleanup, cancel)
		}
	}

	s.started = true
	return nil
}

// Stop tears the container down: it runs the cleanup list in reverse, then
// closes the event log and the historian last (in that order). It is idempotent
// — a second call is a no-op. Close errors are routed to cfg.OnError.
func (s *Services) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true

	s.runCleanupLocked()

	if s.Events != nil {
		if err := s.Events.Close(); err != nil {
			s.reportError(err)
		}
	}
	if s.Hist != nil {
		if err := s.Hist.Close(); err != nil {
			s.reportError(err)
		}
	}
}

// runCleanupLocked runs every registered teardown in reverse registration order
// and empties the list. The caller must hold s.mu.
func (s *Services) runCleanupLocked() {
	for i := len(s.cleanup) - 1; i >= 0; i-- {
		if fn := s.cleanup[i]; fn != nil {
			fn()
		}
	}
	s.cleanup = nil
}

// RecordEvent appends one event to the event log, routing any write error to
// cfg.OnError. It is the container's single event-emission point and is safe to
// call from any goroutine (the event log serialises writes internally).
func (s *Services) RecordEvent(kind eventlog.Kind, source, msg string) {
	if s.Events == nil {
		return
	}
	if err := s.Events.Record(kind, source, msg); err != nil {
		s.reportError(err)
	}
}

// recordAlarmEvent is the alarm bridge's event sink: it books a raised alarm as
// a KindAlarm event.
func (s *Services) recordAlarmEvent(source, msg string) {
	s.RecordEvent(eventlog.KindAlarm, source, msg)
}

// notify is the alarm bridge's notification sink: it forwards to cfg.Notifier
// (when set) and routes a delivery error to cfg.OnError.
func (s *Services) notify(title, body string) {
	if s.cfg.Notifier == nil {
		return
	}
	if err := s.cfg.Notifier(title, body); err != nil {
		s.reportError(err)
	}
}

// reportError hands err to cfg.OnError when one is configured; otherwise the
// error is dropped (background side effects have no caller to return to).
func (s *Services) reportError(err error) {
	if s.cfg.OnError != nil {
		s.cfg.OnError(err)
	}
}

// metaNames returns the registered tag names in deterministic (sorted) order so
// historian recording and stat tracking are stable across runs.
func (s *Services) metaNames() []string {
	names := make([]string, 0, len(s.cfg.TagMeta))
	for name := range s.cfg.TagMeta {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
