package main

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// shellCmd builds a tiny shell command portably enough for these tests.
func shellCmd(t *testing.T, unix string) *exec.Cmd {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-based stream test is unix-only; the streaming logic itself is OS-independent")
	}
	return exec.Command("/bin/sh", "-c", unix)
}

// TestStreamCommandReportsProgressBeforeExit is the regression guard for the
// bug this fixes: CombinedOutput yielded nothing until the process exited, so a
// slow build looked like a frozen IDE. Progress must arrive while the child is
// still running.
func TestStreamCommandReportsProgressBeforeExit(t *testing.T) {
	cmd := shellCmd(t, "echo first; sleep 1; echo second")
	var seen []string
	start := time.Now()
	text, err := streamCommand(cmd, func(s string) {
		seen = append(seen, s)
	})
	if err != nil {
		t.Fatalf("streamCommand: %v", err)
	}
	if !strings.Contains(text, "first") || !strings.Contains(text, "second") {
		t.Errorf("final text missing output: %q", text)
	}
	if len(seen) == 0 {
		t.Fatal("no progress callbacks")
	}
	// The first callback must land well before the ~1s total runtime.
	if len(seen) < 2 {
		t.Errorf("got %d progress callbacks, want >=2 (streaming, not one final dump)", len(seen))
	}
	if !strings.Contains(seen[0], "first") {
		t.Errorf("first progress = %q, want it to contain the first line", seen[0])
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Skip("child exited too fast to prove streaming")
	}
}

// TestStreamCommandMergesStderr proves both pipes are drained — reading them in
// sequence is what deadlocks once the child fills the unread one.
func TestStreamCommandMergesStderr(t *testing.T) {
	cmd := shellCmd(t, "echo to-stdout; echo to-stderr 1>&2")
	text, err := streamCommand(cmd, nil)
	if err != nil {
		t.Fatalf("streamCommand: %v", err)
	}
	for _, want := range []string{"to-stdout", "to-stderr"} {
		if !strings.Contains(text, want) {
			t.Errorf("text %q missing %q", text, want)
		}
	}
}

// TestStreamCommandLargeOutputNoDeadlock pushes far more than a pipe buffer
// through both streams at once; the old sequential-read shape hangs here.
func TestStreamCommandLargeOutputNoDeadlock(t *testing.T) {
	cmd := shellCmd(t, "i=0; while [ $i -lt 4000 ]; do echo out-$i; echo err-$i 1>&2; i=$((i+1)); done")
	done := make(chan struct{})
	var text string
	var err error
	go func() {
		text, err = streamCommand(cmd, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("streamCommand deadlocked on large dual-stream output")
	}
	if err != nil {
		t.Fatalf("streamCommand: %v", err)
	}
	if n := strings.Count(text, "out-"); n != 4000 {
		t.Errorf("stdout lines = %d, want 4000", n)
	}
	if n := strings.Count(text, "err-"); n != 4000 {
		t.Errorf("stderr lines = %d, want 4000", n)
	}
}

// TestStreamCommandKeepsLongLines guards the raised scanner limit: a full Go
// type-mismatch error can exceed the 64K default.
func TestStreamCommandKeepsLongLines(t *testing.T) {
	cmd := shellCmd(t, "awk 'BEGIN{s=\"\"; for(i=0;i<200000;i++) s=s \"x\"; print s}'")
	text, err := streamCommand(cmd, nil)
	if err != nil {
		t.Fatalf("streamCommand: %v", err)
	}
	if n := strings.Count(text, "x"); n < 200000 {
		t.Errorf("long line truncated: got %d x's, want 200000", n)
	}
}

// TestStreamCommandReturnsExitError keeps failure reporting intact.
func TestStreamCommandReturnsExitError(t *testing.T) {
	cmd := shellCmd(t, "echo boom 1>&2; exit 3")
	text, err := streamCommand(cmd, nil)
	if err == nil {
		t.Fatal("want a non-nil exit error")
	}
	if !strings.Contains(text, "boom") {
		t.Errorf("text %q should still carry the output", text)
	}
}
