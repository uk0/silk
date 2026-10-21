package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func configureOutlineNavigation(outline *ged.CodeOutlinePanel, tabs *gui.TabWidget) {
	outline.SetNavigateCallback(func(line int) {
		if editor := activeEditor(tabs); editor != nil {
			editor.NavigateToLineCol(line, 0)
		}
	})
}
