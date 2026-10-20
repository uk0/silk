package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Use this test binary as the child so Windows exercises exactly the same
// dual-pipe concurrency and shutdown contract, without a Unix shell.
func TestProgressChild(t *testing.T) {
	if os.Getenv("SILK_PROGRESS_CHILD") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "first")
	marker := os.Getenv("SILK_PROGRESS_MARKER")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(streamThrottle + 50*time.Millisecond)
	fmt.Fprintln(os.Stderr, "second")
}

func TestStreamCommandSerializesProgress(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "progress")
	cmd := exec.Command(os.Args[0], "-test.run=^TestProgressChild$")
	cmd.Env = append(os.Environ(), "SILK_PROGRESS_CHILD=1", "SILK_PROGRESS_MARKER="+marker, "GORACE=atexit_sleep_ms=0")
	var active, overlap atomic.Int32
	var calls atomic.Int32
	var seen []string // A serialized callback may own ordinary, non-atomic state.
	text, err := streamCommand(cmd, func(s string) {
		if active.Add(1) != 1 {
			overlap.Add(1)
		}
		defer active.Add(-1)
		if calls.Add(1) == 1 {
			if err := os.WriteFile(marker, nil, 0600); err != nil {
				t.Error(err)
			}
			time.Sleep(3 * streamThrottle)
		}
		seen = append(seen, s)
	})
	if err != nil {
		t.Fatal(err)
	}
	if overlap.Load() != 0 {
		t.Fatalf("concurrent progress callbacks: %d", overlap.Load())
	}
	if len(seen) < 2 || seen[len(seen)-1] != text {
		t.Fatalf("missing final progress: %v", seen)
	}
	for i := 1; i < len(seen); i++ {
		if !strings.HasPrefix(seen[i], seen[i-1]) {
			t.Fatalf("progress regressed from %q to %q", seen[i-1], seen[i])
		}
	}
}

func TestRunWithProgressJoinsHeartbeatBeforeReturn(t *testing.T) {
	// The child exits while the first one-second heartbeat is still reporting.
	cmd := exec.Command(os.Args[0], "-test.run=^TestProgressChild$")
	marker := filepath.Join(t.TempDir(), "progress")
	cmd.Env = append(os.Environ(), "SILK_PROGRESS_CHILD=1", "SILK_PROGRESS_MARKER="+marker, "GORACE=atexit_sleep_ms=0")
	var active, overlaps atomic.Int32
	_, err := runWithProgress(cmd, "build", func(s string) {
		if active.Add(1) != 1 {
			overlaps.Add(1)
		}
		defer active.Add(-1)
		if strings.Contains(s, "... running") {
			_ = os.WriteFile(marker, nil, 0600)
			time.Sleep(600 * time.Millisecond)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if active.Load() != 0 {
		t.Error("report callback still running after command returned")
	}
	if overlaps.Load() != 0 {
		t.Errorf("heartbeat/output callbacks overlapped %d times", overlaps.Load())
	}
	// Ensure a failed pre-fix test does not leak its deliberately delayed callback.
	deadline := time.Now().Add(2 * time.Second)
	for active.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
}
