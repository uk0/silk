package gui

import (
	"sync"
	"testing"
	"time"

	"github.com/uk0/silk/core"
)

func isolateTagBridge(t *testing.T) {
	t.Helper()
	reset := func() {
		uiTaskMu.Lock()
		uiTasks = nil
		uiWakeupFn = nil
		uiTaskMu.Unlock()
		animManager.animations = nil
		animManager.groups = nil
	}
	reset()
	t.Cleanup(reset)
}
func TestAnimatedBindingCoalescesBurstAndCancelsPending(t *testing.T) {
	isolateTagBridge(t)
	tag := core.NewTagDB().GetOrCreate("flow", core.Meta{})
	calls := 0
	stop := BindTagAnimated(tag, func(float64) { calls++ }, time.Second)
	for i := 0; i < 1000; i++ {
		tag.SetValue(float64(i))
	}
	uiTaskMu.Lock()
	queued := len(uiTasks)
	uiTaskMu.Unlock()
	if queued > 1 {
		t.Errorf("burst posted %d UI callbacks, expected <= 1", queued)
	}
	stop()
	drainUITasks()
	AnimationTick()
	if calls != 0 || HasActiveAnimations() {
		t.Fatal("cancelled binding still updates or animates")
	}
}
func TestAnimatedBindingCancelStopsRunningAndConcurrentPublish(t *testing.T) {
	isolateTagBridge(t)
	tag := core.NewTagDB().GetOrCreate("flow", core.Meta{})
	calls := 0
	stop := BindTagAnimated(tag, func(float64) { calls++ }, time.Second)
	tag.SetValue(42.0)
	drainUITasks()
	AnimationTick()
	before := calls
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			tag.SetValue(float64(i))
		}
	}()
	stop()
	wg.Wait()
	for i := 0; i < 20; i++ {
		drainUITasks()
		AnimationTick()
	}
	if calls != before || HasActiveAnimations() {
		t.Fatal("callbacks continued after unsubscribe")
	}
}

func TestTagUIBurstDeliversLatestAndUnbindDiscardsQueuedValue(t *testing.T) {
	isolateTagBridge(t)
	tag := core.NewTagDB().GetOrCreate("latest", core.Meta{})
	var values []float64
	stop := BindTagUI(tag, func(v core.Value) { values = append(values, v.Float()) })
	for i := 0; i < 1000; i++ {
		tag.SetValue(float64(i))
	}
	drainUITasks()
	if len(values) != 1 || values[0] != 999 {
		t.Fatal("burst did not deliver latest sample exactly once", values)
	}
	tag.SetValue(1000.0)
	stop()
	stop()
	drainUITasks()
	if len(values) != 1 {
		t.Fatal("cancelled callback escaped", values)
	}
}
