//go:build cgo && !silk_pure_go

package gui

import "github.com/uk0/silk/scene3d/glrender"

func (b *nativeSceneGPU) Capabilities() glrender.Capabilities {
	if b.renderer == nil {
		return glrender.Capabilities{}
	}
	return b.renderer.Capabilities()
}
