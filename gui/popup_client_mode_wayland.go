//go:build linux && wayland

package gui

// GLFW exposes Wayland popups as top-level windows, whose absolute position
// the compositor controls. Toolkit popups live in their owner's client area.
const clientPopups = true
