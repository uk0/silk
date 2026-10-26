//go:build !darwin || !cgo

package gui

const nativeCharacterPaletteAvailable = false

func nativeShowCharacterPalette(win *Window) bool { return false }
