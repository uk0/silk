package main

import (
	"path/filepath"
	"strings"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

// Resolve both page and file identity at execution, not the numeric slot or
// path captured when the popup opened. A background rename or another close
// must never redirect an old menu action to a different editor.
func resolveEditorTabTarget(tabs *gui.TabWidget, ed *gui.CodeEditor) (int, string) {
	if tabs == nil || ed == nil {
		return -1, ""
	}
	for index := 0; index < tabs.Count(); index++ {
		if tabs.Stack().Page(index) == ed {
			for path, current := range openEditors {
				if current == ed && ed.FilePath() == path {
					return index, path
				}
			}
			return index, ""
		}
	}
	return -1, ""
}

var copyEditorTabPath = func(path string) error {
	_, err := gui.Clipboard.SetData(path)
	return err
}

var revealEditorTabPath = func(path string) bool { return globalFileExplorer.RevealPath(path) }

func canRevealEditorTab(path string) bool {
	if path == "" || globalFileExplorer == nil || globalFileExplorer.RootDir() == "" {
		return false
	}
	relative, err := filepath.Rel(globalFileExplorer.RootDir(), path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func buildEditorTabContextMenu(menu *gui.Menu, tabs *gui.TabWidget, index int) {
	if tabs == nil || index < 0 || index >= tabs.Count() {
		return
	}
	ed, ok := tabs.Stack().Page(index).(*gui.CodeEditor)
	if !ok {
		return
	}
	_, path := resolveEditorTabTarget(tabs, ed)
	header := menu.AddButton1(tabs.TabTitle(index), paint.LoadIcon("document"))
	header.SetEnabled(false)
	menu.AddSeparator()
	// Popup entries display their whole label; no global tooltip registration.
	// Rebuilding this owned menu does not retain detached buttons or editors.
	add := func(id, label, icon string, enabled bool, run func()) {
		b := menu.AddButton1(i18n.T(label), paint.LoadIcon(icon))
		b.Action().SetObjName(id)
		b.SetEnabled(enabled)
		b.Action().SetEnabled(enabled)
		b.Action().BindFunc0(run)
	}
	add("tab.save", "Save", "save", path != "", func() {
		if _, live := resolveEditorTabTarget(tabs, ed); live != "" {
			saveEditorToDisk(ed, live)
		}
	})
	add("tab.close", "Close Editor", "close", path != "", func() {
		if idx, live := resolveEditorTabTarget(tabs, ed); idx >= 0 && live != "" && closeEditorTab(tabs, idx) {
			if current := activeEditor(tabs); current != nil {
				focusEditorTab(tabs, current)
			}
		}
	})
	for _, spec := range []struct {
		id, label string
		mode      editorCloseMode
	}{
		{"tab.closeOthers", "Close Other Editors", closeOtherEditors},
		{"tab.closeRight", "Close Editors to the Right", closeRightEditors},
		{"tab.closeAll", "Close All Editors", closeAllEditors},
	} {
		add(spec.id, spec.label, "close", activeEditorClose == nil && path != "" && len(editorCloseTargets(tabs, ed, spec.mode)) > 0, func() {
			if _, live := resolveEditorTabTarget(tabs, ed); live != "" {
				startEditorBatchClose(tabs, ed, spec.mode)
			}
		})
	}
	if activeEditorClose != nil {
		add("tab.cancelClose", "Cancel Closing Editors", "stop", !activeEditorClose.cancelled, cancelEditorBatchClose)
	}
	add("tab.reopen", "Reopen Closed Editor", "edit-undo", canReopenEditor(tabs), func() { reopenClosedEditor(tabs) })
	menu.AddSeparator()
	add("tab.copyPath", "Copy Full Path", "edit-copy", path != "", func() {
		if _, live := resolveEditorTabTarget(tabs, ed); live != "" {
			if err := copyEditorTabPath(live); err != nil {
				silkideToast(i18n.Tf("Could not copy file path: %v", err), gui.ToastError)
			}
		}
	})
	add("tab.reveal", "Reveal File in Explorer", "folder", canRevealEditorTab(path), func() {
		_, live := resolveEditorTabTarget(tabs, ed)
		if canRevealEditorTab(live) && revealEditorTabPath(live) {
			revealWorkbenchTool(globalLeftDock, globalFileExplorer)
			globalFileExplorer.SetFocus()
		}
	})
}
