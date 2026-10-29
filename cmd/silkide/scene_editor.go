package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/scene3d"
)

var showSceneAssetEditor = gui.ShowSceneDocumentEditor
var sceneAssetOpening bool // UI-thread only; one bounded decode at a time.
var startSceneAssetLoad = func(source string, done func(*scene3d.Document, error)) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	_, err := gui.RunAsync(ctx, nil, func(ctx context.Context) (*scene3d.Document, error) {
		return scene3d.LoadDocument(ctx, strings.NewReader(source))
	}, func(d *scene3d.Document, err error) { cancel(); done(d, err) })
	if err != nil {
		cancel()
	}
	return err
}

func sceneAssetEditorAvailable(ed *gui.CodeEditor) bool {
	return ed != nil && !ed.IsReadOnly() && !ed.Formatting() && !sceneAssetOpening && strings.EqualFold(filepath.Ext(ed.FilePath()), ".silkscene")
}
func bindSceneAssetEditor(tabs *gui.TabWidget, canvas *ged.GedView) paletteCommand {
	ed := visibleCaretEditor(tabs, canvas)
	return paletteCommand{Name: i18n.T("Edit 3D Scene..."), Available: func() bool { return visibleCaretEditor(tabs, canvas) == ed && sceneAssetEditorAvailable(ed) }, Fn: func() {
		if ed != nil {
			editSceneAsset(tabs, canvas, ed)
		}
	}}
}

// The text buffer remains the single document authority: structured edits use
// one ordinary undo transaction and reuse save-all, close guards and recovery.
// No independent editor tab state, hidden file write or live driver connection.
func editSceneAsset(tabs *gui.TabWidget, canvas *ged.GedView, expected *gui.CodeEditor) {
	ed := visibleCaretEditor(tabs, canvas)
	if !sceneAssetEditorAvailable(ed) || expected != nil && ed != expected {
		return
	}
	path, revision, source := ed.FilePath(), ed.TextRevision(), ed.Text()
	if rejectBusyFile(path) {
		return
	}
	report := func(err error) {
		if err != nil {
			silkideToast(i18n.T("Edit 3D Scene...")+": "+err.Error(), gui.ToastWarning)
		}
	}
	if len(source) > scene3d.MaxDocumentBytes {
		report(scene3d.ErrLimit)
		return
	}
	current := func() bool {
		return openEditors[path] == ed && ed.FilePath() == path && ed.TextRevision() == revision && !ed.IsReadOnly() && !ed.Formatting() && visibleCaretEditor(tabs, canvas) == ed && !fileRenameAffects(path) && !workspaceSearchAffects(path)
	}
	sceneAssetOpening = true
	err := startSceneAssetLoad(source, func(d *scene3d.Document, err error) {
		sceneAssetOpening = false
		if !current() {
			return
		} // A late completion cannot open over a different tab.
		if err != nil {
			report(err)
			return
		}
		report(showSceneAssetEditor(ed, d, func(next *scene3d.Document) error {
			if !current() {
				return fmt.Errorf("%s", i18n.T("Document changed; reopen the scene editor"))
			}
			var out bytes.Buffer
			if err := scene3d.SaveDocument(context.Background(), &out, next); err != nil {
				return err
			}
			// Format without a second unbounded decoder; SaveDocument already validated.
			var formatted bytes.Buffer
			if err := json.Indent(&formatted, bytes.TrimSpace(out.Bytes()), "", "  "); err != nil {
				return err
			}
			text := formatted.String() + "\n"
			if ed.LineEndings().Preferred == gui.LineEndingCRLF {
				text = strings.ReplaceAll(text, "\n", "\r\n")
			}
			// Keep a near-limit asset reloadable even when pretty printing or CRLF
			// would exceed the bounded document format.
			if len(text) > scene3d.MaxDocumentBytes {
				text = out.String()
			}
			ed.ReplaceAllText(text)
			return nil
		}))
	})
	if err != nil {
		sceneAssetOpening = false
		report(err)
	}
}
