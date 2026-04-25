package gui

import (
	"strconv"
)

// InputText displays a text input dialog with the given title, prompt, and default value.
// Returns the entered text and true if OK was clicked, or empty string and false if cancelled.
func InputText(parent IWidget, title, prompt, defaultValue string) (string, bool) {
	return ShowInputDialog(parent, title, prompt, defaultValue)
}

// InputNumber displays a number input dialog with the given title, prompt, and default value.
// Returns the entered number and true if OK was clicked, or 0 and false if cancelled.
func InputNumber(parent IWidget, title, prompt string, defaultValue float64) (float64, bool) {
	defStr := strconv.FormatFloat(defaultValue, 'f', -1, 64)
	text, ok := ShowInputDialog(parent, title, prompt, defStr)
	if !ok {
		return 0, false
	}
	val, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	return val, true
}

// Confirm displays a confirmation dialog with Yes/No buttons.
// Returns true if the user clicked Yes.
func Confirm(parent IWidget, title, message string) bool {
	return ShowConfirmDialog(parent, title, message)
}
