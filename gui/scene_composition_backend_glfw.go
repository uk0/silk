//go:build cgo && !silk_pure_go && !windows

package gui

import (
	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/scene3d"
	"github.com/uk0/silk/scene3d/glrender"
	"image"
	"image/color"
)

// Only the owner UI thread enters here. The SDK's GL dispatch remains bound to
// this exact context; unrelated native windows do not share framebuffer names.
func newWindowSceneGPUBackend(w *Window) (sceneGPUBackend, error) {
	if !IsUIThread() {
		return nil, ErrUIInvalid
	}
	if w == nil || w.glfwWin == nil {
		return nil, glrender.ErrClosed
	}
	previous := glfw.GetCurrentContext()
	defer sceneRestoreContext(previous)
	w.glfwWin.MakeContextCurrent()
	// The host binding is optional on older GL contexts; never call a nil EXT entry.
	if glfw.GetProcAddress("glBindFramebufferEXT") == nil {
		return nil, glrender.ErrUnavailable
	}
	defer resetSceneHostGL()
	r, err := glrender.NewCurrent()
	if err != nil {
		return nil, err
	}
	if !r.CanBlitColor() {
		r.Close()
		return nil, glrender.ErrUnavailable
	}
	d := windowSceneDevices[w]
	if d == nil {
		d = &nativeSceneDevice{window: w.glfwWin, owner: w, clients: make(map[*nativeSceneGPU]struct{})}
		windowSceneDevices[w] = d
	}
	b := &nativeSceneGPU{device: d, renderer: r, info: r.Info()}
	d.clients[b] = struct{}{}
	return b, nil
}
func closeWindowSceneGPUDevice(w *Window) {
	if !IsUIThread() {
		return
	}
	if d := windowSceneDevices[w]; d != nil {
		for b := range d.clients {
			b.Close()
		}
	}
}
func (b *nativeSceneGPU) sceneWindow() *Window {
	if b.device == nil {
		return nil
	}
	return b.device.owner
}
func (b *nativeSceneGPU) renderResident(s *scene3d.Snapshot, c scene3d.Camera, w, h int) (frame glrender.GPUFrame, err error) {
	err = b.withCurrent(func(r *glrender.Renderer) error {
		var e error
		frame, e = r.RenderGPU(s, c, w, h, color.RGBA{20, 28, 41, 255})
		return e
	})
	return
}
func (b *nativeSceneGPU) blitResident(frame glrender.GPUFrame, w, h int, dst, clip image.Rectangle) error {
	return b.withCurrent(func(r *glrender.Renderer) error { return r.BlitColor(frame, 0, w, h, dst, clip) })
}

// SDK drawing mutates GL state. Restore the window backend baseline before
// uploads/presentation, even when a raster pick occurs outside a paint pass.
func resetSceneHostGL() {
	gl.UseProgram(0)
	gl.BindFramebufferEXT(gl.FRAMEBUFFER_EXT, 0)
	gl.DrawBuffer(gl.BACK)
	gl.ReadBuffer(gl.BACK)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, 0)
	gl.BindBuffer(gl.PIXEL_PACK_BUFFER, 0)
	gl.BindBuffer(gl.PIXEL_UNPACK_BUFFER, 0)
	initGL()
}
func sceneCompositionAvailable() bool { return true }
