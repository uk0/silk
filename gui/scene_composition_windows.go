//go:build windows

package gui

import "github.com/uk0/silk/scene3d/glrender"

// SetSceneGPUComposition is not available on the GDI window presenter. It does
// not disable the ordinary GPU renderer's selective color capture/ID picking.
func (w *Window) SetSceneGPUComposition(enabled bool) error {
	if !IsUIThread() {
		return ErrUIInvalid
	}
	if enabled {
		return glrender.ErrUnavailable
	}
	return nil
}
func (*Window) SceneGPUCompositionStatus() (bool, string) { return false, "" }
