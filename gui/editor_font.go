package gui

// Use native/generic families that the platform can actually resolve. Asking
// Windows for Menlo falls back to a proportional font and disables the editor's
// measured ASCII fast path. Arbitrary SetFont overrides still use real metrics.
func codeEditorFontFamily(goos string) string {
	switch goos {
	case "windows":
		return "Consolas"
	case "darwin":
		return "Menlo"
	default:
		return "monospace"
	}
}
