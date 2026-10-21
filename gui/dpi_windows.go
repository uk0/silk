package gui

import "syscall"

// System awareness prevents Windows from stretching a 96-DPI bitmap. The
// framework uses physical client coordinates throughout; do not claim PMv2
// until layout/input/IME all implement per-monitor scale transitions.
// Never override a manifest/embedding host's existing awareness context.
func initializeWindowsDPI() {
	user32 := syscall.NewLazyDLL("user32.dll")
	set := user32.NewProc("SetProcessDpiAwarenessContext")
	if set.Find() == nil {
		ok, _, err := set.Call(^uintptr(1)) // DPI_AWARENESS_CONTEXT_SYSTEM_AWARE (-2)
		if ok != 0 || err == syscall.Errno(5) {
			return
		} // already configured
	}
	legacy := user32.NewProc("SetProcessDPIAware")
	if legacy.Find() == nil {
		_, _, _ = legacy.Call()
	}
}
