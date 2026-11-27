//go:build !windows && (!darwin || !cgo) && (!linux || !cgo || wayland)

package gui

import "github.com/go-gl/glfw/v3.3/glfw"

// Other GLFW platforms keep their existing native policy. Go-side popup
// ownership/dismissal remains active; this does not claim native transient ownership.
func bindNativePopupOwner(popup, owner *glfw.Window) bool { return false }

func unbindNativePopupOwner(popup *glfw.Window) {}

// Without an application-activation API, preserve the existing GLFW policy.
func nativeApplicationActive(keyWindowFocused bool) bool { return keyWindowFocused }

func prepareNativePopupOwner(popup, owner *glfw.Window) {}
