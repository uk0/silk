package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func TestEditorGitFeedbackCodesHintsDedupAndManualRefresh(t *testing.T) {
	w := testWorkbench(t)
	oldLog, oldToast := globalLog, silkideToast
	globalLog = ged.NewLogPanel()
	defer func() { globalLog, silkideToast = oldLog, oldToast }()
	var banners []string
	silkideToast = func(s string, _ gui.ToastLevel) { banners = append(banners, s) }
	path := writeFile(t, t.TempDir(), "a.go", "package p\n")
	if !openFileInEditor(w.tabs, path) {
		t.Fatal("open")
	}
	ed := openEditors[path]
	ed.CancelGitStatus()
	baselineLogs := len(globalLog.Entries()) // opening the real editor emits its own Info row
	f := &editorGitFeedback{}
	err := diagnostic.New(diagnostic.GitUnavailable, "", errors.New("git executable missing")).At(diagnostic.Location{File: path})
	f.complete(ed, err)
	f.complete(ed, err)
	if len(banners) != 0 || len(globalLog.Entries()) != baselineLogs+1 {
		t.Fatalf("automatic feedback: banners=%v log=%+v", banners, globalLog.Entries())
	}
	if row := globalLog.Entries()[baselineLogs]; !strings.Contains(row.Message, string(err.Code)) || !strings.Contains(row.Message, err.Hint) || !strings.Contains(row.Message, err.CauseText) {
		t.Fatal("lost debug evidence", row)
	}
	editorGitManualRefresh[ed] = path
	f.complete(ed, err)
	if len(banners) != 1 || !strings.Contains(banners[0], "SILK-GIT001") || len(editorGitManualRefresh) != 0 {
		t.Fatal("explicit refresh failure missing", banners)
	}
	editorGitManualRefresh[ed] = path
	f.complete(ed, nil)
	if len(banners) != 2 || banners[1] != i18n.T("Git markers refreshed") {
		t.Fatal("manual success missing", banners)
	}
	f.complete(ed, err)
	if len(globalLog.Entries()) != baselineLogs+2 {
		t.Fatal("new failure after recovery suppressed")
	}
	editorGitManualRefresh[ed] = "old-path.go"
	f.complete(ed, nil)
	if len(banners) != 2 {
		t.Fatal("stale manual request surfaced after rebind")
	}
}
func TestEditorGitPaletteRefreshUsesActiveVisibleDocument(t *testing.T) {
	w := testWorkbench(t)
	old := paletteCommands
	defer func() { paletteCommands = old }()
	path := writeFile(t, t.TempDir(), "main.go", "package main\n")
	if !openFileInEditor(w.tabs, path) {
		t.Fatal("open")
	}
	ed := openEditors[path]
	ed.CancelGitStatus()
	registerPaletteCommands(w.tabs, w.canvas)
	var run func()
	for _, c := range paletteCommands {
		if c.Name == i18n.T("Refresh Editor Git Markers") {
			run = c.Fn
		}
	}
	if run == nil {
		t.Fatal("missing palette action")
	}
	w.activate(modeDesign)
	run()
	if ed.GitStatusLoading() {
		t.Fatal("hidden editor refreshed")
	}
	w.activate(modeEdit)
	run()
	if !ed.GitStatusLoading() || editorGitManualRefresh[ed] != path {
		t.Fatal("visible editor not queued")
	}
	ed.CancelGitStatus()
	delete(editorGitManualRefresh, ed)
}
