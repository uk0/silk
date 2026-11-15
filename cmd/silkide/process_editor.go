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

var showProcessAssetEditor = gui.ShowProcessDocumentEditor
var processAssetOpening bool // UI-thread only; one bounded decode at a time.
var startProcessAssetLoad = func(source string, done func(hmi.Project, error)) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	_, err := gui.RunAsync(ctx, nil, func(ctx context.Context) (hmi.Project, error) {
		if err := ctx.Err(); err != nil {
			return hmi.Project{}, err
		}
		return hmi.Parse([]byte(source))
	}, func(p hmi.Project, err error) { cancel(); done(p, err) })
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
		return openEditors[path] == ed && ed.FilePath() == path && ed.TextRevision() == revision && !ed.IsReadOnly() && !ed.Formatting() && visibleCaretEditor(tabs, canvas) == ed && !fileRenameAffects(path) && !workspaceSearchAffects(path)
	}
	processAssetOpening = true
	err := startProcessAssetLoad(source, func(p hmi.Project, err error) {
		processAssetOpening = false
		if !current() {
			return
		}
		if err != nil {
			report(err)
			return
		}
		report(showProcessAssetEditor(ed, p, func(next hmi.Project) error {
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
		}))
	})
	if err != nil {
		processAssetOpening = false
		report(err)
	}
}
