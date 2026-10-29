package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/scene3d"
)

func sceneAssetTestSource(t *testing.T) string {
	t.Helper()
	s := scene3d.NewScene()
	if err := s.Add(scene3d.Node{ID: "tank", Mesh: scene3d.Box()}); err != nil {
		t.Fatal(err)
	}
	d, err := scene3d.CaptureDocument(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err = scene3d.SaveDocument(context.Background(), &b, d); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
func sceneAssetTestHooks(t *testing.T) {
	t.Helper()
	oldLoad, oldShow, busy := startSceneAssetLoad, showSceneAssetEditor, sceneAssetOpening
	sceneAssetOpening = false
	startSceneAssetLoad = func(source string, done func(*scene3d.Document, error)) error {
		d, err := scene3d.LoadDocument(context.Background(), strings.NewReader(source))
		done(d, err)
		return nil
	}
	t.Cleanup(func() { startSceneAssetLoad = oldLoad; showSceneAssetEditor = oldShow; sceneAssetOpening = busy })
}
func TestSceneAssetEditorMenuPaletteUndoSaveAndReload(t *testing.T) {
	w := testWorkbench(t)
	sceneAssetTestHooks(t)
	source := sceneAssetTestSource(t)
	path := writeFile(t, t.TempDir(), "plant.silkscene", source)
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	ed.SetFocus()
	if ed.Language() != "json" {
		t.Fatal("not JSON")
	}
	called := 0
	showSceneAssetEditor = func(parent gui.IWidget, d *scene3d.Document, commit func(*scene3d.Document) error) error {
		called++
		if parent != ed || d.Nodes[0].ID != "tank" {
			t.Fatal("wrong source")
		}
		d.Nodes[0].Name = "设备 1"
		d.Nodes[0].Transform[13] = 3
		d.Bindings = []scene3d.Binding{{Node: "tank", Tag: "device.height", Property: scene3d.PositionY, Gain: .001}}
		return commit(d)
	}
	m := buildWorkbenchSubMenu("Tools", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m)
	button := menuCommandButton(m, "tools.sceneEditor")
	if button == nil || button.Icon() == nil || !button.IsEnabled() {
		t.Fatal("missing menu")
	}
	button.Action().Trigger(nil)
	if called != 1 || !ed.Modified() || !ed.CanUndo() {
		t.Fatal("not one buffered edit", called)
	}
	disk, _ := os.ReadFile(path)
	if string(disk) != source {
		t.Fatal("hidden file write")
	}
	edited := ed.Text()
	ed.Undo()
	if ed.Text() != source || ed.CanUndo() {
		t.Fatal("undo boundary")
	}
	ed.Redo()
	if ed.Text() != edited {
		t.Fatal("redo")
	}
	if !saveEditorToDisk(ed, path) {
		t.Fatal("save")
	}
	disk, _ = os.ReadFile(path)
	d, err := scene3d.LoadDocument(context.Background(), bytes.NewReader(disk))
	if err != nil || d.Nodes[0].Name != "设备 1" || len(d.Bindings) != 1 || d.Nodes[0].Transform[13] != 3 {
		t.Fatal(err, d)
	}
	ed.SetFocus()
	registerPaletteCommands(w.tabs, w.canvas)
	found := false
	for _, p := range paletteCommands {
		if p.Name == i18n.T("Edit 3D Scene...") {
			found = true
			if !p.Available() || p.Bind == nil {
				t.Fatal("palette disabled")
			}
			p.Bind().Fn()
		}
	}
	if !found || called != 2 {
		t.Fatal("palette not wired")
	}
}
func TestSceneAssetEditorLateLoadsStaleCommitsAndReadOnly(t *testing.T) {
	w := testWorkbench(t)
	sceneAssetTestHooks(t)
	source := sceneAssetTestSource(t)
	path := writeFile(t, t.TempDir(), "a.SILKSCENE", source)
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	ed.SetFocus()
	var complete func(*scene3d.Document, error)
	startSceneAssetLoad = func(_ string, done func(*scene3d.Document, error)) error { complete = done; return nil }
	opened := 0
	showSceneAssetEditor = func(gui.IWidget, *scene3d.Document, func(*scene3d.Document) error) error { opened++; return nil }
	editSceneAsset(w.tabs, w.canvas, ed)
	if !sceneAssetOpening {
		t.Fatal("not reserved")
	}
	d, _ := scene3d.LoadDocument(context.Background(), strings.NewReader(source))
	ed.ReplaceAllText(source + " ")
	complete(d, nil)
	if opened != 0 || sceneAssetOpening {
		t.Fatal("stale decode opened")
	}
	ed.SetReadOnly(true)
	editSceneAsset(w.tabs, w.canvas, ed)
	if sceneAssetOpening {
		t.Fatal("readonly")
	}
	ed.SetReadOnly(false)
	editSceneAsset(w.tabs, w.canvas, ed)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "b.txt", "untouched"))
	complete(d, nil)
	if opened != 0 || activeEditor(w.tabs).Text() != "untouched" {
		t.Fatal("wrong tab completion")
	}
	focusEditorTab(w.tabs, ed)
	startSceneAssetLoad = func(source string, done func(*scene3d.Document, error)) error {
		d, err := scene3d.LoadDocument(context.Background(), strings.NewReader(source))
		done(d, err)
		return nil
	}
	showSceneAssetEditor = func(_ gui.IWidget, d *scene3d.Document, commit func(*scene3d.Document) error) error {
		ed.ReplaceAllText(source + "\n\n")
		before := ed.Text()
		d.Nodes[0].Name = "must not apply"
		if err := commit(d); err == nil {
			t.Fatal("stale commit accepted")
		}
		if ed.Text() != before {
			t.Fatal("stale overwrote buffer")
		}
		return nil
	}
	editSceneAsset(w.tabs, w.canvas, ed)
	startSceneAssetLoad = func(string, func(*scene3d.Document, error)) error { return errors.New("queue closed") }
	editSceneAsset(w.tabs, w.canvas, ed)
	if sceneAssetOpening {
		t.Fatal("failed submit stuck busy")
	}
}
func TestSceneAssetEditorCancellationInvalidDocumentAndLineEndings(t *testing.T) {
	w := testWorkbench(t)
	sceneAssetTestHooks(t)
	source := sceneAssetTestSource(t)
	path := writeFile(t, t.TempDir(), "a.silkscene", strings.TrimSuffix(source, "\n")+"\r\n")
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	ed.SetFocus()
	showSceneAssetEditor = func(gui.IWidget, *scene3d.Document, func(*scene3d.Document) error) error { return nil }
	editSceneAsset(w.tabs, w.canvas, ed)
	if ed.Modified() || ed.CanUndo() {
		t.Fatal("cancel dirtied buffer")
	}
	showSceneAssetEditor = func(_ gui.IWidget, d *scene3d.Document, commit func(*scene3d.Document) error) error {
		d.Nodes[0].Name = "new"
		return commit(d)
	}
	editSceneAsset(w.tabs, w.canvas, ed)
	if !strings.Contains(ed.Text(), "\r\n") || strings.Contains(strings.ReplaceAll(ed.Text(), "\r\n", ""), "\n") {
		t.Fatal("line ending changed")
	}
	ed.ReplaceAllText(`{"version":99}`)
	before := ed.Text()
	showSceneAssetEditor = func(gui.IWidget, *scene3d.Document, func(*scene3d.Document) error) error {
		t.Fatal("opened invalid document")
		return nil
	}
	editSceneAsset(w.tabs, w.canvas, ed)
	if ed.Text() != before || sceneAssetOpening {
		t.Fatal("invalid changed buffer")
	}
}

func TestSceneAssetAuthoringOneHostTransactionAndDiskRoundtrip(t *testing.T) {
	w := testWorkbench(t)
	sceneAssetTestHooks(t)
	source := sceneAssetTestSource(t)
	path := writeFile(t, t.TempDir(), "authoring.silkscene", source)
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	ed.SetFocus()
	var staged *scene3d.Document
	showSceneAssetEditor = func(_ gui.IWidget, d *scene3d.Document, commit func(*scene3d.Document) error) error {
		e, err := gui.NewSceneDocumentEditor(d)
		if err != nil {
			return err
		}
		defer e.Close()
		if err = e.AddNode(scene3d.DocumentNode{ID: "assembly", Name: "装配组"}); err != nil {
			return err
		}
		if err = e.ReparentNode("tank", "assembly", true); err != nil {
			return err
		}
		if err = e.SetBinding(scene3d.Binding{Node: "tank", Tag: "tank.level", Property: scene3d.PositionY, Gain: 1}); err != nil {
			return err
		}
		if _, err = e.DuplicateSubtree("assembly", scene3d.DuplicateOptions{RootID: "second", CopyBindings: true, Tags: map[string]string{"tank.level": "second.level"}}); err != nil {
			return err
		}
		if err = e.RemoveSubtree("assembly"); err != nil {
			return err
		}
		if err = e.Undo(); err != nil {
			return err
		}
		staged, err = e.Document()
		if err != nil {
			return err
		}
		return commit(staged)
	}
	editSceneAsset(w.tabs, w.canvas, ed)
	if staged == nil || len(staged.Nodes) != 4 || len(staged.Bindings) != 2 || len(staged.Meshes) != 1 || !ed.Modified() {
		t.Fatal("authoring not committed")
	}
	disk, err := os.ReadFile(path)
	if err != nil || string(disk) != source {
		t.Fatal("staged changes bypassed Save", err)
	}
	edited := ed.Text()
	ed.Undo()
	if ed.Text() != source || ed.CanUndo() {
		t.Fatal("multiple host undo transactions")
	}
	ed.Redo()
	if ed.Text() != edited {
		t.Fatal("host redo")
	}
	if !saveEditorToDisk(ed, path) {
		t.Fatal("save")
	}
	disk, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := scene3d.LoadDocument(context.Background(), bytes.NewReader(disk))
	if err != nil {
		t.Fatal(err)
	}
	var a, b bytes.Buffer
	if err = scene3d.SaveDocument(context.Background(), &a, staged); err != nil {
		t.Fatal(err)
	}
	if err = scene3d.SaveDocument(context.Background(), &b, reopened); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("save/reopen lost authored state")
	}
}
