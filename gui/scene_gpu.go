package gui

import (
	"github.com/uk0/silk/scene3d"
	"github.com/uk0/silk/scene3d/glrender"
	"image"
	"image/color"
)

// The GUI painter needs color pixels, not a complete CPU picking attachment.
type sceneViewFrame interface {
	RGBA() *image.RGBA
	RenderSequence() uint64
	IsCached() bool
}

type sceneGPUBackend interface {
	Render(*scene3d.Snapshot, scene3d.Camera, int, int, color.RGBA) (sceneViewFrame, error)
	Pick(*scene3d.Snapshot, scene3d.Camera, int, int, color.RGBA, int, int) (string, bool, error)
	Transfers() (glrender.TransferStats, error)
	Info() glrender.Info
	Close()
}

// RenderBackend is auto, cpu or opengl. Auto attempts GPU only for an attached
// native view on the UI thread and preserves the CPU path for offscreen tools.
func (v *SceneView) RenderBackend() string { return v.backendMode }
func (v *SceneView) SetRenderBackend(mode string) {
	if v.closed || (mode != "auto" && mode != "cpu" && mode != "opengl") || mode == v.backendMode {
		return
	}
	v.releaseGPU()
	v.backendMode = mode
	v.gpuAttempted = false
	v.gpuError = nil
	v.RefreshScene()
}
func (v *SceneView) ActiveRenderBackend() string {
	if v.gpu != nil {
		return "opengl"
	}
	return "cpu"
}
func (v *SceneView) GPUInfo() glrender.Info {
	if v.gpu != nil {
		return v.gpu.Info()
	}
	return glrender.Info{}
}

// GPUCapabilities reports the currently active native renderer. Before the first
// draw, after Close, and on CPU fallback it returns no GPU capabilities. Like
// other SceneView accessors this is UI-owner-only; it does not create a context.
func (v *SceneView) GPUCapabilities() glrender.Capabilities {
	if provider, ok := v.gpu.(interface{ Capabilities() glrender.Capabilities }); ok {
		return provider.Capabilities()
	}
	return glrender.Capabilities{}
}

// GPUError exposes why auto fell back, separately from model/render errors.
// Setting another backend then auto allows an explicit retry; Draw never loops
// context creation after a failure.
func (v *SceneView) GPUError() error { return v.gpuError }
func (v *SceneView) releaseGPU() {
	if v.gpu != nil {
		v.gpu.Close()
		v.gpu = nil
	}
	v.uploadedSequence = ^uint64(0)
}
func (v *SceneView) ensureSceneGPU() {
	if v.backendMode != "cpu" && !v.gpuAttempted && IsUIThread() && v.OwnerWindow() != nil {
		v.gpuAttempted = true
		v.gpu, v.gpuError = newSceneGPUBackend()
	}
}

func (v *SceneView) renderScene(s *scene3d.Snapshot, c scene3d.Camera, w, h int) (sceneViewFrame, error) {
	bg := color.RGBA{20, 28, 41, 255}
	v.ensureSceneGPU()
	var f sceneViewFrame
	var err error
	if v.gpu != nil {
		f, err = v.gpu.Render(s, c, w, h, bg)
		if err != nil {
			v.gpuError = err
			v.releaseGPU()
		}
	}
	if v.gpu == nil {
		if v.backendMode == "opengl" {
			if v.gpuError != nil {
				return nil, v.gpuError
			}
			return nil, glrender.ErrUnavailable
		}
		f, err = v.renderer.Render(s, c, w, h, bg)
	}
	if err != nil {
		return nil, err
	}
	if !f.IsCached() {
		v.renderCount++
	}
	return f, nil
}

// GPUTransfers returns cumulative successful transfers of the active backend.
// It does not initialize a context. CPU fallback/before first draw/after Close
// returns zero. Like all SceneView accessors, call on the owning UI thread.
func (v *SceneView) GPUTransfers() (glrender.TransferStats, error) {
	if v.gpu == nil {
		return glrender.TransferStats{}, nil
	}
	return v.gpu.Transfers()
}

func (v *SceneView) pickScene(s *scene3d.Snapshot, c scene3d.Camera, w, h, x, y int) string {
	v.ensureSceneGPU()
	if v.gpu != nil {
		id, cached, err := v.gpu.Pick(s, c, w, h, color.RGBA{20, 28, 41, 255}, x, y)
		if err == nil {
			if !cached {
				v.renderCount++
			}
			return id
		}
		v.gpuError = err
		v.releaseGPU()
	}
	if v.backendMode == "opengl" {
		return ""
	}
	f, err := v.renderer.Render(s, c, w, h, color.RGBA{20, 28, 41, 255})
	if err != nil {
		return ""
	}
	if !f.IsCached() {
		v.renderCount++
	}
	return f.Pick(x, y)
}
