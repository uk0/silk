package gui

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/uk0/silk/core"
)

// resetUIQueue clears global queue state so tests don't bleed into each other
// (and so a headless run after these tests starts clean).
func resetUIQueue() {
	uiTaskMu.Lock()
	uiTasks = nil
	uiWakeupFn = nil
	uiTaskMu.Unlock()
}

func TestUIQueuePostThenDrainRuns(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)

	ran := false
	Post(func() { ran = true })
	drainUITasks()
	if !ran {
		t.Fatal("task did not run after drainUITasks")
	}

	uiTaskMu.Lock()
	n := len(uiTasks)
	uiTaskMu.Unlock()
	if n != 0 {
		t.Fatalf("queue not empty after drain: %d task(s) remain", n)
	}
}

func TestUIQueueFIFOOrder(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)

	var order []int
	for i := 0; i < 5; i++ {
		i := i
		Post(func() { order = append(order, i) })
	}
	drainUITasks()

	if len(order) != 5 {
		t.Fatalf("expected 5 executions, got %d", len(order))
	}
	for i := 0; i < 5; i++ {
		if order[i] != i {
			t.Fatalf("FIFO violated: order=%v", order)
		}
	}
}

func TestUIQueuePostNilNoop(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)

	Post(nil)
	uiTaskMu.Lock()
	n := len(uiTasks)
	uiTaskMu.Unlock()
	if n != 0 {
		t.Fatalf("nil Post enqueued something: %d", n)
	}
	// Draining an empty queue must be safe.
	drainUITasks()
}

func TestUIQueueReentrancyRunsOnNextDrain(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)

	outer := false
	inner := false
	Post(func() {
		outer = true
		// Re-entrant Post: must not deadlock, and must NOT run in this batch.
		Post(func() { inner = true })
	})

	drainUITasks()
	if !outer {
		t.Fatal("outer task did not run")
	}
	if inner {
		t.Fatal("re-posted task ran in the current batch; expected next drain")
	}

	// The re-posted task is queued; the next drain runs it.
	drainUITasks()
	if !inner {
		t.Fatal("re-posted task did not run on the next drain")
	}
}

func TestUIQueuePanicIsolation(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)

	first := false
	third := false
	Post(func() { first = true })
	Post(func() { panic("boom") })
	Post(func() { third = true })

	// A panicking task must not propagate out of drainUITasks...
	drainUITasks()

	// ...and must not stop later tasks in the same batch.
	if !first {
		t.Fatal("task before the panicking one did not run")
	}
	if !third {
		t.Fatal("task after the panicking one did not run")
	}
}

// TestUIQueuePanicIsReported covers the half of the contract
// TestUIQueuePanicIsolation cannot see. Surviving the panic was all the recover
// did: the value went straight into the bin. Everything a background goroutine
// hands the GUI arrives through Post — debugger locals, git refreshes, LSP
// results, toasts — so a panic in one of them showed up as a menu item that
// did nothing, forever, with no line in the log saying the task had even run.
func TestUIQueuePanicIsReported(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)

	var mu sync.Mutex
	var warnings []string
	unregister := core.RegisterLogSink(func(level core.LogLevel, msg string) {
		if level != core.LevelWarn {
			return
		}
		mu.Lock()
		warnings = append(warnings, msg)
		mu.Unlock()
	})
	defer unregister()

	Post(panickingTaskForTest)
	drainUITasks()

	mu.Lock()
	defer mu.Unlock()
	var reported string
	for _, w := range warnings {
		if strings.Contains(w, "boom-from-a-posted-task") {
			reported = w
			break
		}
	}
	if reported == "" {
		t.Fatalf("the panic was swallowed without a report; warnings seen: %v", warnings)
	}
	// Assert the FAILING TASK's own frame, not the recover site's.
	//
	// "runUITask" was the earlier assertion, and it is present by construction:
	// debug.Stack() taken inside runUITask's deferred recover always contains
	// the frame it is running in. So the check passed whether or not any frame
	// from the task survived — truncate the stack at the panic boundary, or
	// stash the value and report it later from another goroutine, and the test
	// stayed green while the report became useless. The point of the stack is
	// naming which posted task died.
	if !strings.Contains(reported, "panickingTaskForTest") {
		t.Errorf("the report does not name the failing task, so it cannot be located:\n%s", reported)
	}
}

// panickingTaskForTest is a named function so its frame appears in the stack.
// A closure would show up as TestUIQueuePanicIsReported.funcN, which names the
// test rather than the task and would make the assertion above meaningless.
func panickingTaskForTest() {
	panic("boom-from-a-posted-task")
}

func TestUIQueueWakeupHookInvoked(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)

	var woke int32
	SetUIWakeup(func() { atomic.AddInt32(&woke, 1) })

	Post(func() {})
	if atomic.LoadInt32(&woke) != 1 {
		t.Fatalf("wakeup hook not invoked exactly once: got %d", woke)
	}

	// drainUITasks must not invoke the wakeup hook.
	drainUITasks()
	if atomic.LoadInt32(&woke) != 1 {
		t.Fatalf("drainUITasks unexpectedly invoked wakeup: count=%d", woke)
	}
}

func TestUIQueueConcurrentPosts(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)

	const goroutines = 16
	const perGoroutine = 50
	const want = goroutines * perGoroutine

	var executed int64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				Post(func() { atomic.AddInt64(&executed, 1) })
			}
		}()
	}

	// Drain in a loop (interleaved with the producers) until we've collected
	// every execution. Producers finishing is tracked separately; we keep
	// draining until the count matches so no late Post is missed.
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	producersDone := false
	for {
		drainUITasks()
		if atomic.LoadInt64(&executed) == want {
			break
		}
		if producersDone {
			// Producers are done but count not yet reached: one more drain
			// pass on the next loop iteration will sweep the remainder.
			continue
		}
		select {
		case <-done:
			producersDone = true
		default:
		}
	}

	if got := atomic.LoadInt64(&executed); got != want {
		t.Fatalf("expected %d executions, got %d", want, got)
	}
}
