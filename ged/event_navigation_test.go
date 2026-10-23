package ged

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/gui"
)

func TestEventHandlerCreateNavigateMultiEventAndUndo(t *testing.T) {
	v := NewGedView()
	s := v.GedScene()
	s.SetFormTitle("Events")
	w := addFakeTo(t, s, "gui.SearchBox", "search")
	p := NewCodePanel()
	p.BindGedView(v)
	defer p.BindGedView(nil)
	if err := p.OpenEventHandler(w, "OnSearch"); err != nil {
		t.Fatal(err)
	}
	first := w.GetCode()
	if w.EventHandlers()["OnSearch"] != "onSearchSearch" || s.UndoStack().Count() != 1 {
		t.Fatal("handler/binding not one transaction")
	}
	if err := p.OpenEventHandler(w, "OnTextChanged"); err != nil {
		t.Fatal(err)
	}
	name := w.EventHandlers()["OnTextChanged"]
	_, line, col, err := findHandler(w.GetCode(), name)
	if err != nil || p.editor.CursorLine() != line || p.editor.CursorCol() != col || p.tabs.Selected() != codeTabHandler {
		t.Fatal("navigation is not exact")
	}
	count := s.UndoStack().Count()
	code := w.GetCode()
	if err := p.OpenEventHandler(w, "OnTextChanged"); err != nil || s.UndoStack().Count() != count || w.GetCode() != code {
		t.Fatal("reopen appended duplicate handler")
	}
	v.Undo()
	if w.GetCode() != first || w.EventHandlers()["OnTextChanged"] != "" {
		t.Fatal("undo split binding from body")
	}
	v.Redo()
	if w.GetCode() != code {
		t.Fatal("redo lost second handler")
	}
	result := s.GenerateCodeIndexed(CodeGenOptions{PackageName: "probe", TypeName: "RenamedUI"})
	if result.Err != nil || strings.Contains(result.Code, "*EventsUI") || !strings.Contains(result.Code, "ui."+name+"(s)") {
		t.Fatalf("multi-event preview failed: %v\n%s", result.Err, result.Code)
	}
}

func TestEventBindingValidationAndSharedHandler(t *testing.T) {
	v := NewGedView()
	s := v.GedScene()
	a := addFakeTo(t, s, "gui.Button", "a")
	b := addFakeTo(t, s, "gui.Button", "b")
	p := NewCodePanel()
	p.BindGedView(v)
	defer p.BindGedView(nil)
	if err := p.OpenEventHandler(a, "OnClick"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEventBinding(b, "OnClick", a.EventHandlers()["OnClick"]); err != nil {
		t.Fatal(err)
	}
	if err := p.OpenEventHandler(b, "OnClick"); err != nil || p.currentWidget != a || b.GetCode() != "" {
		t.Fatal("shared handler was duplicated instead of located", err)
	}
	before := s.UndoStack().Count()
	for _, bad := range []string{"_", "bad-name", "for"} {
		if s.SetEventBinding(b, "OnClick", bad) == nil {
			t.Fatal("invalid name accepted", bad)
		}
	}
	if s.SetEventBinding(b, "OnMagic", "f") == nil || s.UndoStack().Count() != before {
		t.Fatal("unsupported event mutated document")
	}
	if err := s.SetEventBinding(b, "OnClick", ""); err != nil {
		t.Fatal(err)
	}
	v.Undo()
	if b.EventHandlers()["OnClick"] == "" {
		t.Fatal("removal not undoable")
	}
	b.SetEventHandler("RemovedPluginEvent", "legacyHandler")
	if err := s.SetEventBinding(b, "RemovedPluginEvent", ""); err != nil {
		t.Fatal("cannot remove legacy binding", err)
	}
	v.Undo()
	if b.EventHandlers()["RemovedPluginEvent"] != "legacyHandler" {
		t.Fatal("legacy removal not undoable")
	}
	// Reusing a string callback for a parameterless event must not silently bind.
	a.SetCode("func onAClick(s string) {}")
	if p.OpenEventHandler(b, "OnClick") == nil {
		t.Fatal("incompatible signature accepted")
	}
}

func TestEventTemplatesAndMenusUseGeneratorSignatures(t *testing.T) {
	for factory, events := range widgetEvents {
		v := NewGedView()
		w, err := NewFakeWidgetFromFactory(factory)
		if err != nil {
			t.Fatal(err)
		}
		w.SetWidgetName("control")
		w.SetParent(v.GedScene())
		p := NewCodePanel()
		p.BindGedView(v)
		for _, event := range events {
			if err := p.OpenEventHandler(w, event.name); err != nil {
				t.Fatalf("%s.%s: %v", factory, event.name, err)
			}
			actual, _, _, _ := findHandler(w.GetCode(), w.EventHandlers()[event.name])
			expected, _, _, _ := findHandler("func expected("+event.params+"){}", "expected")
			if fmt.Sprint(parameterTypes(actual.Type)) != fmt.Sprint(parameterTypes(expected.Type)) {
				t.Fatal("signature drift", factory, event.name)
			}
		}
		for _, item := range v.EventHandlerMenu(w).Items() {
			if b, ok := item.(*gui.Button); ok && (b.Icon() == nil || !b.Action().IsEnabled()) {
				t.Fatal("unusable event menu", factory)
			}
		}
		p.BindGedView(nil)
	}
}

func TestMultiEventGeneratedSDKRuntime(t *testing.T) {
	if testing.Short() {
		t.Skip("external SDK consumer")
	}
	v := NewGedView()
	s := v.GedScene()
	s.SetFormTitle("Events")
	w := addFakeTo(t, s, "gui.SearchBox", "search")
	addFakeTo(t, s, "gui.Label", "result")
	p := NewCodePanel()
	p.BindGedView(v)
	defer p.BindGedView(nil)
	for _, event := range []string{"OnSearch", "OnTextChanged"} {
		if err := p.OpenEventHandler(w, event); err != nil {
			t.Fatal(err)
		}
	}
	code := strings.ReplaceAll(w.GetCode(), "// TODO: implement OnSearch.", `ui.Result.SetText("search:"+q)`)
	code = strings.ReplaceAll(code, "// TODO: implement OnTextChanged.", `ui.Result.SetText("changed:"+s)`)
	p.HandlerEditor().ReplaceAllText(code) // export must flush without a panel save
	dir := t.TempDir()
	res, err := s.GenerateCodeFile(filepath.Join(dir, "events.go"), CodeGenOptions{PackageName: "probe", TypeName: "ExportedUI"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(res.UserFile)
	if _, err := s.GenerateCodeFile(res.UserFile, CodeGenOptions{PackageName: "probe", TypeName: "ExportedUI"}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(res.UserFile)
	if string(before) != string(after) {
		t.Fatal("regeneration rewrote owned handler file")
	}
	mod := fmt.Sprintf("module event.consumer\n\ngo 1.26.0\nrequire github.com/uk0/silk v0.0.0\nreplace github.com/uk0/silk => %q\n", findModuleRoot(t))
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0600); err != nil {
		t.Fatal(err)
	}
	source := `package probe
import("testing";"github.com/uk0/silk/gui")
func TestEvents(t *testing.T){
 ui:=NewExportedUI()
 ui.Search.SetText("中文😀")
 if ui.Result.Text()!="changed:中文😀"{t.Fatal("change callback",ui.Result.Text())}
 ui.Search.OnKeyDown(gui.KeyEnter,false)
 if ui.Result.Text()!="search:中文😀"{t.Fatal("search callback",ui.Result.Text())}
}`
	if err := os.WriteFile(filepath.Join(dir, "events_runtime_test.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-mod=mod", "-count=1", "./...")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated SDK runtime: %v\n%s\n%s", err, out, before)
	}
	t.Log("generated multi-event SDK consumer: both Unicode callbacks executed; regeneration preserved user file")
}
