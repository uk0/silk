package gui

import (
	"strings"

	"github.com/uk0/silk/core"
)

// Decisions the two clipboard backends must not make independently.
// clipboard_windows.go drives the win32 handles and clipboard_glfw.go drives
// GLFW's string API; callers branch on what they return, so the same clipboard
// state has to produce the same answer on both — down to the error text.

// errNoClipboardWindow reports that the process has no window to associate the
// clipboard with. GLFW cannot reach the OS clipboard without one at all, while
// win32's OpenClipboard(NULL) quietly succeeds against the calling thread — so
// unguarded, a windowless Clear() wipes the user's real clipboard on Windows
// and reports success, where every other platform refuses and touches nothing.
const errNoClipboardWindow = core.StrErr("no window available")

// errClipboardTextNUL reports text neither backend can carry whole: win32's
// syscall.UTF16FromString rejects an embedded NUL with EINVAL, and GLFW hands
// the string to C as a NUL-terminated buffer, which drops everything past it.
// Refusing on both sides beats copying a silently truncated selection on one.
const errClipboardTextNUL = core.StrErr("text contains a NUL")

// clipboardTextErr rejects text no backend can put on the clipboard intact.
// Both call it before they look for a window, so the payload alone decides the
// outcome rather than which backend is running.
func clipboardTextErr(s string) error {
	if strings.IndexByte(s, 0) != -1 {
		return errClipboardTextNUL
	}
	return nil
}

// clipboardPayloadPresent reports whether a payload counts as a format that is
// really there. A zero-length string does not: GLFW's GetClipboardString
// returns "" both for an empty clipboard and for an empty string, so it can
// only ever call that absent — and win32, which can tell the two apart, must
// not answer the same clipboard state with a successful empty read.
func clipboardPayloadPresent(v interface{}) bool {
	s, ok := v.(string)
	return !ok || s != ""
}
