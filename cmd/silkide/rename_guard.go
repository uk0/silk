package main

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// Capture on the UI thread, evaluate only on the UI thread. Text revision also
// rejects edit/undo ABA; ownership, path and editor-map identity reject reuse.
// Before submitting a modal prompt, moving the caret changes the target. After
// an accepted RPC starts, ordinary navigation is permitted, not document edits.
func renameEditorGuard(ed *gui.CodeEditor, path string, caret bool) func() bool {
	revision := ed.TextRevision()
	parent, owner := ed.Parent(), ed.OwnerWindow()
	line, col := ed.CursorLine(), ed.CursorCol()
	return func() bool {
		return openEditors[path] == ed && ed.FilePath() == path && !ed.IsReadOnly() && ed.IsEnabled() &&
			ed.TextRevision() == revision && ed.Parent() == parent && ed.OwnerWindow() == owner &&
			(!caret || ed.CursorLine() == line && ed.CursorCol() == col)
	}
}

type renameBufferSnapshot struct {
	text    string
	doc     *lspDocument
	current func() bool
}

// Immutable strings and transport pointers may cross into the RPC worker;
// current callbacks must stay on the UI thread. No document copies or polling.
func captureRenameBuffers() map[string]renameBufferSnapshot {
	out := make(map[string]renameBufferSnapshot, len(openEditors))
	for path, e := range openEditors {
		if e != nil {
			out[path] = renameBufferSnapshot{e.Text(), lspDocuments[path], renameEditorGuard(e, path, false)}
		}
	}
	return out
}
func syncRenameBuffers(buffers map[string]renameBufferSnapshot) bool {
	for _, s := range buffers {
		if s.doc == nil {
			continue
		}
		got, err := s.doc.sync()
		if err != nil || got.text != s.text {
			return false
		}
	}
	return true
}
func renameTargetsCurrent(edit *core.LSPWorkspaceEdit, buffers map[string]renameBufferSnapshot) bool {
	if edit == nil {
		return false
	}
	for uri := range edit.Changes {
		path := uriToPath(uri)
		if before, ok := buffers[path]; ok {
			if !before.current() {
				return false
			}
		} else if openEditors[path] != nil {
			return false
		}
	}
	return true
}
