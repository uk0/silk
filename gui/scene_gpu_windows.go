//go:build windows && cgo && !silk_pure_go

package gui

import (
	"fmt"
	"image/color"
	"syscall"
	"unsafe"

	"github.com/uk0/silk/scene3d"
	"github.com/uk0/silk/scene3d/glrender"
	"github.com/uk0/silk/win32"
)

// Native WGL avoids initializing a second window/event framework in Win32.
// In particular it does not change process DPI policy or foreground-lock system
// parameters, enumerate joysticks, or create a second event pump.
type nativeSceneDevice struct {
	window  win32.HWND
	dc      win32.HDC
	rc      win32.HGLRC
	clients map[*nativeSceneGPU]struct{}
}
type nativeSceneGPU struct {
	device   *nativeSceneDevice
	renderer *glrender.Renderer
	info     glrender.Info
}

var sceneDevice *nativeSceneDevice
var sceneResourceClass uint16
var sceneResourceWndProc = syscall.NewCallback(func(hwnd win32.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	return win32.DefWindowProc(hwnd, msg, wParam, lParam)
})

func newWGLSceneDevice() (*nativeSceneDevice, error) {
	instance := win32.GetModuleHandle("")
	name := syscall.StringToUTF16Ptr("SILK_SCENE3D_RESOURCE")
	if sceneResourceClass == 0 {
		wc := win32.WNDCLASSEX{Size: uint32(unsafe.Sizeof(win32.WNDCLASSEX{})), Style: win32.CS_OWNDC, WndProc: sceneResourceWndProc, Instance: instance, ClassName: name}
		sceneResourceClass = uint16(win32.RegisterClassEx(&wc))
		if sceneResourceClass == 0 {
			return nil, fmt.Errorf("%w: RegisterClassEx: %v", glrender.ErrUnavailable, syscall.GetLastError())
		}
	}
	d := &nativeSceneDevice{clients: make(map[*nativeSceneGPU]struct{})}
	d.window = win32.CreateWindowEx(win32.WS_EX_TOOLWINDOW|win32.WS_EX_NOACTIVATE, name, nil, win32.WS_POPUP, 0, 0, 1, 1, 0, 0, instance, nil)
	if d.window == 0 {
		return nil, fmt.Errorf("%w: CreateWindowEx: %v", glrender.ErrUnavailable, syscall.GetLastError())
	}
	d.dc = win32.GetDC(d.window)
	if d.dc == 0 {
		d.destroy()
		return nil, fmt.Errorf("%w: GetDC", glrender.ErrUnavailable)
	}
	pfd := win32.PIXELFORMATDESCRIPTOR{Size: uint16(unsafe.Sizeof(win32.PIXELFORMATDESCRIPTOR{})), Version: 1, DwFlags: win32.PFD_DRAW_TO_WINDOW | win32.PFD_SUPPORT_OPENGL | win32.PFD_DOUBLEBUFFER, IPixelType: win32.PFD_TYPE_RGBA, ColorBits: 32, AlphaBits: 8, DepthBits: 24, ILayerType: win32.PFD_MAIN_PLANE}
	format := win32.ChoosePixelFormat(d.dc, &pfd)
	if format == 0 || !win32.SetPixelFormat(d.dc, format, &pfd) {
		d.destroy()
		return nil, fmt.Errorf("%w: SetPixelFormat", glrender.ErrUnavailable)
	}
	d.rc = win32.WglCreateContext(d.dc)
	if d.rc == 0 {
		d.destroy()
		return nil, fmt.Errorf("%w: wglCreateContext", glrender.ErrUnavailable)
	}
	return d, nil
}
func (d *nativeSceneDevice) destroy() {
	if d.rc != 0 {
		win32.WglDeleteContext(d.rc)
		d.rc = 0
	}
	if d.dc != 0 {
		win32.ReleaseDC(d.window, d.dc)
		d.dc = 0
	}
	if d.window != 0 {
		win32.DestroyWindow(d.window)
		d.window = 0
	}
}
func newSceneGPUBackend() (sceneGPUBackend, error) {
	if !IsUIThread() {
		return nil, ErrUIInvalid
	}
	previousDC, previousRC := win32.WglGetCurrentDC(), win32.WglGetCurrentContext()
	defer win32.WglMakeCurrent(previousDC, previousRC)
	if sceneDevice == nil {
		d, e := newWGLSceneDevice()
		if e != nil {
			return nil, e
		}
		sceneDevice = d
	}
	if !win32.WglMakeCurrent(sceneDevice.dc, sceneDevice.rc) {
		if len(sceneDevice.clients) == 0 {
			sceneDevice.destroy()
			sceneDevice = nil
		}
		return nil, fmt.Errorf("%w: wglMakeCurrent", glrender.ErrUnavailable)
	}
	r, e := glrender.NewCurrent()
	if e != nil {
		win32.WglMakeCurrent(previousDC, previousRC)
		if len(sceneDevice.clients) == 0 {
			sceneDevice.destroy()
			sceneDevice = nil
		}
		return nil, e
	}
	b := &nativeSceneGPU{device: sceneDevice, renderer: r, info: r.Info()}
	sceneDevice.clients[b] = struct{}{}
	return b, nil
}
func (b *nativeSceneGPU) Info() glrender.Info { return b.info }
func (b *nativeSceneGPU) Render(s *scene3d.Snapshot, c scene3d.Camera, w, h int, bg color.RGBA) (scene3d.FrameView, error) {
	if b.renderer == nil || b.device == nil || b.device.rc == 0 {
		return nil, glrender.ErrClosed
	}
	if !IsUIThread() {
		return nil, ErrUIInvalid
	}
	previousDC, previousRC := win32.WglGetCurrentDC(), win32.WglGetCurrentContext()
	if !win32.WglMakeCurrent(b.device.dc, b.device.rc) {
		return nil, fmt.Errorf("%w: wglMakeCurrent", glrender.ErrUnavailable)
	}
	defer win32.WglMakeCurrent(previousDC, previousRC)
	return b.renderer.Render(s, c, w, h, bg)
}
func (b *nativeSceneGPU) Close() {
	if b.renderer == nil {
		return
	}
	d := b.device
	previousDC, previousRC := win32.WglGetCurrentDC(), win32.WglGetCurrentContext()
	if win32.WglMakeCurrent(d.dc, d.rc) {
		b.renderer.Close()
	}
	b.renderer = nil
	delete(d.clients, b)
	if previousRC == d.rc {
		previousDC, previousRC = 0, 0
	}
	win32.WglMakeCurrent(previousDC, previousRC)
	if len(d.clients) == 0 {
		d.destroy()
		if sceneDevice == d {
			sceneDevice = nil
		}
	}
}
func closeSceneGPUDevice() {
	if sceneDevice != nil {
		for b := range sceneDevice.clients {
			b.Close()
		}
	}
}
