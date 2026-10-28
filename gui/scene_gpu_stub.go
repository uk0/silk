//go:build !cgo || silk_pure_go

package gui

import "github.com/uk0/silk/scene3d/glrender"

func newSceneGPUBackend() (sceneGPUBackend, error) { return nil, glrender.ErrUnavailable }
func closeSceneGPUDevice()                         {}
