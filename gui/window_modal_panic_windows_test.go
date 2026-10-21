//go:build windows

package gui

import (
	"errors"
	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/win32"
	"testing"
)

func TestWindowsModalShowPanicReleasesOwner(t *testing.T) {
	editor := nativeModalOwner(t)
	owner := editor.Window()
	dlg := NewDialog("Failing initialization", editor)
	input := NewEdit()
	dlg.SetContent(input)
	dlg.SetVisible(false)
	dlg.AttachWindow(WtForm)
	win := dlg.Window()
	// Also clean up the unpatched failure without leaving a disabled HWND.
	t.Cleanup(func() { win.inModal = false; win.destroy(); owner.SetEnabled(true) })
	var escaped any
	func() {
		defer func() { escaped = recover() }()
		win.ShowModal(func() { input.SetFocus(); panic("modal initialization probe") })
	}()
	if escaped != nil || win.inModal || win.hWnd != 0 || !owner.IsEnabled() {
		t.Fatalf("show panic escaped=%v; modal=%v hwnd=%v ownerEnabled=%v", escaped, win.inModal, win.hWnd, owner.IsEnabled())
	}
	if diagnostic.CodeOf(win.ModalError()) != diagnostic.GUIModalPanic {
		t.Fatalf("missing modal diagnostic: %v", win.ModalError())
	}
	if focusWidget != editor {
		t.Fatalf("input not restored after panic: %T", focusWidget)
	}
	win32.SendMessage(owner.hWnd, win32.WM_CHAR, 'x', 1)
	if editor.Text() != "x" {
		t.Fatalf("owner cannot type after show failure: %q", editor.Text())
	}
}

type modalTestInitialFocus struct {
	Edit
	run func()
}

func (e *modalTestInitialFocus) OnFocusChanged(newFocus, oldFocus IWidget) {
	if newFocus == e {
		e.run()
	}
}

func TestWindowsDialogReportsInitializationFailure(t *testing.T) {
	editor := nativeModalOwner(t)
	dlg := NewDialog("Failing focus", editor)
	cause := errors.New("focus initialization probe")
	input := &modalTestInitialFocus{run: func() { dlg.Window().EndModal(DialogOK); panic(cause) }}
	input.Init(input)
	dlg.SetContent(input)
	dlg.SetInitialFocus(input)
	result := dlg.ShowModal()
	if result != DialogCancel || !errors.Is(dlg.LastError(), cause) || diagnostic.CodeOf(dlg.LastError()) != diagnostic.GUIModalPanic {
		t.Fatalf("result=%v diagnostic=%v", result, dlg.LastError())
	}
	if !editor.Window().IsEnabled() || focusWidget != editor {
		t.Fatal("dialog failure retained modal input lock")
	}
	// Reusing the dialog must clear the old failure and return a normal cancel.
	normal := &modalTestInitialFocus{run: func() { dlg.Window().EndModal(DialogCancel) }}
	normal.Init(normal)
	dlg.SetContent(normal)
	dlg.SetInitialFocus(normal)
	if result := dlg.ShowModal(); result != DialogCancel || dlg.LastError() != nil {
		t.Fatalf("reused dialog kept stale failure: %v %v", result, dlg.LastError())
	}
}
