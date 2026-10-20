//go:build windows

package taskrunner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Reuse this test executable: no sh/sleep, MSYS PID namespace or signal(0).
func TestTaskProcessHelper(t *testing.T) {
	mode := os.Getenv("SILK_TASK_PROCESS_HELPER")
	if mode == "" {
		return
	}
	if mode == "leaf" {
		time.Sleep(2 * time.Minute)
		os.Exit(0)
	}
	if mode != "parent" && mode != "orphan-parent" {
		os.Exit(2)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestTaskProcessHelper$")
	child.Env = append(os.Environ(), "SILK_TASK_PROCESS_HELPER=leaf", "GORACE=atexit_sleep_ms=0")
	// Inherit the pipes too: an orphan would keep the runner's stream reads
	// blocked even after its direct parent has been killed.
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Printf("child:%d\nparent:%d\n", child.Process.Pid, os.Getpid())
	if mode == "orphan-parent" {
		// Let the test retain a native parent handle before deliberately
		// exiting, avoiding sleeps/PID reuse as a liveness assertion.
		if !waitFor(func() bool {
			_, err := os.Stat(os.Getenv("SILK_TASK_EXIT_HANDSHAKE"))
			return err == nil
		}, 15*time.Second) {
			os.Exit(2)
		}
		os.Exit(0)
	}
	_ = child.Wait()
	os.Exit(0)
}

func TestRunCommandCancelKillsWindowsTreeButNotSibling(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// A different invocation of the very same executable must survive.
	sibling := exec.Command(exe, "-test.run=^TestTaskProcessHelper$")
	sibling.Env = append(os.Environ(), "SILK_TASK_PROCESS_HELPER=leaf", "GORACE=atexit_sleep_ms=0")
	sibling.Stdout, sibling.Stderr = io.Discard, io.Discard
	if err := sibling.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sibling.Process.Kill(); _ = sibling.Wait() })
	open := func(pid int) windows.Handle {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = windows.CloseHandle(h) })
		return h
	}
	alive := func(h windows.Handle) bool {
		var code uint32
		if err := windows.GetExitCodeProcess(h, &code); err != nil {
			t.Fatal(err)
		}
		return code == 259 // STILL_ACTIVE
	}
	siblingHandle := open(sibling.Process.Pid)
	ctx, cancel := context.WithCancel(context.Background())
	var s sink
	task := Task{ID: "native-tree", Cmd: exe, Args: []string{"-test.run=^TestTaskProcessHelper$"}, Env: map[string]string{"SILK_TASK_PROCESS_HELPER": "parent", "GORACE": "atexit_sleep_ms=0"}}
	done := make(chan struct{})
	var code int
	var runErr error
	go func() {
		defer close(done)
		code, runErr = runCommand(ctx, task, s.emit)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("runner did not finish during cleanup")
		}
	})
	if !waitFor(func() bool { return len(s.kind(EventStdout)) != 0 }, 15*time.Second) {
		t.Fatalf("native parent did not report child: %v", s.all())
	}
	line := s.kind(EventStdout)[0]
	pid, err := strconv.Atoi(strings.TrimPrefix(line, "child:"))
	if err != nil || !strings.HasPrefix(line, "child:") {
		t.Fatalf("invalid native PID: %q (%v)", line, err)
	}
	childHandle := open(pid)
	t.Cleanup(func() {
		// Cleanup only this retained process object, even if an assertion fails.
		if alive(childHandle) {
			_ = windows.TerminateProcess(childHandle, 1)
			_, _ = windows.WaitForSingleObject(childHandle, 5000)
		}
	})
	if !alive(childHandle) || !alive(siblingHandle) {
		t.Fatal("native child/sibling not alive before cancellation")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("cancelled task retained its output pipes")
	}
	if code == 0 || runErr == nil {
		t.Fatalf("cancelled task reported success: code=%d err=%v", code, runErr)
	}
	if result, err := windows.WaitForSingleObject(childHandle, 5000); err != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatalf("descendant survived cancellation: wait=%d err=%v", result, err)
	}
	if !alive(siblingHandle) {
		t.Fatal("cancellation killed an unrelated sibling with the same executable name")
	}
}

// A parent that exits before cancellation is no longer a taskkill target.
// Its descendant must not keep the runner blocked on inherited pipes. This
// is a bounded-read test, not a claim that detached children are job-managed.
func TestRunCommandCancelBoundsOrphanedWindowsPipes(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	handshake := t.TempDir() + "/parent-exit"
	ctx, cancel := context.WithCancel(context.Background())
	var s sink
	done := make(chan struct{})
	var runErr error
	go func() {
		defer close(done)
		_, runErr = runCommand(ctx, Task{ID: "orphan-pipes", Cmd: exe, Args: []string{"-test.run=^TestTaskProcessHelper$"}, Env: map[string]string{"SILK_TASK_PROCESS_HELPER": "orphan-parent", "SILK_TASK_EXIT_HANDSHAKE": handshake, "GORACE": "atexit_sleep_ms=0"}}, s.emit)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("runner cleanup did not finish")
		}
	})
	if !waitFor(func() bool { return len(s.kind(EventStdout)) >= 2 }, 15*time.Second) {
		t.Fatalf("no descendant PID: %v", s.all())
	}
	pid, err := strconv.Atoi(strings.TrimPrefix(s.kind(EventStdout)[0], "child:"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = windows.TerminateProcess(h, 1)
		_, _ = windows.WaitForSingleObject(h, 5000)
		_ = windows.CloseHandle(h)
	})
	parentPID, err := strconv.Atoi(strings.TrimPrefix(s.kind(EventStdout)[1], "parent:"))
	if err != nil {
		t.Fatal(err)
	}
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(parentPID))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(parent) })
	if err := os.WriteFile(handshake, []byte("exit"), 0600); err != nil {
		t.Fatal(err)
	}
	if result, err := windows.WaitForSingleObject(parent, 5000); err != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatalf("parent did not exit before cancellation: wait=%d err=%v", result, err)
	}
	select {
	case <-done:
		t.Fatal("test did not retain inherited output pipes")
	default:
	}
	cancel()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("orphaned stdout/stderr wedged cancellation")
	}
	if runErr == nil {
		t.Fatal("cancelled read reported success")
	}
}
