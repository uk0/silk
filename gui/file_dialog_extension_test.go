package gui

import "testing"

func TestFileDialogExtensionValidation(t *testing.T) {
	for input, want := range map[string]string{"silkhmilibrary": "silkhmilibrary", ".JSON": "json", "SiLkHMI": "silkhmi", "7z": "7z"} {
		if got, ok := fileDialogExtension(input); !ok || got != want {
			t.Fatalf("extension %q = %q, %v", input, got, ok)
		}
	}
	for _, input := range []string{"", ".", "..json", "*.json", "json;exe", "json|*", "a/b", "a\\b", "a\x00b", "json\n", "变量", "abcdefghijklmnopqrstuvwxyz1234567"} {
		if _, ok := fileDialogExtension(input); ok {
			t.Fatalf("accepted invalid extension %q", input)
		}
		if OpenFileDialogForExtension(input) != "" || SaveFileDialogForExtension(input) != "" {
			t.Fatal("invalid extension reached native chooser")
		}
	}
}
