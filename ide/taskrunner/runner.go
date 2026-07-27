// Package taskrunner executes an IDE's build / run / test commands as
// child processes: a dependency graph of Tasks run in topological order
// (independent tasks in parallel), every output line streamed as an
// Event, per-task and whole-run cancellation that kills the entire
// process group, and a History of what finished, how long it took and how
// it ended.
//
// It exists to replace the hardcoded, uncancellable `go build ./...`,
// `go test -v ./...` and `go vet ./...` invocations in cmd/silkide: those
// spawn a bare exec.Command per keystroke, with no queue, no way to stop
// a runaway build and no record of what ran before.
//
// Design notes:
//
//   - Nothing here touches the GUI. The Runner only emits Events; a panel
//     decides how to render them. That keeps the package headless — no
//     GLFW, no Cairo, no font metrics — so it is fully testable.
//
//   - Every task ends with exactly ONE EventExit, including tasks that
//     never spawned a process (cancelled while queued, or skipped because
//     a dependency did not succeed). EventStart is emitted only for a
//     task whose process actually started, so a consumer can rely on
//     "start … exit" or a bare "exit".
//
//   - Events go out on a buffered channel and are never dropped: a
//     consumer MUST drain Events for the whole duration of Run, otherwise
//     a full buffer blocks the task that is producing output. Close ends
//     the stream when the Runner is done with.
//
//   - Cancellation kills the process GROUP, not just the direct child, so
//     the compilers and test binaries the go tool forks die with it (see
//     process.go).
//
//   - A Task never goes through a shell: Cmd and Args are passed to
//     os/exec as-is, so nothing in a task definition can be expanded,
//     globbed or injected.
package taskrunner

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Event kinds. Kind is a plain string so a UI can switch on it and log it
// without a conversion table.
const (
	// EventStart reports that a task's process started. Line carries the
	// rendered command line (Task.CommandLine).
	EventStart = "start"
	// EventStdout is one line the task wrote to stdout, newline stripped.
	EventStdout = "stdout"
	// EventStderr is one line the task wrote to stderr, newline stripped.
	EventStderr = "stderr"
	// EventExit reports that a task reached a final state. Code is the
	// process exit status, or -1 when there is none (killed by a signal,
	// never started, cancelled while queued, skipped). Line carries the
	// reason when the task did not succeed, and is empty otherwise.
	EventExit = "exit"
)

// Status is the lifecycle state of a task within one run.
type Status string

// Task states. Pending and Running are transient; the remaining four are
// terminal and are what shows up in History.
const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
	StatusSkipped   Status = "skipped"
)

// Sentinel errors returned by Submit, Run and Close. Graph problems are
// reported by TopoSort as *CycleError, *DuplicateIDError or
// *MissingDepError.
var (
	ErrEmptyID  = errors.New("taskrunner: task id must not be empty")
	ErrEmptyCmd = errors.New("taskrunner: task command must not be empty")
	ErrRunning  = errors.New("taskrunner: a run is already in flight")
	ErrClosed   = errors.New("taskrunner: runner is closed")
	ErrNoTasks  = errors.New("taskrunner: no tasks submitted")
)

// Task is one command to run. ID is the handle used by Cancel and carried
// by every Event, and must be unique within a Runner; Name is the
// human-readable label a UI shows ("Build", "Run tests"). Cmd/Args are
// passed straight to os/exec — no shell, no expansion. Dir is the working
// directory (empty means the parent's). Env holds overrides layered on
// top of the parent environment, not a replacement for it. DependsOn
// lists task IDs that must SUCCEED first; if one of them fails, is
// cancelled or is itself skipped, this task is skipped.
type Task struct {
	ID        string
	Name      string
	Cmd       string
	Args      []string
	Dir       string
	Env       map[string]string
	DependsOn []string
}

// CommandLine renders the task as a one-line header for a log pane, e.g.
// `go build ./...`. Empty arguments and arguments containing whitespace
// or quotes are quoted so the line stays unambiguous. Display only —
// nothing is ever handed to a shell.
func (t Task) CommandLine() string {
	parts := make([]string, 0, len(t.Args)+1)
	parts = append(parts, t.Cmd)
	for _, arg := range t.Args {
		if arg == "" || strings.ContainsAny(arg, " \t\"") {
			arg = strconv.Quote(arg)
		}
		parts = append(parts, arg)
	}
	return strings.Join(parts, " ")
}

// Event is one thing that happened to a task, streamed live on the
// channel returned by Events. At is the wall-clock time it happened.
type Event struct {
	TaskID string
	Kind   string
	Line   string
	Code   int
	At     time.Time
}

// Record is the post-mortem of one finished task: how it ended, with what
// exit code, and how long it took. Err carries the detail behind a
// non-successful Status (the Wait error for StatusFailed, context.Canceled
// for StatusCancelled, the blocking dependency for StatusSkipped) and is
// nil for StatusSucceeded. Code is -1 when there was no exit status.
type Record struct {
	TaskID   string
	Name     string
	Status   Status
	Code     int
	Started  time.Time
	Finished time.Time
	Duration time.Duration
	Err      error
}

// eventBuffer is the depth of the Event channel. Deep enough that a
// normal build's output never blocks the producer between two UI frames,
// small enough that a runaway task cannot buffer unbounded output.
const eventBuffer = 256

// taskState is the Runner's live bookkeeping for one task of the current
// run. All fields are guarded by Runner.mu.
type taskState struct {
	status Status
	// cancel stops the running process; non-nil only while status is
	// StatusRunning.
	cancel context.CancelFunc
	// requested records a Cancel that arrived before the task started, so
	// the scheduler never launches it.
	requested bool
}

// Runner queues tasks, runs the resulting dependency graph and reports
// what happened. A Runner is safe for concurrent use: Submit, Cancel,
// CancelAll and History may be called from any goroutine (typically the
// UI thread) while Run executes on another.
type Runner struct {
	mu      sync.Mutex
	tasks   []Task
	ids     map[string]bool
	state   map[string]*taskState
	history []Record
	running bool
	closed  bool
	// cancelRun cancels the whole in-flight run; nil when idle.
	cancelRun context.CancelFunc

	events chan Event
}

// New returns an idle Runner with an open event stream.
func New() *Runner {
	return &Runner{
		ids:    make(map[string]bool),
		events: make(chan Event, eventBuffer),
	}
}

// Events is the live stream of the Runner. The consumer must keep reading
// it while Run executes; the channel is closed by Close.
func (r *Runner) Events() <-chan Event {
	return r.events
}

// Close ends the event stream so a `for range Events()` consumer
// terminates. It is idempotent, and reports ErrRunning if a run is still
// in flight (closing the channel under a producer would panic) — cancel
// and let Run return first.
func (r *Runner) Close() error {
	r.mu.Lock()
	switch {
	case r.running:
		r.mu.Unlock()
		return ErrRunning
	case r.closed:
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()
	close(r.events)
	return nil
}

// Submit queues a task. It rejects an empty ID or Cmd, an ID that is
// already queued, and any submission while a run is in flight (the run
// works off a snapshot, so a late task would silently not run).
func (r *Runner) Submit(t Task) error {
	switch {
	case t.ID == "":
		return ErrEmptyID
	case t.Cmd == "":
		return ErrEmptyCmd
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case r.closed:
		return ErrClosed
	case r.running:
		return ErrRunning
	case r.ids[t.ID]:
		return &DuplicateIDError{ID: t.ID}
	}
	r.ids[t.ID] = true
	r.tasks = append(r.tasks, t)
	return nil
}

// Run executes every submitted task, honouring DependsOn: a task starts
// once all of its dependencies have succeeded, and tasks with no
// dependency on each other run concurrently. Run blocks until every task
// has reached a terminal state and all of its events have been handed to
// the stream.
//
// The returned error only describes why the run could not be performed as
// specified — ErrClosed, ErrRunning, ErrNoTasks, or a graph error from
// TopoSort (*CycleError, *DuplicateIDError, *MissingDepError). The
// outcome of the tasks themselves is not an error of Run: read it from
// the EventExit stream or from History.
//
// Cancelling ctx has the same effect as CancelAll: running processes are
// killed and queued tasks are recorded as cancelled.
func (r *Runner) Run(ctx context.Context) error {
	r.mu.Lock()
	switch {
	case r.closed:
		r.mu.Unlock()
		return ErrClosed
	case r.running:
		r.mu.Unlock()
		return ErrRunning
	case len(r.tasks) == 0:
		r.mu.Unlock()
		return ErrNoTasks
	}
	tasks := append([]Task(nil), r.tasks...)
	r.mu.Unlock()

	nodes := make([]Node, len(tasks))
	byID := make(map[string]Task, len(tasks))
	for i, t := range tasks {
		nodes[i] = Node{ID: t.ID, DependsOn: t.DependsOn}
		byID[t.ID] = t
	}
	// TopoSort validates the graph and fixes the deterministic order in
	// which the scheduler considers tasks below.
	order, err := TopoSort(nodes)
	if err != nil {
		return err
	}

	runCtx, cancelRun := context.WithCancel(ctx)
	r.mu.Lock()
	r.running = true
	r.cancelRun = cancelRun
	r.state = make(map[string]*taskState, len(tasks))
	for _, t := range tasks {
		r.state[t.ID] = &taskState{status: StatusPending}
	}
	r.mu.Unlock()
	defer func() {
		cancelRun()
		r.mu.Lock()
		r.running = false
		r.cancelRun = nil
		r.mu.Unlock()
	}()

	// done is sized for the whole run so a finishing task never blocks on
	// the handover, whatever the scheduler is doing.
	done := make(chan Record, len(tasks))
	remaining := len(tasks)
	inflight := 0
	for remaining > 0 {
		// One pass over the graph order: launch everything that became
		// runnable, resolve everything that can no longer run. Tasks that
		// are still waiting are simply left for the next pass.
		for _, id := range order {
			switch act, blocker := r.step(runCtx, byID[id], done); act {
			case actionRun:
				inflight++
			case actionCancel:
				r.settle(unrun(byID[id], StatusCancelled, context.Canceled))
				remaining--
			case actionSkip:
				r.settle(unrun(byID[id], StatusSkipped,
					fmt.Errorf("dependency %q did not succeed", blocker)))
				remaining--
			}
		}
		if inflight == 0 {
			// Nothing running and nothing launched: the pass above
			// resolved every task that was left.
			break
		}
		rec := <-done
		inflight--
		remaining--
		r.settle(rec)
	}
	return nil
}

// Cancel stops one task of the in-flight run: a running task has its
// process group killed, a queued task is dropped before it starts (both
// end up as StatusCancelled, which in turn skips whatever depends on
// them). It reports whether there was such a task left to cancel.
func (r *Runner) Cancel(taskID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return false
	}
	st := r.state[taskID]
	if st == nil {
		return false
	}
	switch st.status {
	case StatusRunning:
		if st.cancel != nil {
			st.cancel()
		}
		return true
	case StatusPending:
		st.requested = true
		return true
	}
	return false
}

// CancelAll stops the whole in-flight run: every running process group is
// killed and every queued task is recorded as cancelled. It is a no-op
// when the Runner is idle. Run returns once the kills have been reaped.
func (r *Runner) CancelAll() {
	r.mu.Lock()
	cancel := r.cancelRun
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// History returns a copy of the finished-task records, oldest first, in
// the order the tasks reached their terminal state. It accumulates across
// runs of the same Runner and is safe to read while a run is in flight.
func (r *Runner) History() []Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Record(nil), r.history...)
}

// action is what the scheduler decided to do with one pending task.
type action int

const (
	// actionWait: the task's dependencies have not all finished yet.
	actionWait action = iota
	// actionRun: the task was claimed and its process was launched.
	actionRun
	// actionCancel: a cancel arrived before the task started.
	actionCancel
	// actionSkip: a dependency did not succeed; the blocker is returned.
	actionSkip
	// actionDone: nothing to do, the task is no longer pending.
	actionDone
)

// step advances a single pending task by one decision and, when that
// decision is "run", claims it: the status flip to StatusRunning, the
// install of the task's cancel func and the launch of its goroutine all
// happen under one lock acquisition, so a Cancel racing the launch can
// never be lost. Resolving a cancelled or skipped task is left to the
// caller (settle publishes an event and must not run under the lock).
func (r *Runner) step(runCtx context.Context, t Task, done chan<- Record) (action, string) {
	r.mu.Lock()
	st := r.state[t.ID]
	if st == nil || st.status != StatusPending {
		r.mu.Unlock()
		return actionDone, ""
	}
	if st.requested || runCtx.Err() != nil {
		r.mu.Unlock()
		return actionCancel, ""
	}
	ready := true
	for _, dep := range t.DependsOn {
		// TopoSort guarantees every dependency is part of the run.
		switch r.state[dep].status {
		case StatusSucceeded:
		case StatusFailed, StatusCancelled, StatusSkipped:
			r.mu.Unlock()
			return actionSkip, dep
		default:
			ready = false
		}
	}
	if !ready {
		r.mu.Unlock()
		return actionWait, ""
	}
	taskCtx, cancel := context.WithCancel(runCtx)
	st.status = StatusRunning
	st.cancel = cancel
	r.mu.Unlock()

	go r.execute(taskCtx, cancel, t, done)
	return actionRun, ""
}

// execute runs one task to completion on its own goroutine and hands the
// resulting Record back to the scheduler. A cancelled context outranks the
// process error: a killed command reports a Wait failure, but what the
// user did was cancel.
func (r *Runner) execute(ctx context.Context, cancel context.CancelFunc, t Task, done chan<- Record) {
	defer cancel()
	started := time.Now()
	code, err := runCommand(ctx, t, r.emit)
	finished := time.Now()
	rec := Record{
		TaskID:   t.ID,
		Name:     t.Name,
		Code:     code,
		Started:  started,
		Finished: finished,
		Duration: finished.Sub(started),
	}
	switch {
	case ctx.Err() != nil:
		rec.Status, rec.Err = StatusCancelled, ctx.Err()
	case err != nil:
		rec.Status, rec.Err = StatusFailed, err
	default:
		rec.Status = StatusSucceeded
	}
	done <- rec
}

// settle applies a terminal Record: it becomes the task's final status,
// it is appended to the history, and it is published as the task's single
// EventExit. The event is sent outside the lock so a blocked consumer
// cannot deadlock Cancel.
func (r *Runner) settle(rec Record) {
	r.mu.Lock()
	if st := r.state[rec.TaskID]; st != nil {
		st.status = rec.Status
		st.cancel = nil
	}
	r.history = append(r.history, rec)
	r.mu.Unlock()

	line := ""
	if rec.Err != nil {
		line = rec.Err.Error()
	}
	r.emit(Event{
		TaskID: rec.TaskID,
		Kind:   EventExit,
		Line:   line,
		Code:   rec.Code,
		At:     rec.Finished,
	})
}

// emit publishes an event, blocking while the consumer is behind rather
// than dropping build output.
func (r *Runner) emit(ev Event) {
	if ev.At.IsZero() {
		ev.At = time.Now()
	}
	r.events <- ev
}

// unrun builds the Record of a task that never spawned a process.
func unrun(t Task, status Status, err error) Record {
	now := time.Now()
	return Record{
		TaskID:   t.ID,
		Name:     t.Name,
		Status:   status,
		Code:     noExitCode,
		Started:  now,
		Finished: now,
		Err:      err,
	}
}
