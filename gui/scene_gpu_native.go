//go:build cgo && !silk_pure_go && !windows

package gui

import (
	"fmt"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/scene3d"
	"github.com/uk0/silk/scene3d/glrender"
	"image/color"
)

// This unshown context is a resource owner, not a visual child window. Color/ID
// FBOs are read back and composited through the same Cairo tree as other widgets.
// A shared device avoids a native context/window per SceneView.
type nativeSceneDevice struct {
	window  *glfw.Window
	clients map[*nativeSceneGPU]struct{}
}
type nativeSceneGPU struct {
	device   *nativeSceneDevice
	renderer *glrender.Renderer
	info     glrender.Info
}

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
func (b *nativeSceneGPU) Render(s *scene3d.Snapshot, c scene3d.Camera, w, h int, bg color.RGBA) (scene3d.FrameView, error) {
	if b.renderer == nil || b.device == nil || b.device.window == nil {
		return nil, glrender.ErrClosed
	}
	if !IsUIThread() {
		return nil, ErrUIInvalid
	}
	previous := glfw.GetCurrentContext()
	b.device.window.MakeContextCurrent()
	defer sceneRestoreContext(previous)
	return b.renderer.Render(s, c, w, h, bg)
}
func (b *nativeSceneGPU) Close() {
	if b.renderer == nil {
		return
	}
	previous := glfw.GetCurrentContext()
	d := b.device
	d.window.MakeContextCurrent()
	b.renderer.Close()
	b.renderer = nil
	delete(d.clients, b)
	if previous == d.window {
		previous = nil
	}
	sceneRestoreContext(previous)
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
	if sceneDevice == nil {
		return
	}
	for b := range sceneDevice.clients {
		b.Close()
	}
}
