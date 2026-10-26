package main

import "github.com/uk0/silk/gui"

// Register the very same handlers used by the IDE's palette/F2/F12/Shift+F12.
// CodeEditor alone retains its independent, local SDK behavior.
func configureEditorSymbolActions(tabs *gui.TabWidget, editor *gui.CodeEditor) {
	actions := editorSymbolActions(tabs, editor, renameSymbolAtActiveEditor, goToDefinitionViaLSP, findReferencesViaLSP)
	// Reuse the owner/closed-page guard for every semantic command.
	extra := editorSymbolActions(tabs, editor, nil,
		func(t *gui.TabWidget) { requestSymbolNavigation(t, "Go to Type Definition") },
		func(t *gui.TabWidget) { requestSymbolNavigation(t, "Go to Implementation") })
	actions.GoToTypeDefinition, actions.GoToImplementation = extra.GoToDefinition, extra.FindReferences
	actions.RenameShortcut = "F2"
	editor.SetSymbolActions(actions)
}

func editorSymbolActions(tabs *gui.TabWidget, editor *gui.CodeEditor, rename, definition, references func(*gui.TabWidget)) gui.CodeEditorSymbolActions {
	bind := func(handler func(*gui.TabWidget)) func(*gui.CodeEditor) {
		return func(sender *gui.CodeEditor) {
			if sender != editor || editor == nil || handler == nil {
				return
			}
			// A menu belongs to its editor, not whichever tab became active
			// meanwhile. If its page was closed, never act on a different file.
			focusEditorTab(tabs, editor)
			if activeEditor(tabs) != editor {
				return
			}
			handler(tabs)
		}
	}
	return gui.CodeEditorSymbolActions{
		RenameSymbol: bind(rename), GoToDefinition: bind(definition), FindReferences: bind(references),
	}
}
