package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func configureOutlineNavigation(outline *ged.CodeOutlinePanel, tabs *gui.TabWidget) {
	outline.SetNavigateCallback(func(line int) {
		if editor := activeEditor(tabs); editor != nil {
			if path := editor.FilePath(); path != "" {
				openFileInEditorAt(tabs, path, line+1, 0)
			} else {
				editor.NavigateToLineCol(line, 0)
			}
		}
	})
}
