//go:build !windows

package gui

/*
#include <pthread.h>
#include <stdint.h>

// pthread_t is an opaque pointer on Darwin and an unsigned long on Linux; the
// cast through uintptr_t is what lets one signature serve both.
static unsigned long long silk_current_thread(void) {
	return (unsigned long long)(uintptr_t)pthread_self();
}
*/
import "C"

// UI thread identity for the GLFW backend, the other half of uithread.go's
// off-thread mutation detector.
//
// Go exposes no thread identity of its own, and the GLFW Go binding is not an
// option: every wrapper drains the package-global error channel, so calling one
// from a worker goroutine would steal an error the main thread was about to
// see. pthread_self is the same kind of primitive win32.GetCurrentThreadId is
// on the other backend, and it costs about 20ns — the check sits on
// Widget.Update, and only SILK_DEBUG builds ever reach it.

// uiThreadId is the thread window_glfw.go's init() locked itself to. Written
// once during package init, before any other goroutine can run, then read-only.
var uiThreadId uint64

func currentThreadId() uint64 { return uint64(C.silk_current_thread()) }
