package taskrunner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// These tests spawn real child processes but never a window: the package
// is pure os/exec, so it runs headless. Two external programs are used,
// both behind a LookPath guard so an unusual environment skips instead of
// failing:
//
//   - the go tool, for a fast deterministic command (`go env GOOS`) and a
//     deterministic failure (`go help <bogus>`),
//   - sleep, for a command that outlives the test unless it is killed.

// requireGo skips the calling test when the go tool is not on PATH.
// Shared with runner_test.go.
func requireGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go not on PATH: %v", err)
	}
}

// requireSleep skips the calling test when there is no sleep binary (it
// does not exist on stock Windows). Shared with runner_test.go.
func requireSleep(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skipf("sleep not on PATH: %v", err)
	}
}

// requireSh skips the calling test when there is no POSIX shell to fork a
// grandchild with — the process-group behaviour it checks is unix-only.
func requireSh(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh not on PATH: %v", err)
	}
}

// waitFor polls cond until it holds, or gives up after d.
func waitFor(cond func() bool, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// processAlive reports whether pid still exists: signal 0 is the standard
// liveness probe on unix (it validates the target without delivering
// anything).
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// goTask builds a task that runs the go tool.
func goTask(id string, args ...string) Task {
	return Task{ID: id, Name: id, Cmd: "go", Args: args}
}

// sleepTask builds a task that sleeps for the given number of seconds.
func sleepTask(id, seconds string) Task {
	return Task{ID: id, Name: id, Cmd: "sleep", Args: []string{seconds}}
}

// sink is a concurrency-safe Event collector: runCommand emits from both
// stream goroutines at once.
type sink struct {
	mu     sync.Mutex
	events []Event
}

func (s *sink) emit(ev Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
}

func (s *sink) all() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.events...)
}

// kind returns the Lines of every collected event of that kind.
func (s *sink) kind(want string) []string {
	var out []string
	for _, ev := range s.all() {
		if ev.Kind == want {
			out = append(out, ev.Line)
		}
	}
	return out
}

// --- streaming -------------------------------------------------------

// TestRunCommandStreamsStdout: `go env GOOS` produces exactly one start
// event carrying the command line, the value on stdout, a zero exit code
// and no error. runCommand never emits an exit event — that is the
// Runner's job — so the absence of one is asserted too.
func TestRunCommandStreamsStdout(t *testing.T) {
	requireGo(t)

	var s sink
	code, err := runCommand(context.Background(), goTask("env", "env", "GOOS"), s.emit)
	if err != nil {
		t.Fatalf("runCommand returned error: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}

	starts := s.kind(EventStart)
	if len(starts) != 1 {
		t.Fatalf("got %d start events, want 1: %v", len(starts), s.all())
	}
	if want := "go env GOOS"; starts[0] != want {
		t.Errorf("start line = %q, want %q", starts[0], want)
	}
	out := s.kind(EventStdout)
	if len(out) != 1 || out[0] != runtime.GOOS {
		t.Errorf("stdout = %v, want [%s]", out, runtime.GOOS)
	}
	if exits := s.kind(EventExit); len(exits) != 0 {
		t.Errorf("runCommand emitted exit events %v, want none", exits)
	}
	for _, ev := range s.all() {
		if ev.TaskID != "env" {
			t.Errorf("event %+v has wrong TaskID", ev)
		}
		if ev.At.IsZero() {
			t.Errorf("event %+v has no timestamp", ev)
		}
	}
}

// TestRunCommandFailureExitCodeAndStderr: a bogus help topic exits
// non-zero and complains on stderr. The exact status is toolchain
// business; what matters is that it is a real code (not the -1 "no exit
// status" placeholder) and that stderr was captured.
func TestRunCommandFailureExitCodeAndStderr(t *testing.T) {
	requireGo(t)

	var s sink
	code, err := runCommand(context.Background(),
		goTask("bogus", "help", "no-such-help-topic-xyz"), s.emit)
	if err == nil {
		t.Fatal("runCommand returned nil error for a failing command")
	}
	if code <= 0 {
		t.Errorf("exit code = %d, want a positive exit status", code)
	}
	errLines := strings.Join(s.kind(EventStderr), "\n")
	if !strings.Contains(errLines, "no-such-help-topic-xyz") {
		t.Errorf("stderr = %q, want it to mention the bogus topic", errLines)
	}
	if out := s.kind(EventStdout); len(out) != 0 {
		t.Errorf("stdout = %v, want none", out)
	}
}

// TestRunCommandStartFailure: an unknown program never starts, so there
// must be no start event and no exit status to report.
func TestRunCommandStartFailure(t *testing.T) {
	var s sink
	task := Task{ID: "ghost", Cmd: "silk-taskrunner-no-such-binary-xyz"}
	code, err := runCommand(context.Background(), task, s.emit)
	if err == nil {
		t.Fatal("runCommand returned nil error for an unknown binary")
	}
	if code != noExitCode {
		t.Errorf("exit code = %d, want %d", code, noExitCode)
	}
	if evs := s.all(); len(evs) != 0 {
		t.Errorf("emitted %v, want no events", evs)
	}
}

// --- cancellation ----------------------------------------------------

// TestRunCommandCancelReturnsPromptly: a long sleep is killed by
// cancelling the context, and runCommand comes back immediately instead
// of waiting out the sleep.
func TestRunCommandCancelReturnsPromptly(t *testing.T) {
	requireSleep(t)

	ctx, cancel := context.WithCancel(context.Background())
	var s sink
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	code, err := runCommand(ctx, sleepTask("sleep", "30"), s.emit)
	elapsed := time.Since(start)
	cancel()

	if elapsed > 5*time.Second {
		t.Fatalf("runCommand took %v after cancel, want a prompt return", elapsed)
	}
	if err == nil {
		t.Error("runCommand returned nil error for a killed process")
	}
	if runtime.GOOS != "windows" && code != noExitCode {
		t.Errorf("exit code = %d, want %d for a signalled process", code, noExitCode)
	}
	if starts := s.kind(EventStart); len(starts) != 1 {
		t.Errorf("got %d start events, want 1", len(starts))
	}
}

// TestRunCommandCancelKillsProcessGroup: the point of the setpgid dance —
// a process the command forked must die with it instead of surviving as an
// orphan (a `go build` leaves compilers behind, and one of them holding the
// output pipe would wedge the next run). The shell backgrounds a sleep and
// reports its PID on stdout; after cancellation that PID must be gone.
func TestRunCommandCancelKillsProcessGroup(t *testing.T) {
	requireSh(t)

	var s sink
	task := Task{ID: "tree", Cmd: "sh", Args: []string{"-c", "sleep 47 & echo $!; sleep 47"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		runCommand(ctx, task, s.emit)
	}()
	if !waitFor(func() bool { return len(s.kind(EventStdout)) > 0 }, 10*time.Second) {
		t.Fatalf("shell never reported the background pid: %v", s.all())
	}
	grandchild, err := strconv.Atoi(strings.TrimSpace(s.kind(EventStdout)[0]))
	if err != nil {
		t.Fatalf("parsing background pid %q: %v", s.kind(EventStdout)[0], err)
	}
	if !processAlive(grandchild) {
		t.Fatalf("grandchild %d is not running, nothing to kill", grandchild)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("runCommand did not return after cancel")
	}
	if !waitFor(func() bool { return !processAlive(grandchild) }, 5*time.Second) {
		t.Errorf("grandchild %d survived the group kill", grandchild)
	}
}

// --- process group ---------------------------------------------------

// TestEnableProcessGroup: on unix the child must be made a group leader
// so cancellation can kill the whole tree; elsewhere the helper has to
// admit it could not (best effort) rather than pretend.
func TestEnableProcessGroup(t *testing.T) {
	cmd := exec.Command("go", "env", "GOOS")
	got := enableProcessGroup(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil after enableProcessGroup")
	}
	want := runtime.GOOS != "windows"
	if got != want {
		t.Errorf("enableProcessGroup = %v on %s, want %v", got, runtime.GOOS, want)
	}
	// Idempotent: a second call must not clobber the attributes.
	if again := enableProcessGroup(cmd); again != got {
		t.Errorf("second enableProcessGroup = %v, want %v", again, got)
	}
}

// TestKillProcessNoProcess: Cancel is called with cmd.Process still nil
// when the command never started; os/exec expects ErrProcessDone for
// "nothing to kill".
func TestKillProcessNoProcess(t *testing.T) {
	if err := killProcess(nil, true); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("killProcess(nil) = %v, want os.ErrProcessDone", err)
	}
}

// --- environment -----------------------------------------------------

// TestCommandEnvNilInherits: no overrides means a nil Env, which makes
// os/exec pass the parent environment through untouched.
func TestCommandEnvNilInherits(t *testing.T) {
	if got := commandEnv(nil); got != nil {
		t.Errorf("commandEnv(nil) = %v, want nil", got)
	}
	if got := commandEnv(map[string]string{}); got != nil {
		t.Errorf("commandEnv(empty) = %v, want nil", got)
	}
}

// TestCommandEnvOverridesAppendedSorted: overrides land after the
// inherited environment (so they win) in sorted key order (so the slice
// is reproducible).
func TestCommandEnvOverridesAppendedSorted(t *testing.T) {
	got := commandEnv(map[string]string{"SILK_B": "2", "SILK_A": "1"})
	base := len(os.Environ())
	if len(got) != base+2 {
		t.Fatalf("len(commandEnv) = %d, want %d", len(got), base+2)
	}
	if got[base] != "SILK_A=1" || got[base+1] != "SILK_B=2" {
		t.Errorf("tail = %v, want [SILK_A=1 SILK_B=2]", got[base:])
	}
}

// TestRunCommandEnvOverrideWins: end-to-end proof that Task.Env reaches
// the child and overrides an inherited value — the go tool reports the
// GOFLAGS it was handed.
func TestRunCommandEnvOverrideWins(t *testing.T) {
	requireGo(t)

	task := goTask("flags", "env", "GOFLAGS")
	task.Env = map[string]string{"GOFLAGS": "-mod=mod"}
	var s sink
	if _, err := runCommand(context.Background(), task, s.emit); err != nil {
		t.Fatalf("runCommand returned error: %v", err)
	}
	if out := s.kind(EventStdout); len(out) != 1 || out[0] != "-mod=mod" {
		t.Errorf("stdout = %v, want [-mod=mod]", out)
	}
}

// TestRunCommandDir: the child runs in Task.Dir. Asked from inside the
// module the go tool reports this repo's go.mod; from a temp directory it
// reports no module at all.
func TestRunCommandDir(t *testing.T) {
	requireGo(t)

	inside := goTask("inside", "env", "GOMOD")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	inside.Dir = wd
	var here sink
	if _, err := runCommand(context.Background(), inside, here.emit); err != nil {
		t.Fatalf("runCommand(inside module) returned error: %v", err)
	}
	got := here.kind(EventStdout)
	if len(got) != 1 || !strings.HasSuffix(got[0], "go.mod") {
		t.Fatalf("GOMOD inside the module = %v, want a path ending in go.mod", got)
	}

	outside := goTask("outside", "env", "GOMOD")
	outside.Dir = t.TempDir()
	var away sink
	if _, err := runCommand(context.Background(), outside, away.emit); err != nil {
		t.Fatalf("runCommand(outside module) returned error: %v", err)
	}
	if elsewhere := away.kind(EventStdout); len(elsewhere) == 1 &&
		strings.HasSuffix(elsewhere[0], "go.mod") {
		t.Errorf("GOMOD outside the module = %v, Dir was ignored", elsewhere)
	}
}

// --- exit codes ------------------------------------------------------

// TestExitCode: a nil error is a clean exit; anything that is not an
// *exec.ExitError carries no status.
func TestExitCode(t *testing.T) {
	if got := exitCode(nil); got != 0 {
		t.Errorf("exitCode(nil) = %d, want 0", got)
	}
	if got := exitCode(errors.New("boom")); got != noExitCode {
		t.Errorf("exitCode(plain error) = %d, want %d", got, noExitCode)
	}
}
