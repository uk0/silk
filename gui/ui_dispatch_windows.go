package gui

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/uk0/silk/win32"
)

// uiMessageWindow has no pixels, activation or entry in winMap. Its sole job
// is to let native modal loops dispatch worker wakeups to the UI thread.
// The mutex joins in-flight PostMessage calls before DestroyWindow, including
// workers which copied an old wakeup hook just before SetUIWakeup(nil).
type uiMessageWindow struct {
	mu   sync.Mutex
	hwnd win32.HWND
}

var uiDispatcher *uiMessageWindow // UI-thread owned
var uiMessageClassOnce sync.Once
var uiMessageProc = syscall.NewCallback(func(hwnd win32.HWND, msg uint32, wParam, lParam uintptr) (ret uintptr) {
	defer func() {
		if e := recover(); e != nil {
			reportWndProcPanic(hwnd, msg, wParam, lParam, e)
		}
	}()
	if msg == WM_UI_WAKEUP {
		serviceUIFrame()
		return 0
	}
	return win32.DefWindowProc(hwnd, msg, wParam, lParam)
})

func newUIMessageWindow() *uiMessageWindow {
	name := syscall.StringToUTF16Ptr("SILK_UI_DISPATCH")
	uiMessageClassOnce.Do(func() {
		wc := win32.WNDCLASSEX{Instance: hInstance, ClassName: name, WndProc: uiMessageProc}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if win32.RegisterClassEx(&wc) == 0 {
			panic("ui: cannot register dispatch window class")
		}
	})
	hwnd := win32.CreateWindowEx(0, name, nil, 0, 0, 0, 0, 0, win32.HWND_MESSAGE, 0, hInstance, nil)
	if hwnd == 0 {
		panic("ui: cannot create dispatch window")
	}
	return &uiMessageWindow{hwnd: hwnd}
}

func (d *uiMessageWindow) wake() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hwnd != 0 {
		win32.PostMessage(d.hwnd, WM_UI_WAKEUP, 0, 0)
	}
}

func (d *uiMessageWindow) close() {
	d.mu.Lock()
	hwnd := d.hwnd
	d.hwnd = 0
	d.mu.Unlock()
	if hwnd != 0 {
		win32.DestroyWindow(hwnd) // on the owner thread, never while holding mu
	}
}

func startUIMessageWindow() {
	if uiDispatcher == nil {
		uiDispatcher = newUIMessageWindow()
		SetUIWakeup(uiDispatcher.wake)
	}
}

func stopUIMessageWindow() {
	SetUIWakeup(nil)
	if uiDispatcher != nil {
		uiDispatcher.close()
		uiDispatcher = nil
	}
}
