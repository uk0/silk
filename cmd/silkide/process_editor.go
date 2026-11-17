package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
)

var showProcessAssetEditor = gui.ShowNavigableProcessDocumentEditor
var processAssetOpening bool // UI-thread only; one decode or modal at a time.
type indexedProcessAsset struct {
	project hmi.Project
	index   *hmi.SourceIndex
}

var startProcessAssetLoad = func(source string, done func(indexedProcessAsset, error)) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	_, err := gui.RunAsync(ctx, nil, func(ctx context.Context) (indexedProcessAsset, error) {
		if err := ctx.Err(); err != nil {
			return indexedProcessAsset{}, err
		}
		p, index, err := hmi.ParseIndexedSource(source)
		if err == nil {
			err = ctx.Err()
		}
		return indexedProcessAsset{p, index}, err
	}, func(p indexedProcessAsset, err error) { cancel(); done(p, err) })
	if err != nil {
		cancel()
	}
	return err
}

func processAssetEditorAvailable(ed *gui.CodeEditor) bool {
	return ed != nil && !ed.IsReadOnly() && !ed.Formatting() && !processAssetOpening && strings.EqualFold(filepath.Ext(ed.FilePath()), ".silkhmi")
}

func bindProcessAssetEditor(tabs *gui.TabWidget, canvas *ged.GedView) paletteCommand {
	ed := visibleCaretEditor(tabs, canvas)
	return paletteCommand{Name: i18n.T("Edit 2D Process..."), Available: func() bool {
		return visibleCaretEditor(tabs, canvas) == ed && processAssetEditorAvailable(ed)
	}, Fn: func() {
		if ed != nil {
			editProcessAsset(tabs, canvas, ed)
		}
	}}
}

// Keep the source buffer as the only document authority. Visual changes become
// one normal host undo step and reuse dirty guards, Save All and session recovery.
// The modal never writes a file or starts data acquisition behind the user's back.
func editProcessAsset(tabs *gui.TabWidget, canvas *ged.GedView, expected *gui.CodeEditor) {
	ed := visibleCaretEditor(tabs, canvas)
	if !processAssetEditorAvailable(ed) || expected != nil && expected != ed {
		return
	}
	path, revision, source := ed.FilePath(), ed.TextRevision(), ed.Text()
	if rejectBusyFile(path) {
		return
	}
	report := func(err error) {
		if err != nil {
			silkideToast(i18n.T("Edit 2D Process...")+": "+err.Error(), gui.ToastWarning)
		}
	}
	if len(source) > hmi.MaxDocumentBytes {
		report(fmt.Errorf("hmi: document too large"))
		return
	}
	current := func() bool {
		return openEditors[path] == ed && ed.FilePath() == path && ed.TextRevision() == revision && !ed.IsReadOnly() && !ed.Formatting() && visibleCaretEditor(tabs, canvas) == ed && !fileRenameAffects(path) && !workspaceSearchAffects(path) && !editorCloseAffects(path)
	}
	processAssetOpening = true
	err := startProcessAssetLoad(source, func(asset indexedProcessAsset, err error) {
		defer func() { processAssetOpening = false }()
		if !current() {
			return
		}
		if err != nil {
			report(err)
			return
		}
		var initial hmi.SourceTarget
		if caret, ok := ed.CursorTextPosition(); ok {
			initial, _ = asset.index.AtPosition(caret.Line, caret.ByteColumn)
		}
		target, err := showProcessAssetEditor(ed, asset.project, func(next hmi.Project) error {
			if !current() {
				return fmt.Errorf("%s", i18n.T("Document changed; reopen the process editor"))
			}
			data, err := next.JSON()
			if err != nil {
				return err
			}
			text := string(data) + "\n"
			if ed.LineEndings().Preferred == gui.LineEndingCRLF {
				text = strings.ReplaceAll(text, "\n", "\r\n")
			}
			// A near-budget document must remain reloadable after formatting/CRLF.
			if len(text) > hmi.MaxDocumentBytes {
				text = string(data)
			}
			ed.ReplaceAllText(text)
			return nil
		}, gui.ProcessDocumentNavigation{Initial: initial, HasTarget: func(s hmi.SourceTarget) bool {
			_, _, ok := asset.index.IDPosition(s)
			return ok
		}})
		if err != nil {
			report(err)
			return
		}
		if target.ID == "" {
			return
		}
		// Never transfer a coordinate to a replacement tab/revision, even if the
		// same path and ID are still present when the native modal releases.
		if !current() {
			report(fmt.Errorf("%s", i18n.T("Document changed; reopen the process editor")))
			return
		}
		if line, col, ok := asset.index.IDPosition(target); ok {
			from := ed.NavigationPosition()
			ed.NavigateToLineCol(line, col)
			if history := editorNavigation(tabs); history != nil && current() {
				history.RecordJump(from, ed.NavigationPosition())
			}
		}
	})
	if err != nil {
		processAssetOpening = false
		report(err)
	}
}
