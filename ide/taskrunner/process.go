package taskrunner

// One child process: spawn it, stream stdout/stderr line by line, and on
// cancellation kill the whole process GROUP rather than just the direct
// child. Everything here is stdlib os/exec + context; the Runner in
// runner.go supplies the scheduling around it.

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// noExitCode is Event.Code / Record.Code when there is no exit status
	// to report: the process was killed by a signal, never started, or
	// the task was skipped.
	noExitCode = -1

	// killGrace bounds how long a cancelled command may keep Wait
	// blocked. It is handed to exec.Cmd.WaitDelay, so after the context
	// is cancelled and the group has been killed, os/exec force-closes
	// the pipes once the grace elapses — a stray descendant that
	// inherited stdout cannot wedge the runner.
	killGrace = 2 * time.Second
)

// runCommand runs t as a child process and streams its output through
// emit: one EventStart with the rendered command line, then one
// EventStdout / EventStderr per output line. It blocks until the process
// has exited and both streams are drained, and returns the exit code
// (noExitCode when the process was signalled or never started) plus the
// error from Wait.
//
// runCommand deliberately does NOT emit EventExit: the Runner owns that
// event because it must also be emitted for tasks that never spawn a
// process (cancelled while queued, skipped after a failed dependency).
//
// Cancelling ctx kills the process group and makes the call return
// promptly; the returned error is then the Wait error of the killed
// process, and the caller distinguishes "cancelled" from "failed" by
// inspecting ctx.Err().
func runCommand(ctx context.Context, t Task, emit func(Event)) (int, error) {
	cmd := exec.CommandContext(ctx, t.Cmd, t.Args...)
	cmd.Dir = t.Dir
	cmd.Env = commandEnv(t.Env)

	// Kill the group, not the child: `go build` and `go test` fork
	// compilers and test binaries that would otherwise survive a
	// cancelled build and keep holding the output pipes.
	group := enableProcessGroup(cmd)
	cmd.Cancel = func() error { return killProcess(cmd.Process, group) }
	cmd.WaitDelay = killGrace

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return noExitCode, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return noExitCode, err
	}
	if err := cmd.Start(); err != nil {
		return noExitCode, err
	}
	emit(Event{TaskID: t.ID, Kind: EventStart, Line: t.CommandLine(), At: time.Now()})

	var wg sync.WaitGroup
	wg.Add(2)
	go streamLines(&wg, stdout, t.ID, EventStdout, emit)
	go streamLines(&wg, stderr, t.ID, EventStderr, emit)
	// Both pipes must be fully read before Wait: Wait closes the parent
	// ends. On cancellation the group kill (or WaitDelay) closes them for
	// us, so this cannot outlive the process.
	wg.Wait()

	waitErr := cmd.Wait()
	return exitCode(waitErr), waitErr
}

// streamLines emits one Event per line read from r. Lines are delivered
// without their trailing newline (and without a CRLF carriage return, so
// Windows tool output reads the same as unix). A final chunk with no
// newline is still emitted. Read errors end the stream silently: EOF is
// the normal case, and "file already closed" is the expected outcome of a
// cancelled run.
func streamLines(wg *sync.WaitGroup, r io.Reader, taskID, kind string, emit func(Event)) {
	defer wg.Done()
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			emit(Event{TaskID: taskID, Kind: kind, Line: line, At: time.Now()})
		}
		if err != nil {
			return
		}
	}
}

// commandEnv builds the child environment: the IDE's own environment plus
// the task's overrides. A nil/empty override map returns nil, which makes
// os/exec inherit the parent environment verbatim. Overrides are appended
// in sorted key order — os/exec keeps the last occurrence of a duplicate
// key, so an override always wins over the inherited value, and the
// resulting slice is the same for the same map.
func commandEnv(over map[string]string) []string {
	if len(over) == 0 {
		return nil
	}
	keys := make([]string, 0, len(over))
	for k := range over {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	env := os.Environ()
	for _, k := range keys {
		env = append(env, k+"="+over[k])
	}
	return env
}

// exitCode extracts the process exit status from a Wait error: 0 for a
// clean exit, the reported status for a non-zero exit, and noExitCode
// when the process was killed by a signal or failed to start.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if code := exit.ExitCode(); code >= 0 {
			return code
		}
	}
	return noExitCode
}

// enableProcessGroup asks the OS to start the child in a NEW process
// group so that killProcess can take down the whole tree — the child plus
// every process it spawns — in one signal.
//
// syscall.SysProcAttr is platform-specific and only carries a Setpgid
// field on unix, so the field is set through reflection. That keeps this
// file compiling for every GOOS without per-platform variants, and it
// reports what actually happened: true when the child will be a group
// leader, false on platforms without the field (notably Windows), where
// cancellation degrades to a best-effort kill of the direct child and
// descendants may survive it.
func enableProcessGroup(cmd *exec.Cmd) bool {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	field := reflect.ValueOf(cmd.SysProcAttr).Elem().FieldByName("Setpgid")
	if !field.IsValid() || field.Kind() != reflect.Bool || !field.CanSet() {
		return false
	}
	field.SetBool(true)
	return true
}

// killProcess terminates p. When the child was started as a group leader
// (group is what enableProcessGroup reported) the kill is addressed to
// -pid, i.e. to every member of that group, so orphaned grandchildren die
// with it. Otherwise — or if the group kill fails, e.g. because the
// leader is already gone — it falls back to killing the direct child.
//
// It returns os.ErrProcessDone when there is no process to kill, which is
// the value exec.Cmd.Cancel uses to mean "the command already finished".
func killProcess(p *os.Process, group bool) error {
	if p == nil {
		return os.ErrProcessDone
	}
	if group {
		if pgroup, err := os.FindProcess(-p.Pid); err == nil {
			if err := pgroup.Signal(os.Kill); err == nil {
				return nil
			}
		}
	}
	return p.Kill()
}
