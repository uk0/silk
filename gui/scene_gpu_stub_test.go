//go:build (!cgo || silk_pure_go) && !windows

package gui

import "github.com/uk0/silk/scene3d/glrender"

var sceneNativeErr error

func runSceneNativeProbe() error { return glrender.ErrUnavailable }
