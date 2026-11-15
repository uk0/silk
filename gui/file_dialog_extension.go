package gui

import "strings"

// OpenFileDialogForExtension opens an owned native chooser for one extension
// (with or without a leading dot). Extensions contain 1-32 ASCII letters/digits.
// Like the legacy file dialogs, it must be called on the UI thread.
func OpenFileDialogForExtension(extension string) string {
	ext, ok := fileDialogExtension(extension)
	if !ok {
		return ""
	}
	return platformOpenExtensionDialog(ext)
}

// SaveFileDialogForExtension avoids the legacy SilkUI default/filter appending
// .silkui to non-form assets. Cancellation or invalid extension returns empty.
func SaveFileDialogForExtension(extension string) string {
	ext, ok := fileDialogExtension(extension)
	if !ok {
		return ""
	}
	return platformSaveExtensionDialog(ext)
}

func fileDialogExtension(extension string) (string, bool) {
	s := strings.TrimPrefix(extension, ".")
	if len(s) == 0 || len(s) > 32 {
		return "", false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return "", false
		}
	}
	return strings.ToLower(s), true
}
