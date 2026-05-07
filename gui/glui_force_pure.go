//go:build !windows && silk_no_cairo

package gui

// forceGluiPath returns true under silk_no_cairo because the Cairo
// back buffer path becomes a no-op (nullPainter) — glui is the only
// renderer that produces pixels in this build configuration.
//
// Hosts can still set SILK_GLUI explicitly; the env check happens
// alongside this function in create() and either signal triggers
// the glui code path.
func forceGluiPath() bool { return true }
