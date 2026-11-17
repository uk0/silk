package decl_test

import (
	"testing"

	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/gui"
)

func TestBuildGroupBoxOwnsAndGatesNestedContent(t *testing.T) {
	// Match the IDE's generated companion, including persisted canonical IDs.
	node := decl.GroupBox(decl.ID("group"),
		decl.P("width", 300), decl.P("height", 180),
		decl.P("Checkable", decl.Persisted{Source: "true"}),
		decl.P("Checked", decl.Persisted{Source: "false"}),
		decl.Child(decl.VBox(decl.ID("body"),
			decl.P("layout_enabled", true),
			decl.Children(
				decl.CheckBox(decl.ID("enabled"), decl.P("checked", true)),
				decl.Button(decl.ID("disabled"), decl.P("可用", decl.Persisted{Source: "false"})),
			))),
	)
	_, index, err := node.BuildWithIndex()
	if err != nil {
		t.Fatal(err)
	}
	group := index["group"].(*gui.GroupBox)
	body := index["body"].(*gui.VBox)
	enabled := index["enabled"].(*gui.CheckBox)
	disabled := index["disabled"].(*gui.Button)
	if group.Content() != body || body.Parent() != group {
		t.Fatal("decl builder parented the child without registering group content")
	}
	if !group.IsCheckable() || group.IsChecked() || !enabled.IsChecked() {
		t.Fatal("persisted group or nested checkbox state was lost")
	}
	if body.IsEnabled() || enabled.IsEnabled() || disabled.IsEnabled() {
		t.Fatal("unchecked content accepted input")
	}
	if !enabled.IsExplicitlyEnabled() || disabled.IsExplicitlyEnabled() {
		t.Fatal("group gate changed authored child flags")
	}
	x, y, width, height := body.Bounds()
	if x <= 0 || y <= 0 || width <= 0 || height <= 0 || x+width > group.Width() || y+height > group.Height() {
		t.Fatal("decl content did not receive the group layout", body.Bounds1())
	}
	group.SetChecked(true)
	if !body.IsEnabled() || !enabled.IsEnabled() || disabled.IsEnabled() {
		t.Fatal("reenabling did not restore only authored-enabled content")
	}
	group.SetChecked(false)
	if enabled.IsEnabled() || disabled.IsEnabled() {
		t.Fatal("second toggle bypassed the group content gate")
	}
}
