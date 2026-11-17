package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

func TestGroupBoxAuthoringPropertiesRoundTripAndFrozenCode(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)
	fake, err := NewFakeWidgetFromFactory("gui.GroupBox")
	if err != nil {
		t.Fatal(err)
	}
	fake.SetWidgetName("options")
	fake.SetBounds(5, 5, 80, 50)
	rec := newPropRecorder()
	fake.EnumProperties(rec)
	for _, id := range []string{"Checkable", "Checked"} {
		if _, ok := rec.sets[id].(func(bool)); !ok {
			t.Fatalf("designer has no editable %s row", id)
		}
	}
	rec.sets["Checkable"].(func(bool))(true)
	rec.sets["Checked"].(func(bool))(false)
	command := graph.NewAddCommand()
	command.AddItem(fake, scene)
	scene.PushCommand(command)
	reopened := NewGedScene()
	if err := reopened.LoadDesign(scene.SaveDesign()); err != nil {
		t.Fatal(err)
	}
	loaded := reopened.Children()[0].(*FakeWidget).Widget().(*gui.GroupBox)
	if !loaded.IsCheckable() || loaded.IsChecked() {
		t.Fatal("save/reopen lost the GroupBox content gate")
	}
	snapshot := reopened.CaptureCodeSnapshot()
	loaded.SetCheckable(false)
	loaded.SetChecked(true)
	code := snapshot.Generate(CodeGenOptions{PackageName: "main", TypeName: "OptionsUI"}).Code
	for _, setter := range []string{"ui.Options.SetCheckable(true)", "ui.Options.SetChecked(false)"} {
		if !strings.Contains(code, setter) {
			t.Fatalf("frozen code lost authored gate: %s\n%s", setter, code)
		}
	}
	vetGeneratedCode(t, code)
}

func TestGroupBoxAuthoringToggledBindingAndInitializationOrder(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)
	fake, err := NewFakeWidgetFromFactory("gui.GroupBox")
	if err != nil {
		t.Fatal(err)
	}
	fake.SetWidgetName("options")
	fake.SetBounds(5, 5, 80, 50)
	group := fake.Widget().(*gui.GroupBox)
	group.SetCheckable(true)
	group.SetChecked(false)
	rec := newPropRecorder()
	fake.EnumProperties(rec)
	bind, ok := rec.sets["OnToggled"].(func(string))
	if !ok {
		t.Fatal("designer does not offer a bindable GroupBox OnToggled event")
	}
	bind("onOptionsToggled")
	fake.SetCode("func onOptionsToggled(checked bool) {}")
	command := graph.NewAddCommand()
	command.AddItem(fake, scene)
	scene.PushCommand(command)
	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "OptionsUI"})
	binding := strings.Index(code, "ui.Options.SigToggled(")
	state := strings.Index(code, "ui.Options.SetChecked(false)")
	if binding < 0 || state < 0 || binding < state {
		t.Fatalf("GroupBox handler binds before its initial checked state, or is missing:\n%s", code)
	}
	vetGeneratedCode(t, code)
}

func TestGeneratedHandlersConnectAfterAllInitialState(t *testing.T) {
	scene := NewGedScene()
	scene.SetSize(200, 150)
	addConfiguredFake(t, scene, "gui.CheckBox", "check", func(w interface{}) { w.(*gui.CheckBox).SetChecked(true) })
	addConfiguredFake(t, scene, "gui.SpinBox", "spin", func(w interface{}) { w.(*gui.SpinBox).SetValue(42) })
	for _, item := range scene.Children() {
		fake := item.(*FakeWidget)
		switch fake.WidgetFactoryName() {
		case "gui.CheckBox":
			fake.SetEventHandler("OnToggled", "onChecked")
			fake.SetCode("func onChecked(checked bool) {}")
		case "gui.SpinBox":
			fake.SetEventHandler("OnValueChanged", "onValue")
			fake.SetCode("func onValue(value int) {}")
		}
	}
	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "InitialStateUI"})
	for _, bind := range []string{"ui.Check.SigCheck(", "ui.Spin.SetValueChangedCallback("} {
		binding := strings.Index(code, bind)
		for _, setter := range []string{"ui.Check.SetChecked(true)", "ui.Spin.SetValue(42)"} {
			state := strings.Index(code, setter)
			if binding < 0 || state < 0 || binding < state {
				t.Fatalf("%s runs while initialization is incomplete at %s\n%s", bind, setter, code)
			}
		}
	}
	vetGeneratedCode(t, code)
}

func TestGroupBoxOldDesignKeepsConstructorDefaults(t *testing.T) {
	fake, err := NewFakeWidgetFromFactory("gui.GroupBox")
	if err != nil {
		t.Fatal(err)
	}
	group := fake.Widget().(*gui.GroupBox)
	if group.IsCheckable() || !group.IsChecked() {
		t.Fatal("factory defaults changed for an older plain group")
	}
	properties := captureWidgetProperties(group)
	delete(properties, "Checkable")
	delete(properties, "Checked")
	// An older file has no new check-state fields. Applying those old fields
	// must retain the constructor's plain-frame/checked defaults.
	applyWidgetProperties(group, properties)
	if group.IsCheckable() || !group.IsChecked() {
		t.Fatal("old property set changed new GroupBox defaults")
	}
}
