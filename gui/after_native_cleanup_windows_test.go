//go:build windows

package gui

import "testing"

func TestPostAfterNativeCleanupWindowsPostsWithoutInlineExecution(t *testing.T) {
	resetUIQueue()
	t.Cleanup(resetUIQueue)
	calls := 0
	PostAfterNativeCleanup(nil)
	PostAfterNativeCleanup(func() { calls++ })
	if calls != 0 {
		t.Fatal("continuation ran inline")
	}
	drainUITasks()
	drainUITasks()
	if calls != 1 {
		t.Fatal("continuation count", calls)
	}
}
