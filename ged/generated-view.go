package ged

import "github.com/uk0/silk/gui"

// Generated source is refreshed by SetText, never by user edit commands. Using
// the SDK read-only mode also retains Copy/Find/navigation and a safe context
// menu on every backend; a key allowlist cannot protect inherited SDK methods.
type generatedView struct{ gui.CodeEditor }

func newGeneratedView() *generatedView {
	p := new(generatedView)
	p.Init(p)
	p.SetReadOnly(true)
	return p
}

// The IDE dispatches application shortcuts before ordinary key events. Claim
// mutating document commands here so a focused preview cannot undo, format or
// rename the unrelated source document behind it. Copy/Find and app commands
// keep their normal routing; there is no second editable buffer in this view.
func (v *generatedView) HandleShortcut(key int, mods uint8) bool {
	if mods == gui.ModAction && (key == 'Z' || key == 'Y' || key == '.') {
		return true
	}
	if mods == gui.ModAction|gui.ModShift && (key == 'Z' || key == 'I') {
		return true
	}
	return mods == 0 && key == gui.KeyF2
}
