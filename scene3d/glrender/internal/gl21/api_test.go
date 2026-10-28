//go:build cgo && !silk_pure_go

package gl21

import (
	"runtime"
	"testing"
)

func TestOpenWithoutContextNeverPublishesPartialTable(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// No GUI library is initialized in this package's test process. Missing GL
	// libraries and missing current contexts are both explicit errors, not stubs
	// that allow calls through a partially initialized/null function table.
	for i := 0; i < 100; i++ {
		api, err := OpenCurrent()
		if err == nil || api != nil {
			if api != nil {
				api.Close()
			}
			t.Fatalf("iteration %d published table without context: %v", i, err)
		}
	}
	var zero API
	if zero.Current() || zero.OnThread() {
		t.Fatal("zero API has native ownership")
	}
	zero.Close()
	if EntryCount != 58 {
		t.Fatal("review any change to the supported GL ABI", EntryCount)
	}
}
