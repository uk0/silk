package gui

import (
	"os"
	"strings"
	"testing"
)

// The clipboard has two independent implementations — clipboard_windows.go
// (win32 handles) and clipboard_glfw.go (GLFW string + a process-local map)
// — and every test here pins a contract both must honour, on every GOOS.

// TestClipboardDataAbsentFormatReturnsError guards the paste path. Edit.paste
// does
//
//	i, err := Clipboard.Data("text/plain")
//	if err == nil { this.pasteString(i.(string)) }
//
// so a (nil data, nil error) return is not a miss, it is a panic:
// "interface conversion: interface {} is nil, not string". The Windows
// backend fell out of its format loop with both values nil whenever the
// requested mime was absent — i.e. Ctrl+V on an empty clipboard crashed the
// editor there while the GLFW backend returned a plain error.
func TestClipboardDataAbsentFormatReturnsError(t *testing.T) {
	data, err := Clipboard.Data("application/x-silk-absent")
	if err == nil {
		t.Fatalf("Data(absent format) = %v, nil; want a non-nil error", data)
	}
	if data != nil {
		t.Fatalf("Data(absent format) = %v, %v; want nil data alongside the error", data, err)
	}
}

// TestClipboardSetDataRejectsUnsupportedType keeps both backends refusing
// payloads neither can put on a real clipboard. The GLFW backend used to
// swallow them into a process-local "application/octet-stream" slot nothing
// ever reads and report success, so TestResultsPanel.clipboardWrite — which
// branches on the error — logged a warning on Windows and stayed silent on
// macOS for the very same value.
func TestClipboardSetDataRejectsUnsupportedType(t *testing.T) {
	format, err := Clipboard.SetData(42)
	if err == nil {
		t.Fatalf("SetData(int) = %q, nil; want a non-nil error", format)
	}
	if format != "" {
		t.Fatalf("SetData(int) format = %q; want an empty format alongside the error", format)
	}
}

// TestClipboardFormatsAreDistinctMimes pins the shape of the format list.
// The Windows backend enumerated raw clipboard ids and appended
// clipboardIdToFormat's "" miss for every id it has no mime for (CF_LOCALE,
// CF_OEMTEXT, HTML Format, ...), and reported text/plain twice because
// Windows synthesises CF_TEXT next to CF_UNICODETEXT and both map to it — a
// caller iterating the result then asked Data("") for a format that cannot
// exist. GLFW can only ever produce a clean, distinct list.
//
// The error is ignored on purpose: headless there is no GLFW window, so that
// backend reports "no window available" and the assertions run against
// whatever it could still list.
func TestClipboardFormatsAreDistinctMimes(t *testing.T) {
	formats, _ := Clipboard.Formats()
	seen := make(map[string]bool, len(formats))
	for _, f := range formats {
		if f == "" {
			t.Fatalf("Formats() = %q; an unknown clipboard id leaked in as an empty mime", formats)
		}
		if seen[f] {
			t.Fatalf("Formats() = %q; %q is listed more than once", formats, f)
		}
		seen[f] = true
	}
}

// TestClipboardSetDataRejectsEmbeddedNUL pins the one text payload neither
// backend can carry whole. syscall.UTF16FromString hands win32 back EINVAL and
// nothing is copied; GLFW passes the string to C as a NUL-terminated buffer,
// so glfwSetClipboardString stores only the head and SetData still reports
// ("text/plain", nil) — copying a selection out of a file with an embedded
// 0x00 silently truncated the clipboard on one platform and failed on the
// other. Both refuse now, with the same error, and both refuse before they
// look for a window so the payload alone decides.
func TestClipboardSetDataRejectsEmbeddedNUL(t *testing.T) {
	format, err := Clipboard.SetData("a\x00b")
	if err != errClipboardTextNUL {
		t.Fatalf("SetData(text with a NUL) = %q, %v; want the %v both backends share", format, err, errClipboardTextNUL)
	}
	if format != "" {
		t.Fatalf("SetData(text with a NUL) format = %q; want an empty format alongside the error", format)
	}
	if err := clipboardTextErr("ab"); err != nil {
		t.Fatalf("clipboardTextErr(%q) = %v; want nil — only an embedded NUL is unencodable", "ab", err)
	}
}

// TestClipboardEmptyPayloadReadsAsAbsent pins the rule Data and Formats share.
// GLFW's GetClipboardString returns "" both for an empty clipboard and for a
// zero-length string, so it can only report that absent. win32 can tell them
// apart — IsClipboardFormatAvailable(CF_UNICODETEXT) is true for a zero-length
// CF_UNICODETEXT — and used to return ("", nil), a successful read, where GLFW
// returned an error for the very same clipboard state.
func TestClipboardEmptyPayloadReadsAsAbsent(t *testing.T) {
	if clipboardPayloadPresent("") {
		t.Fatal(`clipboardPayloadPresent("") = true; empty text must read as absent, the only answer GLFW can give`)
	}
	if !clipboardPayloadPresent("x") {
		t.Fatal(`clipboardPayloadPresent("x") = false; want non-empty text reported as present`)
	}
	if !clipboardPayloadPresent([]byte{}) {
		t.Fatal("clipboardPayloadPresent([]byte{}) = false; the empty-means-absent rule covers text payloads only")
	}
}

// TestClipboardClearWithoutWindowDoesNotSucceed stops a windowless Clear from
// destroying the user's clipboard behind their back. With no window
// AnyWindowId() is 0, and win32's OpenClipboard(NULL) still succeeds against
// the calling thread: EmptyClipboard() then wiped the real system clipboard
// and Clear() reported nil, while GLFW — which cannot touch the clipboard at
// all in that state — refused and left it alone.
//
// Only the win32 backend can fail this; GLFW has always refused. It runs on
// every GOOS because the contract belongs to both.
func TestClipboardClearWithoutWindowDoesNotSucceed(t *testing.T) {
	if AnyWindowId() != 0 {
		t.Skip("a window exists, so the windowless contract does not apply")
	}
	if err := Clipboard.Clear(); err != errNoClipboardWindow {
		t.Fatalf("Clear() with no window = %v; want %v", err, errNoClipboardWindow)
	}
}

// TestWin32ReadsAnEmptyTextPayloadAsAbsent covers the half of the empty-payload
// rule that a macOS host cannot execute. clipboardPayloadPresent is a pure
// function written alongside its own test, so that test cannot go red against
// any earlier code; the substance of the fix is that the win32 read loop
// consults it and keeps looking, and that Formats() hides what Data() would
// then refuse. Removing either guard puts the backends back to answering one
// clipboard state two different ways, which is exactly what this pins.
func TestWin32ReadsAnEmptyTextPayloadAsAbsent(t *testing.T) {
	src, err := os.ReadFile("clipboard_windows.go")
	if err != nil {
		t.Fatalf("cannot read the win32 clipboard backend: %v", err)
	}
	body := funcBodyIn(t, string(src), "func (this *clipBoard) Data(format string)")

	guard := strings.Index(body, "clipboardPayloadPresent(")
	if guard < 0 {
		t.Fatal("the win32 read loop no longer applies the empty-payload rule; a zero-length CF_UNICODETEXT reads as a successful paste while GLFW reports nothing there")
	}
	if !strings.Contains(body[guard:], "continue") {
		t.Error("the win32 read loop stops at the empty payload instead of looking at the remaining formats")
	}
	if !strings.Contains(string(src), "func clipboardHasData(") {
		t.Error("Formats() no longer filters what Data() would refuse; it advertises a format the next read rejects")
	}
}

// funcBodyIn returns the body of the named function in src.
func funcBodyIn(t *testing.T, src, header string) string {
	t.Helper()
	start := strings.Index(src, header)
	if start < 0 {
		t.Fatalf("source does not contain %q", header)
	}
	rest := src[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatalf("%q is not terminated", header)
	}
	return rest[:end]
}
