package gui

import (
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestUIQueueBurstCoalescesWakeupsWithoutDroppingTasks(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	var wakes atomic.Int32
	SetUIWakeup(func() { wakes.Add(1) })
	var order []int
	for i := range 200 {
		Post(func() { order = append(order, i) })
	}
	if got := wakes.Load(); got != 1 {
		t.Fatalf("200 pending tasks woke native loop %d times", got)
	}
	for len(order) < 200 {
		drainUITasks()
	}
	if len(order) != 200 {
		t.Fatalf("coalescing dropped tasks: %d", len(order))
	}
	for i, got := range order {
		if got != i {
			t.Fatalf("FIFO order[%d]=%d", i, got)
		}
	}
	wakesBefore := wakes.Load()
	Post(func() { order = append(order, 200) })
	if wakes.Load() != wakesBefore+1 {
		t.Fatal("next batch did not wake a parked loop")
	}
	drainUITasks()
}
func TestUIQueueInstallingWakeupForAlreadyPendingTasks(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	ran := false
	wakes := 0
	Post(func() { ran = true }) // initialization/headless -> native transition
	SetUIWakeup(func() { wakes++ })
	if wakes != 1 || ran {
		t.Fatalf("installation wakes=%d ran=%v", wakes, ran)
	}
	SetUIWakeup(nil)
	Post(func() {})
	SetUIWakeup(func() { wakes++ })
	if wakes != 2 {
		t.Fatal("reinstalled native hook lost an already non-empty queue")
	}
	drainUITasks()
	if !ran {
		t.Fatal("pre-hook task never ran")
	}
	SetUIWakeup(func() { wakes++ })
	if wakes != 2 {
		t.Fatal("empty hook installation spuriously wakes native loop")
	}
}
func TestUIQueueReentrantPostWakesNextBatchAndCanDrainNested(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	wakes := 0
	var order []string
	SetUIWakeup(func() { wakes++ })
	Post(func() {
		order = append(order, "outer")
		Post(func() { order = append(order, "nested") })
		if wakes != 2 {
			t.Fatal("Post during the drain snapshot did not wake nested modal")
		}
		drainUITasks() // nested modal services only newly queued tasks
		order = append(order, "return")
	})
	Post(func() { order = append(order, "outer-tail") })
	drainUITasks()
	if want := []string{"outer", "nested", "return", "outer-tail"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("nested FIFO=%v", order)
	}
}
func TestUIQueueConcurrentBurstHasOneWakeup(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	var wakes atomic.Int32
	var ran int
	SetUIWakeup(func() { wakes.Add(1) })
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 40 {
				Post(func() { ran++ })
			}
		}()
	}
	wg.Wait()
	if got := wakes.Load(); got != 1 {
		t.Fatalf("burst wakes=%d", got)
	}
	for ran < 640 {
		drainUITasks()
	}
	if ran != 640 {
		t.Fatalf("executed=%d", ran)
	}
}
func TestUIQueueWakeHookIsCalledOutsideQueueLock(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	ran := false
	Post(func() { ran = true })
	// A test backend can service synchronously. Holding uiTaskMu while waking
	// deadlocks both this installation path and Post's empty->pending path.
	SetUIWakeup(drainUITasks)
	if !ran {
		t.Fatal("install did not service pending work")
	}
	ran = false
	Post(func() { ran = true })
	if !ran {
		t.Fatal("Post did not service work")
	}
}
