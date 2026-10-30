//go:build !windows && (!cgo || silk_pure_go)

package gui

import "github.com/uk0/silk/scene3d/glrender"

func newWindowSceneGPUBackend(*Window) (sceneGPUBackend, error) { return nil, glrender.ErrUnavailable }
func closeWindowSceneGPUDevice(*Window)                         {}
func sceneCompositionAvailable() bool                           { return false }
