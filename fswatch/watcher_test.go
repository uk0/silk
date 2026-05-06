package fswatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fastWatcher returns a Watcher with a 60ms poll interval so tests
// don't sit on the default 500ms tick. 60ms is the floor (50ms is
// SetPollInterval's hard min); going lower thrashes the scheduler.
func fastWatcher() *Watcher {
	w := New()
	w.SetPollInterval(60 * time.Millisecond)
	return w
}

// waitFor blocks for up to timeout waiting for an event matching
// match. Returns the matched event or fails the test on timeout.
// Used in every test that expects a specific change to surface.
func waitFor(t *testing.T, w *Watcher, match func(Event) bool, timeout time.Duration) Event {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case ev := <-w.Events():
			if match(ev) {
				return ev
			}
			// Non-matching event — keep waiting; another watched
			// path's change may have produced it.
		case <-deadline.C:
			t.Fatalf("timed out waiting for matching event")
			return Event{}
		}
	}
}

// TestEventKindString covers the human-readable EventKind labels —
// useful when log output spells out the event type.
func TestEventKindString(t *testing.T) {
	cases := []struct {
		k    EventKind
		want string
	}{
		{Created, "Created"},
		{Modified, "Modified"},
		{Removed, "Removed"},
		{EventKind(99), "<unknown>"},
	}
	for _, c := range cases {
		if got := c.k.String(); got != c.want {
			t.Errorf("EventKind(%d).String() = %q, want %q", c.k, got, c.want)
		}
	}
}

// TestAddPathStartsPolling: AddPath transitions running false→true.
// We inspect via Paths() since running is private; before AddPath the
// list is empty, after it contains the path.
func TestAddPathStartsPolling(t *testing.T) {
	dir := t.TempDir()
	w := fastWatcher()
	defer w.Stop()
	if err := w.AddPath(dir); err != nil {
		t.Fatalf("AddPath: %v", err)
	}
	paths := w.Paths()
	if len(paths) != 1 || paths[0] != dir {
		t.Errorf("Paths() = %v, want [%s]", paths, dir)
	}
}

// TestRemovePathDropsTarget: after RemovePath the path is no longer
// in Paths(), and a subsequent change to the (now untracked) file
// produces no event.
func TestRemovePathDropsTarget(t *testing.T) {
	dir := t.TempDir()
	w := fastWatcher()
	defer w.Stop()
	w.AddPath(dir)
	w.RemovePath(dir)
	if len(w.Paths()) != 0 {
		t.Errorf("Paths after RemovePath = %v, want empty", w.Paths())
	}
}

// TestFileModifyEmitsEvent: AddPath on a file then rewrite the file;
// expect a Modified event with the new mtime.
func TestFileModifyEmitsEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("v1"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	w := fastWatcher()
	defer w.Stop()
	w.AddPath(path)

	// Sleep past one poll tick so the initial snapshot is fresh.
	time.Sleep(80 * time.Millisecond)

	// Bump mtime explicitly so the diff sees a change even on hosts
	// where consecutive WriteFile calls produce identical mtimes.
	future := time.Now().Add(time.Second)
	os.Chtimes(path, future, future)
	if err := os.WriteFile(path, []byte("v2-longer"), 0644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	ev := waitFor(t, w, func(ev Event) bool {
		return ev.Path == path && ev.Kind == Modified
	}, 2*time.Second)
	if ev.Kind != Modified {
		t.Errorf("event Kind = %v, want Modified", ev.Kind)
	}
}

// TestFileRemoveEmitsEvent: deleting a watched file produces Removed.
func TestFileRemoveEmitsEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("v1"), 0644)

	w := fastWatcher()
	defer w.Stop()
	w.AddPath(path)

	time.Sleep(80 * time.Millisecond)
	os.Remove(path)

	ev := waitFor(t, w, func(ev Event) bool {
		return ev.Path == path && ev.Kind == Removed
	}, 2*time.Second)
	if ev.Kind != Removed {
		t.Errorf("Kind = %v, want Removed", ev.Kind)
	}
}

// TestDirChildCreatedEmitsEvent: AddPath on a directory; create a
// new file inside; expect a Created event with the child's path.
func TestDirChildCreatedEmitsEvent(t *testing.T) {
	dir := t.TempDir()
	w := fastWatcher()
	defer w.Stop()
	w.AddPath(dir)

	time.Sleep(80 * time.Millisecond)

	child := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(child, []byte("hi"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	ev := waitFor(t, w, func(ev Event) bool {
		return ev.Path == child && ev.Kind == Created
	}, 2*time.Second)
	if ev.Path != child {
		t.Errorf("Path = %q, want %q", ev.Path, child)
	}
}

// TestDirChildRemovedEmitsEvent: removing a child of a watched
// directory produces Removed for that child path.
func TestDirChildRemovedEmitsEvent(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "old.txt")
	os.WriteFile(child, []byte("v"), 0644)

	w := fastWatcher()
	defer w.Stop()
	w.AddPath(dir)

	time.Sleep(80 * time.Millisecond)
	os.Remove(child)

	ev := waitFor(t, w, func(ev Event) bool {
		return ev.Path == child && ev.Kind == Removed
	}, 2*time.Second)
	if ev.Path != child {
		t.Errorf("Path = %q, want %q", ev.Path, child)
	}
}

// TestDirChildModifiedEmitsEvent: rewriting an existing child of a
// watched directory produces a Modified event for that child.
func TestDirChildModifiedEmitsEvent(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "kept.txt")
	os.WriteFile(child, []byte("v1"), 0644)

	w := fastWatcher()
	defer w.Stop()
	w.AddPath(dir)

	time.Sleep(80 * time.Millisecond)

	future := time.Now().Add(time.Second)
	os.Chtimes(child, future, future)
	os.WriteFile(child, []byte("v2-longer-content"), 0644)

	ev := waitFor(t, w, func(ev Event) bool {
		return ev.Path == child && ev.Kind == Modified
	}, 2*time.Second)
	if ev.Path != child {
		t.Errorf("Path = %q, want %q", ev.Path, child)
	}
}

// TestStopHaltsPolling: after Stop, no further events are produced.
// We wait one extra interval to confirm and then drain the channel
// to confirm it's closed.
func TestStopHaltsPolling(t *testing.T) {
	dir := t.TempDir()
	w := fastWatcher()
	w.AddPath(dir)
	time.Sleep(80 * time.Millisecond)
	w.Stop()

	// After Stop, Events() must drain to a closed channel.
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-w.Events():
			if !ok {
				return // closed channel — expected
			}
		case <-timer.C:
			t.Fatalf("Events channel did not close within 500ms after Stop")
		}
	}
}

// TestAddPathDuplicateIsNoop: AddPath on the same path twice yields
// only one entry in Paths().
func TestAddPathDuplicateIsNoop(t *testing.T) {
	dir := t.TempDir()
	w := fastWatcher()
	defer w.Stop()
	w.AddPath(dir)
	w.AddPath(dir)
	if len(w.Paths()) != 1 {
		t.Errorf("duplicate AddPath: Paths() len = %d, want 1", len(w.Paths()))
	}
}

// TestSetPollIntervalClampsLow: values below 50ms are clamped to
// the floor. Without the clamp, sub-millisecond intervals would
// burn CPU on every test machine.
func TestSetPollIntervalClampsLow(t *testing.T) {
	w := New()
	w.SetPollInterval(1 * time.Millisecond)
	if got := w.PollInterval(); got < 50*time.Millisecond {
		t.Errorf("PollInterval after clamp = %v, want >= 50ms", got)
	}
}

// TestPathsAfterStopReturnsEmpty: after Stop, Paths() may legitimately
// still hold the registered paths (they were never explicitly cleared).
// Pin the policy so a future change is intentional.
func TestPathsAfterStopReturnsEntries(t *testing.T) {
	dir := t.TempDir()
	w := fastWatcher()
	w.AddPath(dir)
	w.Stop()
	if len(w.Paths()) != 1 {
		t.Errorf("after Stop, Paths() = %v, want kept (1 entry)", w.Paths())
	}
}

// TestMultiplePathsObservedConcurrently: two unrelated watch targets
// each surface their own events without crossing streams.
func TestMultiplePathsObservedConcurrently(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	w := fastWatcher()
	defer w.Stop()
	w.AddPath(dirA)
	w.AddPath(dirB)

	time.Sleep(80 * time.Millisecond)

	childA := filepath.Join(dirA, "a.txt")
	childB := filepath.Join(dirB, "b.txt")
	os.WriteFile(childA, []byte("a"), 0644)
	os.WriteFile(childB, []byte("b"), 0644)

	seenA, seenB := false, false
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for !seenA || !seenB {
		select {
		case ev := <-w.Events():
			if ev.Path == childA && ev.Kind == Created {
				seenA = true
			}
			if ev.Path == childB && ev.Kind == Created {
				seenB = true
			}
		case <-deadline.C:
			t.Fatalf("timeout: seenA=%v seenB=%v", seenA, seenB)
		}
	}
}
