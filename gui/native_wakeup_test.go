package gui

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativeWakeupStopJoinsInFlightCall(t *testing.T) {
	entered, release, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	w := &nativeWakeup{fn: func() { calls.Add(1); close(entered); <-release }}
	done := make(chan struct{})
	go func() { w.wake(); close(done) }()
	<-entered
	go func() { w.stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Error("backend teardown did not wait for the in-flight native wake")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-done
	<-stopped
	// A producer can retain the old callback even after SetUIWakeup(nil).
	w.wake()
	if calls.Load() != 1 {
		t.Fatalf("retained wake entered retired backend: %d", calls.Load())
	}
}

func TestNativeWakeupConcurrentPostAndRetirement(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	var retired, invalid atomic.Bool
	w := &nativeWakeup{fn: func() {
		if retired.Load() {
			invalid.Store(true)
		}
	}}
	SetUIWakeup(w.wake)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 200 {
				Post(func() {})
				w.wake()
			}
		})
	}
	SetUIWakeup(nil)
	w.stop()
	retired.Store(true)
	wg.Wait()
	if invalid.Load() {
		t.Fatal("native call overlapped or followed destruction")
	}
}
