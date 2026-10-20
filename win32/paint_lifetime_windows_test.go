package win32

import (
	"fmt"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// Exercise the actual SDK wrappers against a native window, not just the name
// of the imported procedure. EndPaint has a const PAINTSTRUCT input: it must
// not acquire a second DC or overwrite BeginPaint's output.
func TestBeginEndPaintReleasesDisplayDC(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	instance := GetModuleHandle("")
	name := syscall.StringToUTF16Ptr(fmt.Sprintf("SilkPaintLifetime%d", time.Now().UnixNano()))
	paints, mutations, noDC := 0, 0, 0
	callback := syscall.NewCallback(func(h HWND, message uint32, w, l uintptr) uintptr {
		if message != WM_PAINT {
			return DefWindowProc(h, message, w, l)
		}
		var ps PAINTSTRUCT
		if BeginPaint(h, &ps) == 0 {
			noDC++
			return 0
		}
		before := ps
		EndPaint(h, &ps)
		paints++
		if ps != before {
			mutations++
		}
		return 0
	})
	class := WNDCLASSEX{Size: uint32(unsafe.Sizeof(WNDCLASSEX{})), Instance: instance, ClassName: name, WndProc: callback}
	if RegisterClassEx(&class) == 0 {
		t.Fatal("RegisterClassEx failed")
	}
	t.Cleanup(func() {
		proc := syscall.NewLazyDLL("user32.dll").NewProc("UnregisterClassW")
		proc.Call(uintptr(unsafe.Pointer(name)), uintptr(instance))
	})
	h := CreateWindowEx(0, name, nil, WS_POPUP, 0, 0, 64, 48, 0, 0, instance, nil)
	if h == 0 {
		t.Fatal("CreateWindowEx failed")
	}
	defer DestroyWindow(h)
	resources := syscall.NewLazyDLL("user32.dll").NewProc("GetGuiResources")
	count := func() uintptr {
		n, _, _ := resources.Call(^uintptr(0), 0)
		return n
	}
	paint := func() {
		InvalidateRect(h, nil, false)
		SendMessage(h, WM_PAINT, 0, 0)
	}
	for i := 0; i < 5; i++ {
		paint()
	}
	baseline := count()
	for i := 0; i < 100; i++ {
		paint()
	}
	after := count()
	if noDC != 0 || paints < 105 {
		t.Fatalf("native paint not exercised: paints=%d nullDC=%d", paints, noDC)
	}
	if mutations != 0 {
		t.Errorf("EndPaint overwrote its read-only PAINTSTRUCT %d times", mutations)
	}
	if after > baseline+2 {
		t.Errorf("BeginPaint/EndPaint leaked display DC resources: before=%d after=%d", baseline, after)
	}
	t.Logf("100 native paint cycles GDI before=%d after=%d; PAINTSTRUCT mutations=%d", baseline, after, mutations)
}
