package main

import (
	"math"
	"path/filepath"
	"slices"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

const closedEditorLimit = 32

// UI-thread state, owned by the workbench. Retain locations, not editors,
// buffers, LSP clients or discarded text. Closing is not a recovery snapshot.
type closedEditorHistory struct{ items []editorSession }

func (h *closedEditorHistory) forget(path string) {
	h.items = slices.DeleteFunc(h.items, func(p editorSession) bool { return p.Path == path })
}

func (h *closedEditorHistory) remember(pos editorSession) {
	if pos.Path == "" {
		return
	}
	if pos.ScrollY < 0 || math.IsNaN(pos.ScrollY) || math.IsInf(pos.ScrollY, 0) {
		pos.ScrollY = 0
	}
	h.forget(pos.Path)
	if len(h.items) == closedEditorLimit {
		h.items = slices.Delete(h.items, 0, 1)
	}
	h.items = append(h.items, pos)
}

func closedEditors(tabs *gui.TabWidget) *closedEditorHistory {
	if w := globalWorkbench; w != nil && w.tabs == tabs && tabs != nil {
		return &w.closedEditors
	}
	return nil
}

func canReopenEditor(tabs *gui.TabWidget) bool {
	h := closedEditors(tabs)
	return h != nil && len(h.items) != 0
}

func forgetClosedEditor(tabs *gui.TabWidget, path string) {
	if h := closedEditors(tabs); h != nil {
		h.forget(path)
	}
}

func reopenClosedEditor(tabs *gui.TabWidget) bool {
	h := closedEditors(tabs)
	if h == nil || len(h.items) == 0 {
		silkideToast(i18n.T("No closed editor to reopen"), gui.ToastInfo)
		return false
	}
	pos := h.items[len(h.items)-1]
	// A pending rename/replace is temporary; keep the entry for retry. Do not
	// read any file on idle/menu construction, or loop over multiple failed
	// paths in one input event (e.g. disconnected network-mounted files).
	if rejectBusyFile(pos.Path) {
		return false
	}
	if !openFileInEditor(tabs, pos.Path) {
		h.forget(pos.Path)
		silkideToast(i18n.Tf("Could not reopen %s", filepath.Base(pos.Path)), gui.ToastWarning)
		return false
	}
	// The shared open path also removes manually reopened entries, reveals
	// Edit mode, focuses the page, and establishes a fresh LSP document.
	ed := activeEditor(tabs)
	ed.RestoreNavigationPosition(gui.NavPosition{FilePath: pos.Path, Line: pos.Line, Column: pos.Column, ScrollY: pos.ScrollY})
	return true
}
