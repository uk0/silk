package gui

import (
	"math"
	"os"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/uk0/silk/win32"
)

func nativeTextInputEditor(t *testing.T) *CodeEditor {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	isolateWindowsPopupState(t)
	e := NewCodeEditor()
	e.SetVisible(false)
	e.SetSize(420, 240)
	e.AttachWindow(WtForm)
	if e.Window() == nil || e.Window().hWnd == 0 {
		t.Fatal("native editor window missing")
	}
	t.Cleanup(e.Window().destroy)
	// Keep the HWND off-screen, but model a visible logical input surface.
	e.NakedWidget().hidden = false
	e.SetFocus()
	return e
}

func TestWindowsTextInputSurrogatePair(t *testing.T) {
	e := nativeTextInputEditor(t)
	h := e.Window().hWnd
	win32.SendMessage(h, win32.WM_CHAR, 0xD83D, 1)
	if got := e.Text(); got != "" {
		t.Fatalf("high surrogate was committed before its pair: %q", got)
	}
	win32.SendMessage(h, win32.WM_CHAR, 0xDE80, 1)
	win32.SendMessage(h, win32.WM_CHAR, 0x4E2D, 1)
	if got := e.Text(); got != "🚀中" {
		t.Fatalf("native UTF-16 input = %q", got)
	}
}

func TestWindowsCtrlSpaceRequestsCompletionWithoutInsertingSpace(t *testing.T) {
	e := nativeTextInputEditor(t)
	e.SetText("fmt.Pr")
	e.ScrollToLineCol(0, 6)
	requests := 0
	e.SigCompletionRequested(func() { requests++ })
	user32 := syscall.NewLazyDLL("user32.dll")
	get, set := user32.NewProc("GetKeyboardState"), user32.NewProc("SetKeyboardState")
	var original [256]byte
	if ok, _, _ := get.Call(uintptr(unsafe.Pointer(&original[0]))); ok == 0 {
		t.Fatal("GetKeyboardState")
	}
	defer set.Call(uintptr(unsafe.Pointer(&original[0])))
	state := [256]byte{}
	state[KeyCtrl] = 0x80
	if ok, _, _ := set.Call(uintptr(unsafe.Pointer(&state[0]))); ok == 0 {
		t.Fatal("SetKeyboardState")
	}
	h := e.Window().hWnd
	win32.SendMessage(h, win32.WM_KEYDOWN, uintptr(KeySpace), 1)
	win32.SendMessage(h, win32.WM_CHAR, ' ', 1)
	if e.Text() != "fmt.Pr" || requests != 1 || !e.CompletionVisible() {
		t.Fatalf("text=%q requests=%d popup=%v", e.Text(), requests, e.CompletionVisible())
	}
	state[win32.VK_MENU] = 0x80 // AltGr still delivers ordinary text.
	set.Call(uintptr(unsafe.Pointer(&state[0])))
	win32.SendMessage(h, win32.WM_CHAR, ' ', 1)
	if e.Text() != "fmt.Pr " {
		t.Fatalf("AltGr input lost: %q", e.Text())
	}
}

func TestWindowsTextInputUnicodeMessage(t *testing.T) {
	e := nativeTextInputEditor(t)
	h := e.Window().hWnd
	if got := win32.SendMessage(h, win32.WM_UNICHAR, 0xFFFF, 0); got != 1 {
		t.Fatalf("WM_UNICHAR capability = %v", got)
	}
	win32.SendMessage(h, win32.WM_UNICHAR, 0x1F680, 1)
	if e.Text() != "🚀" {
		t.Fatalf("WM_UNICHAR input = %q", e.Text())
	}
}

func TestWindowsIMEPositionsAtEditorCaret(t *testing.T) {
	e := nativeTextInputEditor(t)
	e.SetText("first\nsecond line")
	e.cursorLine, e.cursorCol = 1, 4
	h := e.Window().hWnd
	imm := syscall.NewLazyDLL("imm32.dll")
	context, _, _ := imm.NewProc("ImmCreateContext").Call()
	if context == 0 {
		if os.Getenv("SILK_TEST_NATIVE_IME") != "1" {
			t.Skip("IMM context unavailable: rerun on interactive desktop with SILK_TEST_NATIVE_IME=1")
		}
		t.Fatal("ImmCreateContext failed on required interactive desktop")
	}
	previous, _, _ := imm.NewProc("ImmAssociateContext").Call(uintptr(h), context)
	defer func() {
		imm.NewProc("ImmAssociateContext").Call(uintptr(h), previous)
		imm.NewProc("ImmDestroyContext").Call(context)
	}()
	// Read back the OS state, not a stubbed setter. Explicit HIMC makes this
	// deterministic even on a non-interactive test thread with English input.
	win32.SendMessage(h, win32.WM_IME_STARTCOMPOSITION, 0, 0)
	var form struct {
		Style    uint32
		Position win32.POINT
		Area     win32.RECT
	}
	ok, _, _ := imm.NewProc("ImmGetCompositionWindow").Call(context, uintptr(unsafe.Pointer(&form)))
	if ok == 0 {
		t.Fatal("ImmGetCompositionWindow failed")
	}
	x, bottom := e.caretLocalXY()
	top := bottom - e.font.FontExtents().Height - 2
	if form.Style != 2 || math.Abs(float64(form.Position.X)-x) > 1 || math.Abs(float64(form.Position.Y)-top) > 1 {
		t.Fatalf("composition = %+v; want CFS_POINT at caret %.1f,%.1f", form, x, top)
	}
	var candidate struct {
		Index, Style uint32
		Position     win32.POINT
		Area         win32.RECT
	}
	ok, _, _ = imm.NewProc("ImmGetCandidateWindow").Call(context, 0, uintptr(unsafe.Pointer(&candidate)))
	if ok == 0 || candidate.Style != 0x80 || math.Abs(float64(candidate.Area.Bottom)-bottom) > 1 || candidate.Area.Right <= candidate.Area.Left {
		t.Fatalf("candidate exclusion = %+v, result=%d; want below caret %.1f", candidate, ok, bottom)
	}
	if e.Text() != "first\nsecond line" {
		t.Fatal("positioning IME changed editor text")
	}
	// Exercise a logically focused descendant, not just a root text widget.
	// The native anchor must include its offset but not the HWND's screen origin.
	e.NakedWidget().hidden = false
	search := NewSearchBox()
	search.SetParent(e)
	search.SetBounds(70, 110, 200, 28)
	search.SetText("query")
	search.SetFocus()
	win32.SendMessage(h, win32.WM_IME_STARTCOMPOSITION, 0, 0)
	imm.NewProc("ImmGetCompositionWindow").Call(context, uintptr(unsafe.Pointer(&form)))
	rect := search.InputMethodCursorRect()
	if math.Abs(float64(form.Position.X)-(70+rect.X)) > 1 || math.Abs(float64(form.Position.Y)-(110+rect.Y)) > 1 {
		t.Fatalf("nested search anchor=%+v, local caret=%+v", form, rect)
	}
	// IMN's lParam is a bitmask. Lists 1 and 3 must receive their own index.
	win32.SendMessage(h, win32.WM_IME_NOTIFY, 5, (1<<1)|(1<<3))
	for _, index := range []uintptr{1, 3} {
		ok, _, _ = imm.NewProc("ImmGetCandidateWindow").Call(context, index, uintptr(unsafe.Pointer(&candidate)))
		if ok == 0 || candidate.Index != uint32(index) || candidate.Style != 0x80 || candidate.Area.Left != form.Position.X {
			t.Fatalf("list %d candidate=%+v result=%d", index, candidate, ok)
		}
	}
	// Repeated context acquisition/release must not alter text or get stuck in
	// the reentrancy guard; IMM setters synchronously notify the same window.
	for i := 0; i < 100; i++ {
		if !e.Window().positionInputMethod(1) {
			t.Fatalf("position update %d failed", i)
		}
	}
	if search.Text() != "query" || e.Window().positioningIME {
		t.Fatal("IME update changed text or retained reentrancy guard")
	}
}

func TestWindowsTextInputFocusAndWindowIsolation(t *testing.T) {
	e := nativeTextInputEditor(t)
	h := e.Window().hWnd
	other := NewSearchBox()
	other.SetParent(e)
	other.SetSize(120, 30)
	// The logical root is visible without displaying its native test HWND.
	e.NakedWidget().hidden = false
	win32.SendMessage(h, win32.WM_CHAR, 0xD83D, 1)
	other.SetFocus()
	e.SetFocus()
	win32.SendMessage(h, win32.WM_CHAR, 0xDE80, 1)
	if e.Text() != "" {
		t.Fatalf("surrogate crossed focus changes: %q", e.Text())
	}
	other.SetFocus()
	win32.SendMessage(h, win32.WM_CHAR, 'a', 1)
	if other.Text() != "a" {
		t.Fatalf("nested text widget lost input: %q", other.Text())
	}
	other.SetEnabled(false)
	win32.SendMessage(h, win32.WM_CHAR, 'b', 1)
	if other.Text() != "a" {
		t.Fatal("disabled field received input")
	}
	other.SetEnabled(true)
	// A different Form must not type into the globally focused field.
	unrelated := NewLabel("other form")
	unrelated.SetVisible(false)
	unrelated.AttachWindow(WtForm)
	t.Cleanup(unrelated.Window().destroy)
	win32.SendMessage(unrelated.Window().hWnd, win32.WM_CHAR, 'z', 1)
	if other.Text() != "a" {
		t.Fatal("unrelated HWND typed into focused field")
	}
	e.SetFocus()
	win32.SendMessage(h, win32.WM_CHAR, 0xD83D, 1)
	win32.SendMessage(h, win32.WM_KILLFOCUS, 0, 0)
	win32.SendMessage(h, win32.WM_CHAR, 0xDE80, 1)
	win32.SendMessage(h, win32.WM_UNICHAR, 0x110000, 1)
	win32.SendMessage(h, win32.WM_UNICHAR, 0xD800, 1)
	if e.Text() != "" {
		t.Fatalf("invalid/abandoned text committed: %q", e.Text())
	}
}

func TestWindowsTextInputRejectsHiddenAndDisabledAncestors(t *testing.T) {
	e := nativeTextInputEditor(t)
	h := e.Window().hWnd
	grandparent, parent := NewLabel("grandparent"), NewLabel("parent")
	grandparent.SetParent(e)
	parent.SetParent(grandparent)
	field := NewSearchBox()
	field.SetParent(parent)
	field.SetSize(180, 30)
	field.SetFocus()
	for _, w := range []IWidget{field, parent, grandparent, e} {
		w.NakedWidget().hidden = true
		win32.SendMessage(h, win32.WM_CHAR, 'x', 1)
		win32.SendMessage(h, win32.WM_UNICHAR, 0x1F642, 1)
		w.NakedWidget().hidden = false
		w.NakedWidget().disabled = true
		win32.SendMessage(h, win32.WM_CHAR, 'y', 1)
		win32.SendMessage(h, win32.WM_UNICHAR, 0x1F642, 1)
		w.NakedWidget().disabled = false
		if field.Text() != "" || e.Text() != "" {
			t.Fatalf("hidden/disabled %T received input: %q/%q", w, field.Text(), e.Text())
		}
	}
	// Valid Unicode still reaches the field once its complete chain is eligible.
	win32.SendMessage(h, win32.WM_CHAR, 0x4E2D, 1)
	win32.SendMessage(h, win32.WM_UNICHAR, 0x1F642, 1)
	if field.Text() != "中🙂" {
		t.Fatalf("eligible input lost: %q", field.Text())
	}
	e.SetFocus()
	e.NakedWidget().hidden = true
	win32.SendMessage(h, win32.WM_CHAR, 'z', 1)
	if e.Text() != "" {
		t.Fatal("hidden root consumed fallback character")
	}
	e.NakedWidget().hidden = false
}
