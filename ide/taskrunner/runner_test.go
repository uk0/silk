package taskrunner

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// The Runner tests spawn real child processes (the go tool and sleep,
// both behind the LookPath guards in process_test.go) but never any UI:
// no Frame, no Draw, no font metrics. requireGo, requireSleep, goTask and
// sleepTask are shared with process_test.go.

// --- helpers ---------------------------------------------------------

// stream drains a Runner's event channel for the whole test — a consumer
// that stops reading would block the tasks that are producing output.
type stream struct {
	r    *Runner
	mu   sync.Mutex
	evs  []Event
	done chan struct{}
}

func newStream(r *Runner) *stream {
	s := &stream{r: r, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		for ev := range r.Events() {
			s.mu.Lock()
			s.evs = append(s.evs, ev)
			s.mu.Unlock()
		}
	}()
	return s
}

func (s *stream) all() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.evs...)
}

// close ends the stream and returns everything that was collected. It
// must be called after Run has returned.
func (s *stream) close(t *testing.T) []Event {
	t.Helper()
	if err := s.r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	<-s.done
	return s.all()
}

// index is the position of the first (kind, taskID) event, or -1.
func (s *stream) index(kind, taskID string) int {
	for i, ev := range s.all() {
		if ev.Kind == kind && ev.TaskID == taskID {
			return i
		}
	}
	return -1
}

// count is how many (kind, taskID) events arrived.
func (s *stream) count(kind, taskID string) int {
	n := 0
	for _, ev := range s.all() {
		if ev.Kind == kind && ev.TaskID == taskID {
			n++
		}
	}
	return n
}

// lines is the Line of every (kind, taskID) event.
func (s *stream) lines(kind, taskID string) []string {
	var out []string
	for _, ev := range s.all() {
		if ev.Kind == kind && ev.TaskID == taskID {
			out = append(out, ev.Line)
		}
	}
	return out
}

// await blocks until a (kind, taskID) event shows up, and reports whether
// it did within d.
func (s *stream) await(t *testing.T, kind, taskID string, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if s.index(kind, taskID) >= 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no %s event for %q within %v; events: %v", kind, taskID, d, s.all())
}

// submitAll queues tasks, failing the test on a rejected submission.
func submitAll(t *testing.T, r *Runner, tasks ...Task) {
	t.Helper()
	for _, task := range tasks {
		if err := r.Submit(task); err != nil {
			t.Fatalf("Submit(%q): %v", task.ID, err)
		}
	}
}

// records indexes History by task ID.
func records(r *Runner) map[string]Record {
	out := make(map[string]Record)
	for _, rec := range r.History() {
		out[rec.TaskID] = rec
	}
	return out
}

// wantStatus asserts one task's terminal status.
func wantStatus(t *testing.T, r *Runner, taskID string, want Status) Record {
	t.Helper()
	rec, ok := records(r)[taskID]
	if !ok {
		t.Fatalf("no history record for %q; history: %+v", taskID, r.History())
	}
	if rec.Status != want {
		t.Errorf("%q status = %q (code %d, err %v), want %q",
			taskID, rec.Status, rec.Code, rec.Err, want)
	}
	return rec
}

// mustRun runs the graph and fails on a setup error.
func mustRun(t *testing.T, r *Runner) {
	t.Helper()
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

// --- submission ------------------------------------------------------

func TestSubmitRejectsBadTasks(t *testing.T) {
	r := New()
	defer r.Close()

	if err := r.Submit(Task{Cmd: "go"}); !errors.Is(err, ErrEmptyID) {
		t.Errorf("Submit(no id) = %v, want ErrEmptyID", err)
	}
	if err := r.Submit(Task{ID: "a"}); !errors.Is(err, ErrEmptyCmd) {
		t.Errorf("Submit(no cmd) = %v, want ErrEmptyCmd", err)
	}
	submitAll(t, r, goTask("a", "version"))
	var dup *DuplicateIDError
	if err := r.Submit(goTask("a", "version")); !errors.As(err, &dup) {
		t.Errorf("Submit(duplicate) = %v, want *DuplicateIDError", err)
	}
}

func TestRunNoTasks(t *testing.T) {
	r := New()
	defer r.Close()
	if err := r.Run(context.Background()); !errors.Is(err, ErrNoTasks) {
		t.Errorf("Run(empty) = %v, want ErrNoTasks", err)
	}
}

func TestClosedRunnerRejectsWork(t *testing.T) {
	r := New()
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Errorf("second Close = %v, want nil (idempotent)", err)
	}
	if err := r.Submit(goTask("a", "version")); !errors.Is(err, ErrClosed) {
		t.Errorf("Submit after Close = %v, want ErrClosed", err)
	}
	if err := r.Run(context.Background()); !errors.Is(err, ErrClosed) {
		t.Errorf("Run after Close = %v, want ErrClosed", err)
	}
}

// TestRunRejectsCycle: a malformed graph is refused before anything is
// spawned, so the history stays empty.
func TestRunRejectsCycle(t *testing.T) {
	r := New()
	defer r.Close()
	a := goTask("a", "version")
	a.DependsOn = []string{"b"}
	b := goTask("b", "version")
	b.DependsOn = []string{"a"}
	submitAll(t, r, a, b)

	var cyc *CycleError
	if err := r.Run(context.Background()); !errors.As(err, &cyc) {
		t.Fatalf("Run(cycle) = %v, want *CycleError", err)
	}
	if h := r.History(); len(h) != 0 {
		t.Errorf("history = %+v, want empty", h)
	}
}

// --- happy path ------------------------------------------------------

// TestRunSingleTask: start, one stdout line, exit 0 — plus a history
// record with a real duration.
func TestRunSingleTask(t *testing.T) {
	requireGo(t)

	r := New()
	s := newStream(r)
	submitAll(t, r, goTask("env", "env", "GOOS"))
	mustRun(t, r)
	s.close(t)

	if got := s.count(EventStart, "env"); got != 1 {
		t.Errorf("start events = %d, want 1", got)
	}
	if got := s.lines(EventStdout, "env"); len(got) != 1 || got[0] == "" {
		t.Errorf("stdout = %v, want one non-empty line", got)
	}
	if got := s.count(EventExit, "env"); got != 1 {
		t.Fatalf("exit events = %d, want exactly 1", got)
	}
	exit := s.all()[s.index(EventExit, "env")]
	if exit.Code != 0 || exit.Line != "" {
		t.Errorf("exit event = %+v, want code 0 and no reason", exit)
	}
	// start must precede the output, output must precede the exit.
	if a, b := s.index(EventStart, "env"), s.index(EventStdout, "env"); a >= b {
		t.Errorf("start at %d, stdout at %d: start must come first", a, b)
	}
	if a, b := s.index(EventStdout, "env"), s.index(EventExit, "env"); a >= b {
		t.Errorf("stdout at %d, exit at %d: exit must come last", a, b)
	}

	rec := wantStatus(t, r, "env", StatusSucceeded)
	if rec.Err != nil {
		t.Errorf("Err = %v, want nil", rec.Err)
	}
	if rec.Code != 0 {
		t.Errorf("Code = %d, want 0", rec.Code)
	}
	if rec.Duration <= 0 || rec.Duration != rec.Finished.Sub(rec.Started) {
		t.Errorf("Duration = %v (started %v, finished %v)", rec.Duration, rec.Started, rec.Finished)
	}
}

// TestRunRespectsDependencyOrder: a dependent task must not start before
// its dependency has exited, and history follows the same order.
func TestRunRespectsDependencyOrder(t *testing.T) {
	requireGo(t)

	r := New()
	s := newStream(r)
	second := goTask("second", "env", "GOARCH")
	second.DependsOn = []string{"first"}
	submitAll(t, r, goTask("first", "env", "GOOS"), second)
	mustRun(t, r)
	s.close(t)

	if a, b := s.index(EventExit, "first"), s.index(EventStart, "second"); a < 0 || b < 0 || a >= b {
		t.Errorf("first exited at %d, second started at %d: %v", a, b, s.all())
	}
	wantStatus(t, r, "first", StatusSucceeded)
	wantStatus(t, r, "second", StatusSucceeded)
	if h := r.History(); len(h) != 2 || h[0].TaskID != "first" || h[1].TaskID != "second" {
		t.Errorf("history order = %v, want [first second]", historyIDs(h))
	}
}

// TestRunIndependentTasksOverlap: two tasks with no edge between them run
// at the same time. Both sleeps must be started before either exits —
// serial execution could not produce that interleaving.
func TestRunIndependentTasksOverlap(t *testing.T) {
	requireSleep(t)

	r := New()
	s := newStream(r)
	submitAll(t, r, sleepTask("one", "1"), sleepTask("two", "1"))
	mustRun(t, r)
	s.close(t)

	startOne, startTwo := s.index(EventStart, "one"), s.index(EventStart, "two")
	exitOne, exitTwo := s.index(EventExit, "one"), s.index(EventExit, "two")
	if startOne < 0 || startTwo < 0 || exitOne < 0 || exitTwo < 0 {
		t.Fatalf("missing events: %v", s.all())
	}
	if startTwo > exitOne || startOne > exitTwo {
		t.Errorf("tasks did not overlap: starts %d/%d, exits %d/%d",
			startOne, startTwo, exitOne, exitTwo)
	}
	wantStatus(t, r, "one", StatusSucceeded)
	wantStatus(t, r, "two", StatusSucceeded)
}

// TestRunTwiceAccumulatesHistory: the Runner keeps the record of earlier
// runs, which is what makes History a build history and not just the last
// result.
func TestRunTwiceAccumulatesHistory(t *testing.T) {
	requireGo(t)

	r := New()
	s := newStream(r)
	submitAll(t, r, goTask("env", "env", "GOOS"))
	mustRun(t, r)
	mustRun(t, r)
	s.close(t)

	h := r.History()
	if len(h) != 2 {
		t.Fatalf("history = %v, want two records", historyIDs(h))
	}
	for i, rec := range h {
		if rec.TaskID != "env" || rec.Status != StatusSucceeded {
			t.Errorf("record %d = %+v, want env/succeeded", i, rec)
		}
	}
	if got := s.count(EventStart, "env"); got != 2 {
		t.Errorf("start events = %d, want 2", got)
	}
}

// --- failure propagation ---------------------------------------------

// TestFailedDependencySkipsDependents: a failing task takes its whole
// downstream with it — transitively — and the skipped tasks are never
// spawned but still report an exit event and a reason.
func TestFailedDependencySkipsDependents(t *testing.T) {
	requireGo(t)

	r := New()
	s := newStream(r)
	mid := goTask("mid", "env", "GOOS")
	mid.DependsOn = []string{"bad"}
	leaf := goTask("leaf", "env", "GOARCH")
	leaf.DependsOn = []string{"mid"}
	submitAll(t, r, goTask("bad", "help", "no-such-help-topic-xyz"), mid, leaf)
	mustRun(t, r)
	s.close(t)

	bad := wantStatus(t, r, "bad", StatusFailed)
	if bad.Code <= 0 {
		t.Errorf("bad exit code = %d, want a positive exit status", bad.Code)
	}
	if bad.Err == nil {
		t.Error("failed task has no Err")
	}

	for _, tc := range []struct{ id, blocker string }{{"mid", "bad"}, {"leaf", "mid"}} {
		rec := wantStatus(t, r, tc.id, StatusSkipped)
		if rec.Err == nil || !strings.Contains(rec.Err.Error(), tc.blocker) {
			t.Errorf("%q Err = %v, want it to name %q", tc.id, rec.Err, tc.blocker)
		}
		if rec.Code != noExitCode {
			t.Errorf("%q Code = %d, want %d", tc.id, rec.Code, noExitCode)
		}
		if got := s.count(EventStart, tc.id); got != 0 {
			t.Errorf("%q emitted %d start events, want 0 (never spawned)", tc.id, got)
		}
		if got := s.count(EventExit, tc.id); got != 1 {
			t.Errorf("%q emitted %d exit events, want 1", tc.id, got)
		}
	}
	// Failure first, then the downstream in graph order.
	if h := r.History(); len(h) != 3 ||
		h[0].TaskID != "bad" || h[1].TaskID != "mid" || h[2].TaskID != "leaf" {
		t.Errorf("history order = %v, want [bad mid leaf]", historyIDs(h))
	}
}

// --- cancellation ----------------------------------------------------

// TestCancelAllStopsRun: a long-running task is killed and Run comes back
// promptly instead of waiting the sleep out. While the run is in flight,
// Submit and Close must both refuse.
func TestCancelAllStopsRun(t *testing.T) {
	requireSleep(t)

	r := New()
	s := newStream(r)
	submitAll(t, r, sleepTask("long", "30"))

	errc := make(chan error, 1)
	go func() { errc <- r.Run(context.Background()) }()
	s.await(t, EventStart, "long", 10*time.Second)

	if err := r.Submit(goTask("late", "version")); !errors.Is(err, ErrRunning) {
		t.Errorf("Submit during a run = %v, want ErrRunning", err)
	}
	if err := r.Close(); !errors.Is(err, ErrRunning) {
		t.Errorf("Close during a run = %v, want ErrRunning", err)
	}

	start := time.Now()
	r.CancelAll()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after CancelAll")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Run took %v after CancelAll, want a prompt return", elapsed)
	}
	s.close(t)

	rec := wantStatus(t, r, "long", StatusCancelled)
	if !errors.Is(rec.Err, context.Canceled) {
		t.Errorf("Err = %v, want context.Canceled", rec.Err)
	}
	if got := s.count(EventExit, "long"); got != 1 {
		t.Errorf("exit events = %d, want 1", got)
	}
	// A cancelled Runner is idle again: CancelAll is now a no-op.
	r.CancelAll()
}

// TestCancelQueuedTaskNeverStarts: cancelling a task that is still
// waiting on a dependency drops it without spawning anything, while the
// dependency itself completes normally.
func TestCancelQueuedTaskNeverStarts(t *testing.T) {
	requireGo(t)
	requireSleep(t)

	r := New()
	s := newStream(r)
	queued := goTask("queued", "env", "GOOS")
	queued.DependsOn = []string{"gate"}
	submitAll(t, r, sleepTask("gate", "1"), queued)

	errc := make(chan error, 1)
	go func() { errc <- r.Run(context.Background()) }()
	s.await(t, EventStart, "gate", 10*time.Second)

	if !r.Cancel("queued") {
		t.Fatal("Cancel(queued) = false, want true for a queued task")
	}
	if r.Cancel("ghost") {
		t.Error("Cancel(unknown id) = true, want false")
	}
	if err := <-errc; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	s.close(t)

	wantStatus(t, r, "gate", StatusSucceeded)
	rec := wantStatus(t, r, "queued", StatusCancelled)
	if !errors.Is(rec.Err, context.Canceled) {
		t.Errorf("Err = %v, want context.Canceled", rec.Err)
	}
	if got := s.count(EventStart, "queued"); got != 0 {
		t.Errorf("queued task emitted %d start events, want 0", got)
	}
	if got := s.count(EventExit, "queued"); got != 1 {
		t.Errorf("queued task emitted %d exit events, want 1", got)
	}
}

// TestCancelIdleRunner: nothing is running, so there is nothing to
// cancel and nothing to panic about.
func TestCancelIdleRunner(t *testing.T) {
	r := New()
	defer r.Close()
	submitAll(t, r, goTask("a", "version"))
	if r.Cancel("a") {
		t.Error("Cancel while idle = true, want false")
	}
	r.CancelAll()
}

// TestRunWithCancelledContext: a context that is already done cancels
// every task in the graph without spawning a single process. Run itself
// still succeeds — the outcome lives in the records.
func TestRunWithCancelledContext(t *testing.T) {
	r := New()
	s := newStream(r)
	second := goTask("second", "env", "GOARCH")
	second.DependsOn = []string{"first"}
	submitAll(t, r, goTask("first", "env", "GOOS"), second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	s.close(t)

	for _, id := range []string{"first", "second"} {
		rec := wantStatus(t, r, id, StatusCancelled)
		if !errors.Is(rec.Err, context.Canceled) {
			t.Errorf("%q Err = %v, want context.Canceled", id, rec.Err)
		}
		if got := s.count(EventStart, id); got != 0 {
			t.Errorf("%q emitted %d start events, want 0", id, got)
		}
		if got := s.count(EventExit, id); got != 1 {
			t.Errorf("%q emitted %d exit events, want 1", id, got)
		}
	}
}

// --- misc ------------------------------------------------------------

func TestTaskCommandLine(t *testing.T) {
	for _, tc := range []struct {
		task Task
		want string
	}{
		{Task{Cmd: "go", Args: []string{"build", "./..."}}, "go build ./..."},
		{Task{Cmd: "go"}, "go"},
		{Task{Cmd: "go", Args: []string{"test", "-run", "Test A"}}, `go test -run "Test A"`},
		{Task{Cmd: "echo", Args: []string{""}}, `echo ""`},
	} {
		if got := tc.task.CommandLine(); got != tc.want {
			t.Errorf("CommandLine() = %q, want %q", got, tc.want)
		}
	}
}

// historyIDs is a readable rendering of a history slice for failures.
func historyIDs(h []Record) []string {
	out := make([]string, len(h))
	for i, rec := range h {
		out[i] = rec.TaskID + ":" + string(rec.Status)
	}
	return out
}
