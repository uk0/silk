package main

import (
	"github.com/uk0/silk/gui"
	"runtime"
	"testing"
)

func TestTimerDemoRestartOnlyCommitsLatestAndCancelIsQuiet(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f := newTimerDemo()
	defer f.stopTimers()
	f.restartDelay()
	f.restartDelay()
	gui.PumpTimersForTest()
	if f.state.DelayFires != 1 || f.state.FiredGeneration != 2 || f.state.DelayArmed {
		t.Fatal(f.state)
	}
	gui.PumpTimersForTest()
	if f.state.DelayFires != 1 {
		t.Fatal("one-shot delay repeated", f.state)
	}
	f.restartDelay()
	f.cancelDelay()
	gui.PumpTimersForTest()
	if f.state.DelayFires != 1 || f.state.DelayArmed || f.delay != 0 {
		t.Fatal("canceled delay ran", f.state)
	}
}
func TestTimerDemoSmallLayoutStaysInsideOwner(t *testing.T) {
	f := newTimerDemo()
	defer f.stopTimers()
	for _, size := range [][2]float64{{0, 0}, {12, 8}, {320, 280}, {720, 350}} {
		f.SetSize(size[0], size[1])
		f.OnResize()
		for _, w := range f.widgets {
			b := w.Bounds1()
			if b.X < 0 || b.Y < 0 || b.Width < 0 || b.Height < 0 || b.Right() > size[0] || b.Bottom() > size[1] {
				t.Fatal("escaped owner", size, b)
			}
		}
	}
}
