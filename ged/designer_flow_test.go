package ged

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

// The two tests in this file drive the designer the way a person does, through
// the same entry points the UI calls, and assert on what the person would see.
// Everything else in ged/ tests one stage; the defects that reach users come
// from the seams between stages — a widget that keeps its parent but stops
// being drawn, a name that survives a paste but collides in generated code, a
// regeneration that eats the file the developer was editing. None of those is
// visible from inside a single stage.
//
// Costs: each test calls vetGeneratedFiles exactly once, and that call shells
// out to `go mod tidy` plus `go vet`. That is why the per-step check below is
// the cheap one — parse the output and ask the generator which widgets it
// emitted — rather than a type check after every operation.

// ---------------------------------------------------------------------------
// driving the designer
// ---------------------------------------------------------------------------

// allFakeWidgets flattens the design, parents before children. It reuses the
// codegen walk so "every widget in the design" means the same set here as it
// does to the generator.
func allFakeWidgets(scene *GedScene) []*FakeWidget {
	var out []*FakeWidget
	walkFakeWidgets(scene.Children(), func(f *FakeWidget) { out = append(out, f) })
	return out
}

// widgetLabel names a widget the way an error message should: its designed
// name, or its type when it has none.
func widgetLabel(fake *FakeWidget) string {
	if n := fake.WidgetName(); n != "" {
		return n
	}
	return "未命名 " + fake.WidgetFactoryName()
}

// placeFromPalette drops factoryName onto the canvas at the scene point (x, y)
// through GedView.OnDrop — the palette's own path, so the factory registry, the
// default size table, the grid snap, the drop-into-container rule and the undo
// command all run. Constructing a FakeWidget and calling SetParent, which is
// what most tests here do, skips every one of those.
func placeFromPalette(t *testing.T, view *GedView, factoryName string, x, y float64) *FakeWidget {
	t.Helper()

	scene := view.GedScene()
	before := map[*FakeWidget]bool{}
	for _, w := range allFakeWidgets(scene) {
		before[w] = true
	}

	ctx := &dropCtx{pa: gui.DndCopy, payload: factoryName}
	ctx.SetAction(gui.DndIgnore)
	px, py := view.MapFromScene(x, y)
	view.OnDrop(px, py, ctx)
	if ctx.Action() != gui.DndCopy {
		t.Fatalf("dropping %s at (%g,%g) was refused: action = %v", factoryName, x, y, ctx.Action())
	}

	var added *FakeWidget
	for _, w := range allFakeWidgets(scene) {
		if before[w] {
			continue
		}
		if added != nil {
			t.Fatalf("one drop of %s added more than one widget", factoryName)
		}
		added = w
	}
	if added == nil {
		t.Fatalf("dropping %s added nothing to the design", factoryName)
	}
	return added
}

// editThroughSheet performs one property-sheet edit: it re-enumerates the
// widget's rows the way binding the sheet to a new selection does, then calls
// the setter that row published.
//
// Going through the row rather than the widget's own setter is the point. That
// layer is where the mark-dirty adapter wraps every embedded setter and where
// the 事件 rows turn into SetEventHandlerChecked, and it is the only layer a
// real edit ever passes through. A test that calls Widget().(*gui.Button).SetText
// proves nothing about whether the sheet can reach it.
func editThroughSheet(t *testing.T, fake *FakeWidget, id string, v interface{}) {
	t.Helper()

	rec := newPropRecorder()
	fake.EnumProperties(rec)
	set, ok := rec.sets[id]
	if !ok || set == nil {
		t.Fatalf("%s has no editable property-sheet row %q; rows = %v",
			fake.WidgetFactoryName(), id, rec.ids)
	}
	sv := reflect.ValueOf(set)
	av := reflect.ValueOf(v)
	if sv.Kind() != reflect.Func || sv.Type().NumIn() != 1 || !av.Type().AssignableTo(sv.Type().In(0)) {
		t.Fatalf("row %q takes %v, the test passed %T", id, sv.Type(), v)
	}
	sv.Call([]reflect.Value{av})
}

// readThroughSheet reads one property back through the same rows, so a
// round-trip assertion is made at the layer the user reads at.
func readThroughSheet(t *testing.T, fake *FakeWidget, id string) interface{} {
	t.Helper()

	rec := newPropRecorder()
	fake.EnumProperties(rec)
	get, ok := rec.gets[id]
	if !ok || get == nil {
		t.Fatalf("%s has no property-sheet row %q; rows = %v",
			fake.WidgetFactoryName(), id, rec.ids)
	}
	gv := reflect.ValueOf(get)
	if gv.Kind() != reflect.Func || gv.Type().NumIn() != 0 || gv.Type().NumOut() != 1 {
		t.Fatalf("row %q is not readable: getter is %v", id, gv.Type())
	}
	return gv.Call(nil)[0].Interface()
}

// ---------------------------------------------------------------------------
// invariants
// ---------------------------------------------------------------------------

// checkDesignInvariants asserts, after one editing step, the three properties
// every later stage silently assumes. They are checked after every structural
// operation because each of them has been broken by one before, and none of
// them is visible from an assertion on Parent() or on the undo count.
func checkDesignInvariants(t *testing.T, scene *GedScene, step string) {
	t.Helper()
	checkWidgetsAreDrawn(t, scene, step)
	checkNamesAreUnique(t, scene, step)
	checkCodegenSeesEveryWidget(t, scene, step)
}

// checkWidgetsAreDrawn asserts every widget still shows on the canvas.
//
// A FakeWidget has no local coordinate space — only SceneItem sets localCoord,
// so every widget in a design shares one space and a child's bounds are scene
// bounds. Item.DrawAll clips a child to the intersection with its parent and
// returns early when that intersection is empty, so a widget whose rect misses
// its parent's is still in the document, still answers Parent() correctly, and
// cannot be seen or clicked. A tree re-parent shipped exactly that; the test
// that was supposed to catch it asserted Parent() and an undo count.
//
// The assertion is a non-empty intersection rather than full containment on
// purpose: this is an absolute-position designer, a widget is allowed to hang
// over its container's edge, and parkInsideParent only ever restores overlap.
// Demanding containment would fail designs that are fine and say nothing extra
// about the ones that are broken — empty intersection is what "hidden" means.
func checkWidgetsAreDrawn(t *testing.T, scene *GedScene, step string) {
	t.Helper()

	var walk func(items []graph.IItem, parent string, px, py, pw, ph float64)
	walk = func(items []graph.IItem, parent string, px, py, pw, ph float64) {
		for _, it := range items {
			fake, ok := it.(*FakeWidget)
			if !ok {
				continue
			}
			x, y, w, h := fake.X(), fake.Y(), fake.Width(), fake.Height()
			if x >= px+pw || x+w <= px || y >= py+ph || y+h <= py {
				t.Errorf("%s: %s at (%g,%g %g×%g) does not overlap %s at (%g,%g %g×%g) — it is in the document and invisible on the canvas",
					step, widgetLabel(fake), x, y, w, h, parent, px, py, pw, ph)
			}
			walk(fake.Children(), widgetLabel(fake), x, y, w, h)
		}
	}
	x, y, w, h := scene.Bounds()
	walk(scene.Children(), "the form", x, y, w, h)
}

// checkNamesAreUnique asserts no two widgets anywhere in the document carry the
// same name. Codegen declares one struct field per widget and will suffix a
// collision away rather than emit a file that does not build, so a duplicate
// does not fail the build — it silently retargets half the calls the designer
// wrote against the other widget. Unnamed widgets are skipped: they carry no
// name to collide, and codegen synthesizes their field names separately.
func checkNamesAreUnique(t *testing.T, scene *GedScene, step string) {
	t.Helper()

	owners := map[string][]string{}
	for _, fake := range allFakeWidgets(scene) {
		if n := fake.WidgetName(); n != "" {
			owners[n] = append(owners[n], fake.WidgetFactoryName())
		}
	}
	for name, factories := range owners {
		if len(factories) > 1 {
			t.Errorf("%s: %d widgets share the name %q (%v)", step, len(factories), name, factories)
		}
	}
}

// checkCodegenSeesEveryWidget is the per-step half of the codegen check: the
// design must still render to source that parses, with one constructor
// statement for each widget in it, on a line of its own.
//
// It is deliberately not a type check. vetGeneratedFiles shells out to `go mod
// tidy` and `go vet` and costs seconds per call, so running it after every
// operation would make this test one nobody runs; each test below runs it once,
// at the end, over the state all the steps produced. What this catches in the
// meantime is a step that drops a widget from the emitted tree or produces
// source the parser rejects — and it catches it by asking the generator which
// widgets it emitted, not by matching strings in its output.
func checkCodegenSeesEveryWidget(t *testing.T, scene *GedScene, step string) {
	t.Helper()

	gen := scene.GenerateCodeIndexed(CodeGenOptions{PackageName: "main", TypeName: "StepUI"})
	if gen.Err != nil {
		t.Errorf("%s: the design no longer generates: %v", step, gen.Err)
		return
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "step.go", gen.Code, 0); err != nil {
		t.Errorf("%s: generated source does not parse: %v\n--- code ---\n%s", step, err, gen.Code)
		return
	}

	widgets := allFakeWidgets(scene)
	at := map[int]*FakeWidget{}
	for _, fake := range widgets {
		line, ok := gen.Line(fake)
		if !ok {
			t.Errorf("%s: %s is in the design but the generator emitted no constructor for it", step, widgetLabel(fake))
			continue
		}
		if other, clash := at[line]; clash {
			t.Errorf("%s: %s and %s were both mapped to generated line %d — they collapsed into one field",
				step, widgetLabel(other), widgetLabel(fake), line)
		}
		at[line] = fake
	}
}

// ---------------------------------------------------------------------------
// comparing two saves of the same design
// ---------------------------------------------------------------------------

// canonicalDesign renders doc in a form two independent saves of one design
// always agree on.
//
// FakeWidget.SaveDesign writes its "props" and "events" blocks by ranging over
// a Go map, so the order those children land in is randomized per run and a raw
// String() comparison would be flaky rather than wrong. Siblings that all carry
// distinct non-empty keys are exactly the map-shaped ones and get sorted;
// siblings without keys are the widget lists, whose order is the design's
// z-order and sibling index, and they keep it — so a design that comes back
// with its widgets in a different order still compares unequal.
//
// It reorders doc in place; callers hand it a document SaveDesign just built.
func canonicalDesign(doc *core.TDoc) string {
	sortKeyedSiblings(doc)
	return doc.String()
}

func sortKeyedSiblings(doc *core.TDoc) {
	kids := doc.Childdren()
	keyed, seen := len(kids) > 1, map[string]bool{}
	for _, k := range kids {
		if k.Key() == "" || seen[k.Key()] {
			keyed = false
			break
		}
		seen[k.Key()] = true
	}
	if keyed {
		doc.Sort(nil)
	}
	for _, k := range kids {
		sortKeyedSiblings(k)
	}
}

// ---------------------------------------------------------------------------
// the flow the designer exists for
// ---------------------------------------------------------------------------

// TestDesignerFlowPaletteToVettedCode performs, headlessly and through the real
// APIs, the whole job the designer is for: place widgets from the palette,
// configure them in the property sheet, bind an event, save the design to a
// .silkui file, open that file into a fresh scene, and export the code — then
// export again and prove the developer's file survived it.
//
// Every stage of this has its own test. What none of them covers is the
// composition, which is where the failures land: a property the sheet can edit
// but the file cannot carry, a binding that survives the file and is dropped by
// the generator, a second export that rewrites the half the developer owns.
func TestDesignerFlowPaletteToVettedCode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: this test type-checks generated code with go vet")
	}

	view := NewGedView()
	scene := view.GedScene()
	scene.SetFormTitle("Flow")
	scene.SetSize(200, 150)
	view.SetGridStep(5)

	// 1. Place from the factory registry, through the palette's drop path. The
	//    button is dropped over the box, so the drop-into-container rule nests
	//    it without anyone naming a parent.
	panel := placeFromPalette(t, view, "gui.VBox", 20, 20)
	button := placeFromPalette(t, view, "gui.Button", 25, 25)
	field := placeFromPalette(t, view, "gui.Edit", 20, 100)
	check := placeFromPalette(t, view, "gui.CheckBox", 20, 115)

	if button.Parent() != graph.IItem(panel) {
		t.Fatalf("the button dropped over the box is parented to %v, want the box", button.Parent())
	}
	for _, top := range []*FakeWidget{panel, field, check} {
		if top.Parent() != graph.IItem(scene) {
			t.Fatalf("%s is parented to %v, want the form root", top.WidgetFactoryName(), top.Parent())
		}
	}

	// 2. Configure through the property sheet, including one value per scalar
	//    kind the design file has to carry: a name, an edited caption with
	//    non-ASCII text, and a bool.
	editThroughSheet(t, panel, "控件名称", "panel")
	editThroughSheet(t, button, "控件名称", "btnOK")
	editThroughSheet(t, button, "文本", "确定")
	editThroughSheet(t, field, "控件名称", "editPath")
	editThroughSheet(t, field, "文本", "日志目录")
	editThroughSheet(t, check, "控件名称", "chkAuto")
	editThroughSheet(t, check, "选中", true)

	// 3. Bind an event — also a property-sheet row, which is why codegen and
	//    the sheet cannot disagree about what is bindable.
	editThroughSheet(t, button, "OnClick", "onOK")

	checkDesignInvariants(t, scene, "after placing and configuring the design")

	// 4. Save to a real .silkui file, the way 保存 does.
	dir := t.TempDir()
	designPath := filepath.Join(dir, "flow"+DefaultDesignExt)
	scene.SetFilename(designPath)
	if !scene.Save() {
		t.Fatalf("Save() reported failure writing %s", designPath)
	}

	// 5. Open it into a scene that has never seen this design.
	loaded := NewGedScene()
	if err := loaded.OpenFile(designPath); err != nil {
		t.Fatalf("opening the saved design: %v", err)
	}
	if missing := loaded.MissingWidgets(); len(missing) != 0 {
		t.Fatalf("the load dropped widgets whose factories it could not find: %v", missing)
	}

	// 6. The loaded design is the saved design. Compared as documents rather
	//    than field by field, so a value nobody thought to assert on — a lock
	//    flag, the grid, a widget property this test never mentions — is
	//    covered too.
	if got, want := canonicalDesign(loaded.SaveDesign()), canonicalDesign(scene.SaveDesign()); got != want {
		t.Errorf("the design changed across save/load\n--- saved ---\n%s\n--- reloaded ---\n%s", want, got)
	}

	// The comparison above is only as strong as what SaveDesign writes: a
	// writer that drops a field drops it on both sides and the two documents
	// still match. So pin the floor separately — the same widgets came back, in
	// the same tree, carrying the values that were typed in — and read those
	// values back through the property sheet, where the user reads them.
	reloaded := allFakeWidgets(loaded)
	if len(reloaded) != 4 {
		t.Fatalf("the reloaded design holds %d widgets, want 4", len(reloaded))
	}
	byName := map[string]*FakeWidget{}
	for _, fake := range reloaded {
		byName[fake.WidgetName()] = fake
	}
	for _, want := range []string{"panel", "btnOK", "editPath", "chkAuto"} {
		if byName[want] == nil {
			t.Fatalf("the reloaded design has no widget named %q", want)
		}
	}
	for id, want := range map[string]interface{}{"文本": "确定", "OnClick": "onOK"} {
		if got := readThroughSheet(t, byName["btnOK"], id); got != want {
			t.Errorf("reloaded btnOK row %q = %v, want %v", id, got, want)
		}
	}
	if got := readThroughSheet(t, byName["chkAuto"], "选中"); got != true {
		t.Errorf("reloaded chkAuto row 选中 = %v, want true", got)
	}
	if byName["btnOK"].Parent() != graph.IItem(byName["panel"]) {
		t.Errorf("the reloaded button is parented to %v, want the reloaded panel", byName["btnOK"].Parent())
	}

	// 7. Export the split pair from the reloaded scene and type-check it. The
	//    machine file calls ui.onOK and only the user file declares it, so the
	//    pair is the unit — vetting either half alone proves nothing.
	opts := CodeGenOptions{PackageName: "main", TypeName: "FlowUI"}
	res, err := loaded.GenerateCodeFile(filepath.Join(dir, "flow.go"), opts)
	if err != nil {
		t.Fatalf("exporting the design: %v", err)
	}
	if !res.UserCreated {
		t.Error("the first export did not create the user file")
	}
	vetGeneratedFiles(t, readPair(t, res))

	// 8. The developer works in their half, then the design is exported again.
	//    A regeneration may append a stub for an event the design has gained
	//    and may make no other write to that file, so with the design unchanged
	//    it must come back byte for byte — the developer's line included.
	//
	//    The edit is what gives this step teeth. Re-exporting an untouched user
	//    file returns the same bytes even if the generator rewrote it from
	//    scratch, because the input did not change; only a file that has been
	//    edited can tell "left alone" from "regenerated identically".
	generated, err := os.ReadFile(res.UserFile)
	if err != nil {
		t.Fatalf("reading the user file: %v", err)
	}
	before := string(generated) + "\nfunc (ui *FlowUI) mine() string { return \"keep me\" }\n"
	if err := os.WriteFile(res.UserFile, []byte(before), 0644); err != nil {
		t.Fatalf("writing the developer's edit: %v", err)
	}

	res2, err := loaded.GenerateCodeFile(filepath.Join(dir, "flow.go"), opts)
	if err != nil {
		t.Fatalf("re-exporting the design: %v", err)
	}
	if res2.UserCreated {
		t.Error("the second export rewrote the user file from scratch")
	}
	if len(res2.AddedStubs) != 0 {
		t.Errorf("the second export appended %v to the user file; the design gained no event", res2.AddedStubs)
	}
	after, err := os.ReadFile(res.UserFile)
	if err != nil {
		t.Fatalf("reading the user file after re-export: %v", err)
	}
	if string(after) != before {
		t.Errorf("re-exporting rewrote the user file\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
}

// ---------------------------------------------------------------------------
// the structural edits, where the defects have actually been
// ---------------------------------------------------------------------------

// TestDesignerStructuralEditsKeepTheDesignSound runs the structural half of the
// designer — nest, un-nest, copy, paste, undo, redo — and after every single
// operation asserts the design is still one a user can see, name and export.
//
// The per-step invariants are the point. Each of these operations has a test
// that asserts the thing it was written for (a parent pointer, a sibling index,
// an undo count) and each of those tests stayed green through a defect that
// broke one of the three properties checked here.
func TestDesignerStructuralEditsKeepTheDesignSound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: this test type-checks generated code with go vet")
	}

	view := NewGedView()
	scene := view.GedScene()
	scene.SetFormTitle("Structure")
	scene.SetSize(200, 150)
	view.SetGridStep(5)

	// The box is 20..60 × 20..45; the nested button is dropped into its lower
	// half so that the widget the tree drop parks in its top-left corner later
	// lands on empty space and stays pressable.
	panel := placeFromPalette(t, view, "gui.VBox", 20, 20)
	editThroughSheet(t, panel, "控件名称", "panel")
	inner := placeFromPalette(t, view, "gui.Button", 25, 35)
	editThroughSheet(t, inner, "控件名称", "btnInner")
	loose := placeFromPalette(t, view, "gui.Button", 20, 100)
	editThroughSheet(t, loose, "控件名称", "btnLoose")

	if inner.Parent() != graph.IItem(panel) {
		t.Fatalf("the button dropped over the box is parented to %v, want the box", inner.Parent())
	}
	checkDesignInvariants(t, scene, "after placing three widgets")

	// 1. Nest through the object tree. This gesture, not the canvas drag, is
	//    the one that shipped a widget that kept its parent and stopped being
	//    drawn: a tree drop has no pointer position, so the widget arrives
	//    carrying the coordinates it had somewhere else on the form.
	insp := boundInspector(view)
	dragTreeRow(t, insp, loose, panel)
	if loose.Parent() != graph.IItem(panel) {
		t.Fatalf("after the tree drop btnLoose is parented to %v, want the panel", loose.Parent())
	}
	checkDesignInvariants(t, scene, "after nesting through the object tree")

	// 2. Take it back out, by dragging it onto empty canvas.
	view.Selection().Clear()
	view.Selection().Add(loose)
	dragWidgetOnCanvas(view, loose, 150, 120)
	if loose.Parent() != graph.IItem(scene) {
		t.Fatalf("after dragging onto the background btnLoose is parented to %v, want the form root", loose.Parent())
	}
	checkDesignInvariants(t, scene, "after dragging it back out to the form")

	// 3. Copy the container. Copying must not touch the design.
	view.Selection().Clear()
	view.Selection().Add(panel)
	view.CopySelected()
	checkDesignInvariants(t, scene, "after copying the container")
	if n := len(allFakeWidgets(scene)); n != 3 {
		t.Fatalf("copying changed the design: %d widgets, want 3", n)
	}

	// 4. Paste. The copy brings the container's subtree, offset by one grid
	//    pitch and renamed so nothing in the document collides.
	view.PasteItems()
	checkDesignInvariants(t, scene, "after pasting the copy")
	if n := len(allFakeWidgets(scene)); n != 5 {
		t.Fatalf("after pasting the container the design holds %d widgets, want 5", n)
	}

	// 5. Undo it, 6. redo it. Both run commands off the stack without pushing
	//    anything, which is the path that skips whatever a push would have
	//    repaired.
	view.Undo()
	checkDesignInvariants(t, scene, "after undoing the paste")
	if n := len(allFakeWidgets(scene)); n != 3 {
		t.Fatalf("after undoing the paste the design holds %d widgets, want 3", n)
	}

	view.Redo()
	checkDesignInvariants(t, scene, "after redoing the paste")
	if n := len(allFakeWidgets(scene)); n != 5 {
		t.Fatalf("after redoing the paste the design holds %d widgets, want 5", n)
	}

	// The type check runs once, over the design all six steps produced —
	// see checkCodegenSeesEveryWidget for why it is not run per step.
	res, err := scene.GenerateCodeFile(filepath.Join(t.TempDir(), "structure.go"),
		CodeGenOptions{PackageName: "main", TypeName: "StructureUI"})
	if err != nil {
		t.Fatalf("exporting the edited design: %v", err)
	}
	vetGeneratedFiles(t, readPair(t, res))
}

// dragWidgetOnCanvas drags one widget to the scene point (toX, toY) by pressing
// its middle. The middle rather than a corner: a selected widget's corners carry
// its resize handles, and DecorPart pre-empts MovePart, so a press one
// millimetre inside the top-left corner is a resize — which never re-parents
// anything and leaves the gesture looking like it silently did nothing.
func dragWidgetOnCanvas(view *GedView, fake *FakeWidget, toX, toY float64) {
	x, y := fake.Pos()
	dragOnCanvas(view, x+fake.Width()*0.5, y+fake.Height()*0.5, toX, toY)
}

// dragTreeRow replays a row drag in the object tree: press the item's row, move
// onto the middle of the target's row (the band that nests rather than
// reorders), release.
func dragTreeRow(t *testing.T, insp *ObjectInspector, item, target *FakeWidget) {
	t.Helper()

	from, to := insp.rowOfItem(item), insp.rowOfItem(target)
	if from < 0 || to < 0 {
		t.Fatalf("object tree has no row for %s (%d) or %s (%d)",
			widgetLabel(item), from, widgetLabel(target), to)
	}
	insp.OnLeftDown(10, rowMidY(insp, from))
	insp.OnMouseMove(10, rowMidY(insp, to))
	if insp.dropInto != to {
		t.Fatalf("hovering the middle of row %d set dropInto=%d; the drop would reorder, not nest", to, insp.dropInto)
	}
	insp.OnLeftUp(10, rowMidY(insp, to))
}
