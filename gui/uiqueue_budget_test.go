package gui

import (
	"reflect"
	"testing"
	"time"
)

func TestUIQueueYieldsBeforeDrainingAnEntireBurst(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	wakes := 0
	SetUIWakeup(func() { wakes++ })
	ran := 0
	for range 1000 {
		Post(func() { ran++ })
	}
	drainUITasks()
	if ran == 0 || ran > 64 {
		t.Fatalf("one turn ran %d tasks; input/paint must get a turn after at most 64", ran)
	}
	if wakes != 2 {
		t.Fatalf("remaining work was not rescheduled: wakes=%d", wakes)
	}
	for ran < 1000 {
		drainUITasks()
	}
	if ran != 1000 {
		t.Fatalf("lost/repeated callbacks: %d", ran)
	}
}

func TestUIQueueSlowTaskYieldsAndKeepsRemainderAheadOfNewPosts(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	var order []int
	Post(func() {
		order = append(order, 0)
		Post(func() { order = append(order, 2) })
		time.Sleep(6 * time.Millisecond)
	})
	Post(func() { order = append(order, 1) })
	drainUITasks()
	if !reflect.DeepEqual(order, []int{0}) {
		t.Fatalf("slow callback did not yield: %v", order)
	}
	drainUITasks()
	if !reflect.DeepEqual(order, []int{0, 1, 2}) {
		t.Fatalf("yield broke FIFO: %v", order)
	}
}
