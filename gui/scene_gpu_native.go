//go:build cgo && !silk_pure_go && !windows

package gui

import (
	"fmt"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/scene3d/glrender"
	"runtime"
)

// This unshown context is a resource owner, not a visual child window. Color
// is composited through the ordinary Cairo tree. IDs are read only on picking.
// A shared device avoids a native context/window per SceneView.
type nativeSceneDevice struct {
	window  *glfw.Window
	clients map[*nativeSceneGPU]struct{}
	owner   *Window // non-nil: borrowed visible context, never destroy here
}
type nativeSceneGPU struct {
	device   *nativeSceneDevice
	renderer *glrender.Renderer
	info     glrender.Info
	frame    glrender.ColorFrame
}

var windowSceneDevices = make(map[*Window]*nativeSceneDevice)

var sceneDevice *nativeSceneDevice // UI-thread-only, like the window registry

func sceneRestoreContext(previous *glfw.Window) {
	if previous != nil {
		previous.MakeContextCurrent()
	} else {
		glfw.DetachCurrentContext()
	}
}
func newSceneGPUBackend() (sceneGPUBackend, error) {
	if !IsUIThread() {
		return nil, ErrUIInvalid
	}
	// The GLFW application backend initializes the library on the UI thread.
	if e := glfw.Init(); e != nil {
		return nil, fmt.Errorf("%w: %v", glrender.ErrUnavailable, e)
	}
	previous := glfw.GetCurrentContext()
	defer sceneRestoreContext(previous)
	if sceneDevice == nil {
		glfw.DefaultWindowHints()
		defer glfw.DefaultWindowHints()
		glfw.WindowHint(glfw.Visible, glfw.False)
		glfw.WindowHint(glfw.Focused, glfw.False)
		glfw.WindowHint(glfw.FocusOnShow, glfw.False)
		glfw.WindowHint(glfw.ContextVersionMajor, 2)
		glfw.WindowHint(glfw.ContextVersionMinor, 1)
		w, e := glfw.CreateWindow(1, 1, "Silk 3D resource context", nil, nil)
		if e != nil {
			return nil, fmt.Errorf("%w: %v", glrender.ErrUnavailable, e)
		}
		sceneDevice = &nativeSceneDevice{window: w, clients: make(map[*nativeSceneGPU]struct{})}
	}
	sceneDevice.window.MakeContextCurrent()
	r, e := glrender.NewCurrent()
	if e != nil {
		if len(sceneDevice.clients) == 0 {
			sceneDevice.window.Destroy()
			sceneDevice = nil
		}
		return nil, e
	}
	b := &nativeSceneGPU{device: sceneDevice, renderer: r, info: r.Info()}
	sceneDevice.clients[b] = struct{}{}
	return b, nil
}
func (b *nativeSceneGPU) Info() glrender.Info { return b.info }
func (b *nativeSceneGPU) withCurrent(fn func(*glrender.Renderer) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Reject a worker before reading fields the UI thread may change on Close.
	if !IsUIThread() {
		return ErrUIInvalid
	}
	if b.renderer == nil || b.device == nil || b.device.window == nil {
		return glrender.ErrClosed
	}
	previous := glfw.GetCurrentContext()
	b.device.window.MakeContextCurrent()
	defer sceneRestoreContext(previous)
	if b.device.owner != nil {
		defer resetSceneHostGL()
	}
	return fn(b.renderer)
}
func (b *nativeSceneGPU) Close() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if !IsUIThread() {
		return
	}
	if b.renderer == nil {
		return
	}
	previous := glfw.GetCurrentContext()
	d := b.device
	d.window.MakeContextCurrent()
	b.renderer.Close()
	b.renderer = nil
	b.frame = glrender.ColorFrame{}
	delete(d.clients, b)
	if d.owner == nil && previous == d.window {
		previous = nil
	}
	if d.owner != nil {
		resetSceneHostGL()
	}
	sceneRestoreContext(previous)
	if d.owner != nil {
		if len(d.clients) == 0 {
			delete(windowSceneDevices, d.owner)
		}
		return
	}
	if len(d.clients) == 0 {
		d.window.Destroy()
		d.window = nil
		if sceneDevice == d {
			sceneDevice = nil
		}
	}
}

// Called before the native event loop destroys/terminates graphics contexts.
// Views may outlive their window; subsequent Close remains idempotent.
func closeSceneGPUDevice() {
	if !IsUIThread() {
		return
	}
	for w := range windowSceneDevices {
		closeWindowSceneGPUDevice(w)
	}
	if sceneDevice == nil {
		return
	}
	for b := range sceneDevice.clients {
		b.Close()
	}
}
