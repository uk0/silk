//go:build !windows

package gui

import "github.com/go-gl/glfw/v3.3/glfw"

func (win *Window) dispatchCharacterPalette(key glfw.Key, action glfw.Action, mods glfw.ModifierKey) bool {
	if !CharacterPaletteAvailable() || key != glfw.KeySpace ||
		mods&(glfw.ModControl|glfw.ModSuper|glfw.ModAlt|glfw.ModShift) != glfw.ModControl|glfw.ModSuper {
		return false
	}
	if action != glfw.Press {
		return true // holding the chord must not open repeated system panels
	}
	return ShowCharacterPalette(win.focusedInputTarget())
}
