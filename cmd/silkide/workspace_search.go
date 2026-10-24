package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

// Reservations are UI-thread-owned. Disk work never accesses editors.
var workspaceSearchReserved map[string]bool

func workspaceSearchKey(path string) string {
	path = canonicalEditorPath(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}
func workspaceSearchBusy() bool { return len(workspaceSearchReserved) != 0 }
func workspaceSearchAffects(path string) bool {
	return workspaceSearchReserved[workspaceSearchKey(path)]
}

func configureWorkspaceSearch(p *ged.GlobalSearchPanel, tabs *gui.TabWidget, canvas *ged.GedView) {
	p.SigOpenMatch(func(m ged.SearchMatch) {
		navigateEditor(tabs, m.FilePath, func(ed *gui.CodeEditor) {
			if !navigateSearchMatch(ed, m) {
				silkideToast("Search result is stale; search again before jumping", gui.ToastWarning)
			}
		})
	})
	p.SigPreviewReplacement(func(r ged.FileReplacement) {
		showDiffDialog("Replace: "+filepath.Base(r.Path), func(context.Context) (diffSource, error) { return diffSource{r.Before, r.After}, nil })
	})
	p.SetReplacementGuard(func(reps []ged.FileReplacement) (func(error), error) {
		return prepareWorkspaceReplacement(tabs, canvas, reps)
	})
}

func navigateSearchMatch(ed *gui.CodeEditor, m ged.SearchMatch) bool {
	lines := ed.Lines()
	if m.Line < 1 || m.Line > len(lines) {
		return false
	}
	line := strings.TrimSuffix(lines[m.Line-1], "\r")
	if line != m.LineText || m.Column < 0 || m.Column > len(line) || !utf8.ValidString(line[:m.Column]) {
		return false
	}
	ed.NavigateToLineCol(m.Line-1, utf8.RuneCountInString(line[:m.Column]))
	return true
}

type workspaceReservedEditor struct {
	path          string
	editor        *gui.CodeEditor
	before, after string
	revision      uint64
	enabled       bool
}

func prepareWorkspaceReplacement(tabs *gui.TabWidget, canvas *ged.GedView, reps []ged.FileReplacement) (func(error), error) {
	if workspaceSearchBusy() || activeFileRename != nil || macroBusy || globalDebug != nil || globalDebugAttempt != nil {
		return nil, fmt.Errorf("finish the current file operation, macro expansion or debug session before replacing")
	}
	reserved := make(map[string]bool, len(reps))
	var editors []workspaceReservedEditor
	for _, r := range reps {
		key := workspaceSearchKey(r.Path)
		if key == "" || reserved[key] {
			return nil, fmt.Errorf("invalid or duplicate replacement path: %s", r.Path)
		}
		reserved[key] = true
		// A live designer also owns autosave and generated companion files. Until
		// those paths participate in this transaction, keep its directory untouched.
		if canvas != nil && canvas.GedScene() != nil {
			design := canvas.GedScene().Filename()
			if design != "" && workspaceSearchKey(filepath.Dir(design)) == workspaceSearchKey(filepath.Dir(r.Path)) {
				return nil, fmt.Errorf("close the design document before replacing files in its directory")
			}
		}
		for path, ed := range openEditors {
			if ed == nil || workspaceSearchKey(path) != key {
				continue
			}
			if ed.Modified() || ed.Text() != r.Before {
				return nil, fmt.Errorf("save or discard the changed buffer and preview again: %s", path)
			}
			editors = append(editors, workspaceReservedEditor{path, ed, r.Before, r.After, ed.TextRevision(), ed.IsEnabled()})
		}
	}
	workspaceSearchReserved = reserved
	for _, e := range editors {
		e.editor.SetEnabled(false)
		e.editor.CancelCompletion()
	}
	released := false
	return func(err error) {
		if released {
			return
		}
		released = true
		defer func() {
			workspaceSearchReserved = nil
			for _, e := range editors {
				e.editor.SetEnabled(e.enabled)
			}
		}()
		for _, e := range editors {
			if err != nil || openEditors[e.path] != e.editor {
				continue
			}
			if e.editor.TextRevision() != e.revision || e.editor.Text() != e.before {
				silkideToast("Disk replacement finished; a newer buffer was preserved: "+filepath.Base(e.path), gui.ToastWarning)
				continue
			}
			e.editor.ReplaceAllText(e.after) // one undo step; normal text signal updates LSP
			e.editor.MarkSaved()
			e.editor.RefreshGitStatus()
			refreshEditorTab(tabs, e.editor, e.path)
		}
	}, nil
}
