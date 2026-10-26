//go:build darwin && cgo

package gui

/*
#cgo LDFLAGS: -framework Cocoa
void *silkBeginFileDialog(void *owner, int kind);
int silkFileDialogDone(void *session);
const char *silkFileDialogPath(void *session);
void silkCancelFileDialog(void *session);
void silkReleaseFileDialog(void *session);
void silkRestoreFileDialogFocus(void *session);
*/
import "C"

import (
	"unsafe"

	"github.com/uk0/silk/core"
)

type cocoaFileDialogSession struct {
	handle unsafe.Pointer
	owner  *Window
}

func (s *cocoaFileDialogSession) result() (string, bool) {
	if C.silkFileDialogDone(s.handle) == 0 {
		return "", false
	}
	return C.GoString(C.silkFileDialogPath(s.handle)), true
}

func (s *cocoaFileDialogSession) cancel() { C.silkCancelFileDialog(s.handle) }
func (s *cocoaFileDialogSession) close() {
	if fileDialogOwnerCanRestoreFocus(s.owner) {
		C.silkRestoreFileDialogFocus(s.handle)
	}
	C.silkReleaseFileDialog(s.handle)
	s.handle = nil
}

func platformOpenFileDialog() string { return runCocoaFileDialog(nativeFileDialogOpen) }
func platformSaveFileDialog() string { return runCocoaFileDialog(nativeFileDialogSave) }

func runCocoaFileDialog(kind nativeFileDialogKind) string {
	if err := nativeDesktopError("file dialog"); err != nil {
		core.Warn(err)
		return ""
	}
	if uiThreadOwner == nil || !uiThreadOwner() {
		core.Warn("file dialog: native Cocoa panels must be opened on the UI thread")
		return ""
	}
	owner := cocoaFileDialogOwner()
	if owner == nil {
		core.Warn("file dialog: no visible native owner window")
		return ""
	}
	owner.dismissOwnedPopups()
	handle := C.silkBeginFileDialog(owner.glfwWin.GetCocoaWindow(), C.int(kind))
	if handle == nil {
		core.Warn("file dialog: cannot attach native sheet (owner unavailable or already has a sheet)")
		return ""
	}
	session := &cocoaFileDialogSession{handle: handle, owner: owner}
	return waitNativeFileDialog(owner, session, func() bool {
		return owner.widget != nil && owner.glfwWin != nil && owner.IsVisible()
	})
}

func cocoaFileDialogOwner() *Window {
	var focused *Window
	if focusWidget != nil {
		focused = focusWidget.OwnerWindow()
	}
	candidates := []*Window{focused, activeForm, pointerWindow}
	for _, win := range winMap {
		candidates = append(candidates, win)
	}
	for _, win := range candidates {
		for win != nil && win.wt == WtPopup && win.popupOwner != win {
			win = win.popupOwner
		}
		if win != nil && win.wt == WtForm && win.widget != nil && win.glfwWin != nil && win.IsVisible() {
			return win
		}
	}
	return nil
}
