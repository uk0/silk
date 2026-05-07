//go:build !windows && !silk_no_cairo

package gui

// forceGluiPath returns true when the current build cannot render
// through Cairo and the host should auto-engage SILK_GLUI behaviour.
// Default-build (Cairo enabled) returns false — the user opt-ins
// explicitly via SILK_GLUI=1 when they want the GPU path.
func forceGluiPath() bool { return false }
