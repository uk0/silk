package fswatch

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EventKind enumerates the change types a Watcher detects. Mirrors
// QFileSystemWatcher's two-event split (fileChanged / directoryChanged)
// expanded into Created/Modified/Removed for finer-grain handling —
// most consumers want to know whether a path appeared or vanished.
type EventKind int

const (
	// Created fires when a path inside a watched directory appears.
	// Never fires for directly-watched files (you can't observe
	// "creation" of a file you're already watching).
	Created EventKind = iota + 1

	// Modified fires when a watched file's mtime or size changes,
	// or when a child of a watched directory is rewritten.
	Modified

	// Removed fires when a watched path or a directory child
	// disappears. After a directly-watched path emits Removed, the
	// Watcher continues polling so a re-creation can be reported as
	// Modified (or re-emit Removed if it stays gone).
	Removed
)

// String aids debug + log output.
func (k EventKind) String() string {
	switch k {
	case Created:
		return "Created"
	case Modified:
		return "Modified"
	case Removed:
		return "Removed"
	}
	return "<unknown>"
}

// Event is a single filesystem-change report. Path is the absolute
// path of the changed entry; ModTime is the filesystem-reported
// modification time at observation (zero on Removed). Kind tells the
// consumer what happened.
type Event struct {
	Path    string
	Kind    EventKind
	ModTime time.Time
}

// Watcher is the central object. Construct with New, then AddPath
// to register watch targets. The polling goroutine starts on the
// first AddPath; Stop terminates it and closes the Events channel.
//
// Concurrency: AddPath / RemovePath / Stop are safe to call from
// any goroutine. Events is read-only; the producer goroutine is
// the polling loop.
type Watcher struct {
	mu sync.Mutex

	// pollInterval is how often the watcher snapshots watched paths
	// and emits diffs. Default 500ms; override before AddPath via
	// SetPollInterval.
	pollInterval time.Duration

	// snapshots stores the last-seen state for every AddPath'd path.
	// Files: map entry holds mtime + size; directories: nested map
	// of children keyed by name.
	snapshots map[string]*snapshot

	// events is the public channel. Buffered to avoid blocking the
	// poller on a slow consumer; see package docs.
	events chan Event

	// running flags whether the polling goroutine is active. Set on
	// first AddPath; cleared by Stop.
	running bool

	// stop signals the polling goroutine to exit. Closed by Stop.
	stop chan struct{}

	// wg tracks the polling goroutine for orderly shutdown. Stop
	// waits on wg so callers see "polling actually halted" semantics
	// when Stop returns.
	wg sync.WaitGroup
}

// snapshot is the last-observed state for a single watched path.
// IsDir true means children is the snapshot of immediate children
// (one level — recursive watch is out of scope).
type snapshot struct {
	exists   bool
	isDir    bool
	modTime  time.Time
	size     int64
	children map[string]childInfo
}

// childInfo is a single directory-child entry tracked across polls.
// We keep mtime so re-writes of an existing file emit Modified.
type childInfo struct {
	modTime time.Time
	size    int64
}

// New creates a stopped Watcher with the default 500ms poll interval
// and a 64-slot event buffer. AddPath starts the polling loop on the
// first call.
func New() *Watcher {
	return &Watcher{
		pollInterval: 500 * time.Millisecond,
		snapshots:    make(map[string]*snapshot),
		events:       make(chan Event, 64),
		stop:         make(chan struct{}),
	}
}

// SetPollInterval overrides the default 500ms cadence. Call before
// AddPath; the goroutine reads pollInterval at start and changes
// don't apply mid-run. Minimum 50ms — sub-50ms intervals churn the
// scheduler without measurable gain.
func (w *Watcher) SetPollInterval(d time.Duration) {
	if d < 50*time.Millisecond {
		d = 50 * time.Millisecond
	}
	w.mu.Lock()
	w.pollInterval = d
	w.mu.Unlock()
}

// PollInterval returns the current polling cadence.
func (w *Watcher) PollInterval() time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pollInterval
}

// Events returns the read-only channel that the polling goroutine
// publishes on. Closed when Stop returns.
func (w *Watcher) Events() <-chan Event { return w.events }

// AddPath registers path (file or directory). Errors from Stat are
// returned to the caller; the path is added regardless and a Removed
// event will fire if it later appears and disappears.
//
// Adding the same path twice is a no-op — duplicate watches don't
// produce duplicate events.
func (w *Watcher) AddPath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	w.mu.Lock()
	if _, ok := w.snapshots[abs]; ok {
		w.mu.Unlock()
		return nil
	}
	snap := w.takeSnapshot(abs)
	w.snapshots[abs] = snap
	first := !w.running
	if first {
		w.running = true
	}
	w.mu.Unlock()
	if first {
		w.wg.Add(1)
		go w.poll()
	}
	return nil
}

// RemovePath drops the path from the watch list. Does not retroactively
// emit any events. No-op when path was never added.
func (w *Watcher) RemovePath(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	w.mu.Lock()
	delete(w.snapshots, abs)
	w.mu.Unlock()
}

// Paths returns the currently-watched absolute paths. Order is map-
// iteration (unstable) — callers needing a stable list should sort.
func (w *Watcher) Paths() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.snapshots))
	for p := range w.snapshots {
		out = append(out, p)
	}
	return out
}

// Stop terminates the polling goroutine and closes Events. Safe to
// call multiple times — the second call is a no-op. Returns when
// the goroutine has actually exited.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	close(w.stop)
	w.mu.Unlock()
	w.wg.Wait()
	close(w.events)
}

// poll is the polling goroutine. Wakes on pollInterval, snapshots
// every watched path, diffs against the previous snapshot, emits
// events for any change.
func (w *Watcher) poll() {
	defer w.wg.Done()
	w.mu.Lock()
	interval := w.pollInterval
	w.mu.Unlock()

	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-w.stop:
			return
		case <-t.C:
			w.tick()
		}
	}
}

// tick takes a fresh snapshot of every watched path and emits diff
// events. Runs under mu so concurrent AddPath/RemovePath sees a
// consistent state.
func (w *Watcher) tick() {
	w.mu.Lock()
	// Copy the current path list before iterating so AddPath /
	// RemovePath called from inside an Event handler doesn't perturb
	// the loop. Snapshot pointers are mutated in place after the
	// iteration based on the diff results computed below.
	paths := make([]string, 0, len(w.snapshots))
	for p := range w.snapshots {
		paths = append(paths, p)
	}
	w.mu.Unlock()

	for _, path := range paths {
		w.mu.Lock()
		prev, ok := w.snapshots[path]
		if !ok {
			// Removed mid-tick.
			w.mu.Unlock()
			continue
		}
		w.mu.Unlock()

		curr := w.takeSnapshot(path)
		w.diffAndEmit(path, prev, curr)

		w.mu.Lock()
		w.snapshots[path] = curr
		w.mu.Unlock()
	}
}

// takeSnapshot reads the current filesystem state for path. Missing
// path → snapshot{exists: false}. File → mtime + size. Directory →
// child name → mtime + size map.
func (w *Watcher) takeSnapshot(path string) *snapshot {
	info, err := os.Stat(path)
	if err != nil {
		return &snapshot{exists: false}
	}
	s := &snapshot{
		exists:  true,
		isDir:   info.IsDir(),
		modTime: info.ModTime(),
		size:    info.Size(),
	}
	if !s.isDir {
		return s
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return s
	}
	s.children = make(map[string]childInfo, len(entries))
	for _, e := range entries {
		ei, err := e.Info()
		if err != nil {
			continue
		}
		s.children[e.Name()] = childInfo{
			modTime: ei.ModTime(),
			size:    ei.Size(),
		}
	}
	return s
}

// diffAndEmit compares prev with curr for path and pushes events
// onto the channel. The polling loop's caller has already taken a
// fresh curr; this function does no IO.
func (w *Watcher) diffAndEmit(path string, prev, curr *snapshot) {
	// Path-level transitions.
	switch {
	case prev.exists && !curr.exists:
		w.emit(Event{Path: path, Kind: Removed})
		return
	case !prev.exists && curr.exists:
		// Path appeared. We treat re-appearance as Modified rather
		// than Created — the path was "registered" so a consumer
		// already knows about it; "Created" is reserved for new
		// children inside a watched directory.
		w.emit(Event{Path: path, Kind: Modified, ModTime: curr.modTime})
		return
	case !prev.exists && !curr.exists:
		// Still missing — nothing to report.
		return
	}

	// Both exist. For files: check mtime/size; for dirs: diff children.
	if !curr.isDir {
		if !prev.modTime.Equal(curr.modTime) || prev.size != curr.size {
			w.emit(Event{Path: path, Kind: Modified, ModTime: curr.modTime})
		}
		return
	}

	// Directory: diff children.
	for name, ci := range curr.children {
		full := filepath.Join(path, name)
		old, ok := prev.children[name]
		switch {
		case !ok:
			w.emit(Event{Path: full, Kind: Created, ModTime: ci.modTime})
		case !old.modTime.Equal(ci.modTime) || old.size != ci.size:
			w.emit(Event{Path: full, Kind: Modified, ModTime: ci.modTime})
		}
	}
	for name := range prev.children {
		if _, ok := curr.children[name]; !ok {
			full := filepath.Join(path, name)
			w.emit(Event{Path: full, Kind: Removed})
		}
	}
}

// emit pushes ev onto the events channel. Drops the event if the
// channel buffer is full — the consumer is too slow and we'd rather
// the watcher keep up than block the polling loop. Hosts that need
// loss-free delivery should use a dedicated drainer goroutine
// forwarding into a larger queue.
func (w *Watcher) emit(ev Event) {
	select {
	case w.events <- ev:
	default:
		// drop
	}
}
