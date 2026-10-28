package gui

import (
	"github.com/uk0/silk/scene3d"
	"github.com/uk0/silk/scene3d/glrender"
	"image/color"
)

type sceneGPUBackend interface {
	Render(*scene3d.Snapshot, scene3d.Camera, int, int, color.RGBA) (scene3d.FrameView, error)
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
func (v *SceneView) renderScene(s *scene3d.Snapshot, c scene3d.Camera, w, h int) (scene3d.FrameView, error) {
	bg := color.RGBA{20, 28, 41, 255}
	if v.backendMode != "cpu" && !v.gpuAttempted && IsUIThread() && v.OwnerWindow() != nil {
		v.gpuAttempted = true
		v.gpu, v.gpuError = newSceneGPUBackend()
	}
	var f scene3d.FrameView
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
