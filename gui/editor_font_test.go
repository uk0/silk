package gui

import "testing"

func TestCodeEditorPlatformFontFamily(t *testing.T) {
	for goos, family := range map[string]string{"windows": "Consolas", "darwin": "Menlo", "linux": "monospace"} {
		t.Run(goos, func(t *testing.T) {
			if got := codeEditorFontFamily(goos); got != family {
				t.Fatalf("editor font = %q, want %q", got, family)
			}
		})
	}
}
