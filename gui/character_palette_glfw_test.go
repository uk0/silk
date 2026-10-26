//go:build !windows

package gui

import (
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestCharacterPaletteShortcutAndInputScope(t *testing.T) {
	if !CharacterPaletteAvailable() {
		t.Skip("native palette is a macOS capability")
	}
	for _, kind := range []string{"editor", "edit", "search"} {
		t.Run(kind, func(t *testing.T) {
			isolatePopupState(t)
			var target IWidget
			switch kind {
			case "editor":
				target = NewCodeEditor()
			case "edit":
				target = NewEdit()
			case "search":
				target = NewSearchBox()
			}
			native, win := testPopupWindow(target, WtForm, nil)
			win.enabled = true
			target.SetFocus()
			old := requestCharacterPalette
			t.Cleanup(func() { requestCharacterPalette = old })
			calls := 0
			requestCharacterPalette = func(got *Window) bool {
				if got != win {
					t.Fatal("wrong owner")
				}
				calls++
				return true
			}
			onKey(native, glfw.KeySpace, 49, glfw.Press, glfw.ModControl|glfw.ModSuper)
			if calls != 1 {
				t.Fatalf("standard character shortcut requested %d palettes, want 1", calls)
			}
			onKey(native, glfw.KeySpace, 49, glfw.Repeat, glfw.ModControl|glfw.ModSuper)
			onKey(native, glfw.KeySpace, 49, glfw.Release, glfw.ModControl|glfw.ModSuper)
			if calls != 1 {
				t.Fatal("repeat/release opened another palette")
			}
		})
	}
}

func TestCharacterPaletteRejectsIneligibleTargets(t *testing.T) {
	if !CharacterPaletteAvailable() {
		t.Skip("native palette is a macOS capability")
	}
	for _, kind := range []string{"nil", "unfocused", "hidden", "disabled", "hidden-parent", "disabled-parent", "native-disabled", "detached", "foreign-form", "not-text"} {
		t.Run(kind, func(t *testing.T) {
			isolatePopupState(t)
			root := NewEdit()
			_, win := testPopupWindow(root, WtForm, nil)
			win.enabled = true
			parent := NewLabel("")
			parent.SetParent(root)
			field := NewEdit()
			field.SetParent(parent)
			field.SetFocus()
			var target IWidget = field
			old := requestCharacterPalette
			t.Cleanup(func() { requestCharacterPalette = old })
			calls := 0
			requestCharacterPalette = func(*Window) bool { calls++; return true }
			switch kind {
			case "nil":
				target = nil
			case "unfocused":
				focusWidget = root
			case "hidden":
				field.NakedWidget().hidden = true
			case "disabled":
				field.NakedWidget().disabled = true
			case "hidden-parent":
				parent.NakedWidget().hidden = true
			case "disabled-parent":
				parent.NakedWidget().disabled = true
			case "native-disabled":
				win.enabled = false
			case "detached":
				field.SetParent(nil)
			case "foreign-form":
				other := NewEdit()
				_, ow := testPopupWindow(other, WtForm, nil)
				ow.enabled = true
				other.SetFocus()
			case "not-text":
				parent.SetFocus()
				focusWidget = parent
				target = parent
			}
			if ShowCharacterPalette(target) || calls != 0 {
				t.Fatal("ineligible request opened palette")
			}
		})
	}
}

func TestCharacterPalettePreservesOtherChords(t *testing.T) {
	isolatePopupState(t)
	field := NewEdit()
	_, win := testPopupWindow(field, WtForm, nil)
	win.enabled = true
	field.SetFocus()
	old := requestCharacterPalette
	t.Cleanup(func() { requestCharacterPalette = old })
	requestCharacterPalette = func(*Window) bool { t.Fatal("non-palette chord requested palette"); return false }
	for _, mods := range []glfw.ModifierKey{0, glfw.ModControl, glfw.ModSuper, glfw.ModControl | glfw.ModSuper | glfw.ModAlt, glfw.ModControl | glfw.ModSuper | glfw.ModShift} {
		if win.dispatchCharacterPalette(glfw.KeySpace, glfw.Press, mods) {
			t.Fatal("claimed unrelated chord", mods)
		}
	}
	if win.dispatchCharacterPalette(glfw.KeyTab, glfw.Press, glfw.ModControl|glfw.ModSuper) {
		t.Fatal("claimed Tab")
	}
}

func TestCharacterPaletteRespectsHostShortcutAndComposition(t *testing.T) {
	if !CharacterPaletteAvailable() {
		t.Skip("native palette is a macOS capability")
	}
	isolatePopupState(t)
	field := NewEdit()
	native, win := testPopupWindow(field, WtForm, nil)
	win.enabled = true
	field.SetFocus()
	oldRequest, oldHandlers := requestCharacterPalette, shortcutHandlers
	oldQuery, oldReady := glfwCompositionActive, glfwReady
	t.Cleanup(func() {
		requestCharacterPalette = oldRequest
		shortcutHandlers = oldHandlers
		glfwCompositionActive = oldQuery
		glfwReady = oldReady
	})
	requestCharacterPalette = func(*Window) bool { t.Fatal("palette stole host/IME ownership"); return false }
	shortcutHandlers = map[shortcutKey]func(){}
	calls := 0
	RegisterShortcut(ModAction, KeySpace, func() { calls++ })
	onKey(native, glfw.KeySpace, 49, glfw.Press, glfw.ModControl|glfw.ModSuper)
	if calls != 1 {
		t.Fatal("explicit host binding was bypassed")
	}
	RegisterShortcut(ModAction, KeySpace, nil)
	glfwReady = true
	glfwCompositionActive = func(*glfw.Window) bool { return true }
	onKey(native, glfw.KeySpace, 49, glfw.Press, glfw.ModControl|glfw.ModSuper)
}
