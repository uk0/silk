package gui

import "testing"

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
