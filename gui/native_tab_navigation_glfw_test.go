//go:build !windows

package gui

import "testing"

func TestWindowNativeKeyRoutingCleanupIsIdempotent(t *testing.T) {
	calls := 0
	win := &Window{}
	win.nativeKeyCleanup = func() { calls++; win.releaseNativeKeyRouting() }
	win.releaseNativeKeyRouting()
	win.releaseNativeKeyRouting()
	if calls != 1 || win.nativeKeyCleanup != nil {
		t.Fatal("native key routing survived or repeated teardown")
	}
	win.nativeKeyCleanup = func() { calls++ }
	win.destroy()
	win.destroy()
	if calls != 2 || win.nativeKeyCleanup != nil {
		t.Fatal("destroy did not retire native key routing")
	}
}
