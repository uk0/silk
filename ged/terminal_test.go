package ged

import (
	"sync"
	"testing"
	"time"
)

// TestTerminalPanelRunDispatchesCommand: TerminalPanel.Run("...")
// fires the SigSubmit observer with the same string, proving the
// programmatic dispatch reuses the user-typed submit path. silkide's
// "Run" toolbar button depends on this contract — without it, the
// button's call falls into a private path and the user sees nothing.
func TestTerminalPanelRunDispatchesCommand(t *testing.T) {
	tp := NewTerminalPanel()

	var (
		mu    sync.Mutex
		got   string
		fired bool
	)
	tp.SigSubmit(func(cmd string) {
		mu.Lock()
		got = cmd
		fired = true
		mu.Unlock()
	})

	tp.Run("echo hello")

	// SigSubmit fires synchronously inside submitCommand, so a single
	// Wait isn't required. Defensive deadline anyway in case the
	// terminal ever goes async.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		if fired {
			mu.Unlock()
			break
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if !fired {
		t.Fatal("SigSubmit didn't fire after Run()")
	}
	if got != "echo hello" {
		t.Errorf("Submitted cmd = %q, want %q", got, "echo hello")
	}
}

// TestTerminalPanelRunIgnoresEmpty: Run("") shouldn't fire SigSubmit
// (matches the user-typed Enter on empty input — submitCommand
// short-circuits before the callback).
func TestTerminalPanelRunIgnoresEmpty(t *testing.T) {
	tp := NewTerminalPanel()
	var called bool
	tp.SigSubmit(func(string) { called = true })
	tp.Run("")
	if called {
		t.Errorf("SigSubmit fired for empty Run()")
	}
}

// TestTerminalPanelRunWithEnvReadsRunningLocked: RunWithEnv's "a command is
// already in flight" early-out must read the running flag under mu.
// submitCommand sets that flag and runWorker clears it from the worker
// goroutine, both under mu, so an unlocked read races the worker — and a
// -race build turns that into a WARNING: DATA RACE that fails the whole ged
// package, not just this call. The writer loop below stands in for those two;
// keeping the flag set means RunWithEnv always takes the early-out and no
// shell is ever spawned.
func TestTerminalPanelRunWithEnvReadsRunningLocked(t *testing.T) {
	tp := NewTerminalPanel()

	tp.mu.Lock()
	tp.running = true
	tp.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			tp.mu.Lock()
			tp.running = true
			tp.mu.Unlock()
		}
	}()

	var called bool
	tp.SigSubmit(func(string) { called = true })
	for i := 0; i < 500; i++ {
		tp.RunWithEnv("echo must-not-run", nil)
	}
	<-done

	if called {
		t.Error("RunWithEnv submitted a command while one was already running")
	}
}
