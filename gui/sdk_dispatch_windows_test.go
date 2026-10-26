//go:build windows

package gui

import (
	"errors"
	"github.com/uk0/silk/win32"
	"runtime"
	"testing"
)

func TestSDKDispatchWindowsCreationRejectsWorkerBeforeNativeState(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	old := uiThreadOwner
	owner := win32.GetCurrentThreadId()
	uiThreadOwner = func() bool { return win32.GetCurrentThreadId() == owner }
	defer func() { uiThreadOwner = old }()
	result := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		w := &Window{}
		err := w.create(nil, WtPopup)
		if w.hWnd != 0 || w.popupOwner != nil || w.popupRestoreFocus != nil {
			result <- errors.New("worker changed native state")
			return
		}
		result <- err
	}()
	if err := dispatchWait(t, result); !errors.Is(err, ErrUIThread) {
		t.Fatalf("worker was not rejected: %v", err)
	}
}

func TestSDKDispatchWindowsCheckedAttachHasRealOwner(t *testing.T) {
	isolateWindowsPopupState(t)
	parent := NewForm()
	parent.SetVisible(false)
	if err := parent.TryAttachWindow(WtForm); err != nil {
		t.Fatal(err)
	}
	defer parent.Window().destroy()
	child := NewForm()
	child.SetParent(parent)
	child.SetVisible(false)
	if err := child.TryAttachWindow(WtForm); err != nil {
		t.Fatal(err)
	}
	defer child.Window().destroy()
	if child.Window().ParentWindow() != parent.Window() {
		t.Fatal("native owner lost")
	}
	if tid, _ := win32.GetWindowThreadProcessId(child.Window().hWnd); uint32(tid) != win32.GetCurrentThreadId() {
		t.Fatal("wrong native owner thread", tid)
	}
}
