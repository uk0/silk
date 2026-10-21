package gui

import (
	"math"
	"unicode"
	"unicode/utf8"

	"github.com/uk0/silk/win32"
)

// Logical widget focus lives inside one HWND. Reset even on an A -> B -> A
// change so an unfinished surrogate can never be joined across fields.
func textInputFocusChanged() {
	for _, win := range winMap {
		win.textInput.reset()
	}
}

func (win *Window) textInputTarget() IEventTextInput {
	target := focusWidget
	if target == nil || win.keyTarget() != target || !target.IsEnabled() {
		return nil
	}
	input, _ := target.(IEventTextInput)
	return input
}

func (win *Window) dispatchUTF16(unit uint16) bool {
	// TranslateMessage emits a printable WM_CHAR for Ctrl+Space too. The
	// keydown already requested completion; inserting that space immediately
	// cancels it and changes the buffer. Keep AltGr (Ctrl+Alt) text input intact.
	if unit == ' ' && IsKeyDown(KeyCtrl) && !IsKeyDown(win32.VK_MENU) {
		win.textInput.reset()
		return true
	}
	input := win.textInputTarget()
	if input == nil {
		win.textInput.reset()
		return false
	}
	if text := win.textInput.decode(unit); text != "" {
		input.OnTextInput(text)
	}
	return !unicode.IsControl(rune(unit))
}

func (win *Window) dispatchUnicode(value uintptr) {
	win.textInput.reset()
	if value > utf8.MaxRune {
		return
	}
	r := rune(value)
	if !utf8.ValidRune(r) || unicode.IsControl(r) {
		return
	}
	if input := win.textInputTarget(); input != nil {
		input.OnTextInput(string(r))
	}
}

// Update only on native IME events, never on an idle timer or every frame.
// Leave preedit rendering, candidate selection and the final WM_CHAR commit to
// DefWindowProc/the system IME. Handling GCS_RESULTSTR here as well would insert
// every committed character twice.
func (win *Window) positionInputMethod(candidateMask uint32) bool {
	if win.positioningIME || win.hWnd == 0 || win.textInputTarget() == nil {
		return false
	}
	target := focusWidget
	cursor, ok := target.(IInputMethodCursor)
	if !ok {
		return false
	}
	r := cursor.InputMethodCursorRect()
	if r.Width <= 0 || r.Height <= 0 {
		return false
	}
	x, y := target.MapToGlobal(r.X, r.Y)
	cx, cy, ok := win32.ScreenToClient(win.hWnd, int(math.Round(x)), int(math.Round(y)))
	if !ok {
		return false
	}
	context := win32.ImmGetContext(win.hWnd)
	if context == 0 {
		return false
	}
	defer win32.ImmReleaseContext(win.hWnd, context)
	// Setters can synchronously send IMN_SETCOMPOSITIONWINDOW back to this HWND.
	win.positioningIME = true
	defer func() { win.positioningIME = false }()
	area := win32.RECT{Left: int32(cx), Top: int32(cy), Right: int32(cx) + int32(math.Ceil(r.Width)), Bottom: int32(cy) + int32(math.Ceil(r.Height))}
	comp := win32.COMPOSITIONFORM{DwStyle: win32.CFS_POINT, PtCurrentPos: win32.POINT{X: area.Left, Y: area.Top}}
	success := win32.ImmSetCompositionWindow(context, &comp)
	if candidateMask == 0 {
		candidateMask = 1
	}
	// IMM supports candidate list indices 0..3. The notification carries a mask,
	// not an index; update each requested list without changing the user's IME.
	for index := uint32(0); index < 4; index++ {
		if candidateMask&(1<<index) == 0 {
			continue
		}
		candidate := win32.CANDIDATEFORM{DwIndex: index, DwStyle: win32.CFS_EXCLUDE, PtCurrentPos: win32.POINT{X: area.Left, Y: area.Bottom}, RcArea: area}
		success = win32.ImmSetCandidateWindow(context, &candidate) && success
	}
	return success
}
