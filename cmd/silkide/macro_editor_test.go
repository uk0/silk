package main

import (
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/macro"
)

func TestMacroWorkerCoalescesAndRetires(t *testing.T) {
	delivered := make(chan macroSnapshot, 4)
	var count atomic.Int32
	w := newMacroWorker("a.silkgo", func(s macroSnapshot, _ macro.Analysis) { delivered <- s })
	w.analyze = func(_ string, _ []byte) macro.Analysis { count.Add(1); return macro.Analysis{} }
	for i := uint64(1); i <= 1000; i++ {
		w.update(macroSnapshot{text: "latest", revision: i})
	}
	select {
	case s := <-delivered:
		if s.revision != 1000 || count.Load() != 1 {
			t.Fatal(s, count.Load())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no analysis")
	}
	w.close()
	select {
	case <-w.done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	w.update(macroSnapshot{revision: 1001})
	select {
	case <-delivered:
		t.Fatal("closed worker delivered")
	default:
	}
}
func TestMacroWorkerDropsInflightOnClose(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var delivered atomic.Int32
	w := newMacroWorker("a.silkgo", func(macroSnapshot, macro.Analysis) { delivered.Add(1) })
	w.analyze = func(_ string, _ []byte) macro.Analysis { close(started); <-release; return macro.Analysis{} }
	w.update(macroSnapshot{revision: 1})
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	w.close()
	close(release)
	select {
	case <-w.done:
	case <-time.After(time.Second):
		t.Fatal("worker did not finish")
	}
	if delivered.Load() != 0 {
		t.Fatal("retired result delivered")
	}
}
func TestMacroEditorDefinitionReferencesAndCompletionWithoutGopls(t *testing.T) {
	w := testWorkbench(t)
	oldLSP, oldRefs := globalLSP, globalReferencesPanel
	globalLSP = nil
	globalReferencesPanel = ged.NewReferencesPanel()
	t.Cleanup(func() { globalLSP = oldLSP; globalReferencesPanel = oldRefs })
	text := "package p\n//silk:macro Declare(Name)\nvar ${Name} int\n//silk:end\n//silk:expand Declare(Value)\n"
	path := writeFile(t, t.TempDir(), "types.silkgo", text)
	if !openFileInEditor(w.tabs, path) {
		t.Fatal("open")
	}
	ed := activeEditor(w.tabs)
	ed.ScrollToLineCol(4, 17)
	goToDefinitionViaLSP(w.tabs)
	if ed.CursorLine() != 1 || ed.CursorCol() != 13 {
		t.Fatalf("definition at %d:%d", ed.CursorLine(), ed.CursorCol())
	}
	findReferencesViaLSP(w.tabs)
	if got := globalReferencesPanel.Locations(); len(got) != 2 || got[0].Kind != ged.RefDeclaration || got[1].Line != 5 {
		t.Fatal(got)
	}
	ed.ReplaceAllText(strings.Replace(text, "Declare(Value)", "Dec", 1))
	ed.ScrollToLineCol(4, 17)
	if !requestMacroCompletion(path, ed) || !ed.CompletionVisible() {
		t.Fatal("macro completion unavailable")
	}
	// Drive the SDK's actual accept action, not a copied replacement algorithm.
	ed.OnKeyDown(gui.KeyEnter, false)
	if !strings.Contains(ed.Text(), "//silk:expand Declare\n") {
		t.Fatal(ed.Text())
	}
	ed.Undo()
	if !strings.Contains(ed.Text(), "//silk:expand Dec\n") {
		t.Fatal("completion not atomic", ed.Text())
	}
}
func TestMacroProblemLocationUnicodeAndSourceOwnership(t *testing.T) {
	text := "package p\nvar _ = `😀` // end\n//silk:expand Missing()\n"
	a := macro.Analyze(filepath.Join(t.TempDir(), "中文.silkgo"), []byte(text))
	rows := macroProblems("中文.silkgo", text, a)
	if len(rows) != 1 || rows[0].Line != 3 || rows[0].Col != 0 || !strings.Contains(rows[0].Message, string(diagnostic.MacroUnknown)) {
		t.Fatal(rows)
	}
	prior := []ged.Problem{{Source: ged.BuildSource, Message: "build"}, {Source: ged.ProblemSource{Kind: ged.ProblemLanguage, Doc: "go.go"}, Message: "gopls"}}
	merged := ged.MergeProblems(prior, macroProblemSource("中文.silkgo"), rows)
	if len(merged) != 3 || len(ged.MergeProblems(merged, macroProblemSource("中文.silkgo"), nil)) != 2 {
		t.Fatal("unrelated diagnostics lost")
	}
}

func TestMacroProblemColumnsAreRunesNotBytesOrUTF16(t *testing.T) {
	text := "package p\n//silk:macro M()\n// 😀 ${Unknown}\n//silk:end\n"
	a := macro.Analyze("a.silkgo", []byte(text))
	rows := macroProblems("a.silkgo", text, a)
	if len(rows) != 1 || rows[0].Line != 3 || rows[0].Col != 5 {
		t.Fatalf("wrong Unicode navigation: %+v", rows)
	}
}
