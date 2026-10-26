//go:build !darwin || !cgo

package gui

import "testing"

func TestCharacterPaletteUnsupportedDoesNotClaimInput(t *testing.T) {
	if CharacterPaletteAvailable() {
		t.Fatal("unsupported backend advertised native character palette")
	}
	if ShowCharacterPalette(nil) {
		t.Fatal("nil target opened palette")
	}
	e := NewCodeEditor()
	if ShowCharacterPalette(e) {
		t.Fatal("unsupported request succeeded")
	}
}
