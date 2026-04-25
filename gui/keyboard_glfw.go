//go:build !windows

package gui

import (
	"runtime"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var glfwKeyToVK = map[glfw.Key]int{
	glfw.KeyBackspace:    KeyBackSpace,
	glfw.KeyTab:          KeyTab,
	glfw.KeyEnter:        KeyEnter,
	glfw.KeyLeftShift:    KeyShift,
	glfw.KeyRightShift:   KeyShift,
	glfw.KeyLeftControl:  KeyCtrl,
	glfw.KeyRightControl: KeyCtrl,
	glfw.KeyLeftAlt:      KeyMenu,
	glfw.KeyRightAlt:     KeyMenu,
	glfw.KeyPause:        KeyPause,
	glfw.KeyCapsLock:     KeyCapsLock,
	glfw.KeyEscape:       KeyEsc,
	glfw.KeySpace:        KeySpace,

	glfw.KeyPageUp:   KeyPageUp,
	glfw.KeyPageDown: KeyPageDown,
	glfw.KeyEnd:      KeyEnd,
	glfw.KeyHome:     KeyHome,

	glfw.KeyLeft:  KeyLeft,
	glfw.KeyUp:    KeyUp,
	glfw.KeyRight: KeyRight,
	glfw.KeyDown:  KeyDown,

	glfw.KeyPrintScreen: KeyPrintScreen,
	glfw.KeyInsert:      KeyInsert,
	glfw.KeyDelete:      KeyDelete,

	glfw.KeyLeftSuper:  KeyLWin,
	glfw.KeyRightSuper: KeyRWin,

	glfw.KeyKP0: KeyNumPad0,
	glfw.KeyKP1: KeyNumPad1,
	glfw.KeyKP2: KeyNumPad2,
	glfw.KeyKP3: KeyNumPad3,
	glfw.KeyKP4: KeyNumPad4,
	glfw.KeyKP5: KeyNumPad5,
	glfw.KeyKP6: KeyNumPad6,
	glfw.KeyKP7: KeyNumPad7,
	glfw.KeyKP8: KeyNumPad8,
	glfw.KeyKP9: KeyNumPad9,

	glfw.KeyKPMultiply: KeyMultiply,
	glfw.KeyKPAdd:      KeyAdd,
	glfw.KeyKPSubtract: KeySubtract,
	glfw.KeyKPDivide:   KeyDivide,
	glfw.KeyKPDecimal:  KeyDecimal,

	glfw.KeyF1:  KeyF1,
	glfw.KeyF2:  KeyF2,
	glfw.KeyF3:  KeyF3,
	glfw.KeyF4:  KeyF4,
	glfw.KeyF5:  KeyF5,
	glfw.KeyF6:  KeyF6,
	glfw.KeyF7:  KeyF7,
	glfw.KeyF8:  KeyF8,
	glfw.KeyF9:  KeyF9,
	glfw.KeyF10: KeyF10,
	glfw.KeyF11: KeyF11,
	glfw.KeyF12: KeyF12,
	glfw.KeyF13: KeyF13,
	glfw.KeyF14: KeyF14,
	glfw.KeyF15: KeyF15,
	glfw.KeyF16: KeyF16,

	glfw.KeyNumLock:    KeyNumLock,
	glfw.KeyScrollLock: KeyScrollLock,

	// A-Z map to their ASCII values (0x41-0x5A)
	glfw.KeyA: 0x41,
	glfw.KeyB: 0x42,
	glfw.KeyC: 0x43,
	glfw.KeyD: 0x44,
	glfw.KeyE: 0x45,
	glfw.KeyF: 0x46,
	glfw.KeyG: 0x47,
	glfw.KeyH: 0x48,
	glfw.KeyI: 0x49,
	glfw.KeyJ: 0x4A,
	glfw.KeyK: 0x4B,
	glfw.KeyL: 0x4C,
	glfw.KeyM: 0x4D,
	glfw.KeyN: 0x4E,
	glfw.KeyO: 0x4F,
	glfw.KeyP: 0x50,
	glfw.KeyQ: 0x51,
	glfw.KeyR: 0x52,
	glfw.KeyS: 0x53,
	glfw.KeyT: 0x54,
	glfw.KeyU: 0x55,
	glfw.KeyV: 0x56,
	glfw.KeyW: 0x57,
	glfw.KeyX: 0x58,
	glfw.KeyY: 0x59,
	glfw.KeyZ: 0x5A,

	// 0-9 map to their ASCII values (0x30-0x39)
	glfw.Key0: 0x30,
	glfw.Key1: 0x31,
	glfw.Key2: 0x32,
	glfw.Key3: 0x33,
	glfw.Key4: 0x34,
	glfw.Key5: 0x35,
	glfw.Key6: 0x36,
	glfw.Key7: 0x37,
	glfw.Key8: 0x38,
	glfw.Key9: 0x39,
}

func translateKey(key glfw.Key, mods glfw.ModifierKey) int {
	// On macOS, map Cmd (Super) to Ctrl for standard shortcuts
	if runtime.GOOS == "darwin" && mods&glfw.ModSuper != 0 {
		if key == glfw.KeyLeftSuper || key == glfw.KeyRightSuper {
			return KeyCtrl
		}
	}

	vk, ok := glfwKeyToVK[key]
	if ok {
		return vk
	}

	// For keys that directly correspond to ASCII values
	if key >= 32 && key <= 126 {
		return int(key)
	}

	return 0
}
