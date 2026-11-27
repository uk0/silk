//go:build !windows && (!linux || wayland)

package gui

// Wayland requires an exported xdg-foreign handle, not a raw GLFW surface.
func desktopFilePortal(_ bool, _ string) (string, bool) { return "", false }
