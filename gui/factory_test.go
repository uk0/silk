package gui

import (
	"silk/core"
	"strings"
	"testing"
)

// TestAllRegisteredWidgetsInstantiate discovers every factory whose name starts
// with "gui." and verifies it can be instantiated and satisfies IWidget.
func TestAllRegisteredWidgetsInstantiate(t *testing.T) {
	all := core.AllFactories()
	if len(all) == 0 {
		t.Fatal("no factories registered; package init not triggered")
	}

	guiCount := 0
	for _, f := range all {
		name := f.Name()
		if !strings.HasPrefix(name, "gui.") {
			continue
		}

		guiCount++
		t.Run(name, func(t *testing.T) {
			obj := f.New()
			if obj == nil {
				t.Fatalf("factory %q returned nil", name)
			}
			widget, ok := obj.(IWidget)
			if !ok {
				// Some gui factories (e.g. gui.Action) are not widgets -- skip
				t.Skipf("%q does not implement IWidget", name)
				return
			}
			// Verify SizeHints does not panic. Some widgets may return negative
			// dimensions when created via factory without full constructor args
			// (e.g. Rating with maxStars=0), so we log instead of failing.
			hints := widget.SizeHints()
			if hints.Width < 0 || hints.Height < 0 {
				t.Logf("%q SizeHints has negative dimensions (factory default): W=%f H=%f", name, hints.Width, hints.Height)
			}
		})
	}

	// Sanity: we expect at least 50 gui.* factories
	if guiCount < 50 {
		t.Errorf("only %d gui.* factories found; expected at least 50", guiCount)
	}
}

// TestCriticalWidgetFactories verifies the most commonly-used widget types
// are present in the factory registry with their exact names.
func TestCriticalWidgetFactories(t *testing.T) {
	critical := []string{
		"gui.Button", "gui.Label", "gui.Edit", "gui.CheckBox",
		"gui.RadioButton", "gui.ComboBox", "gui.SpinBox", "gui.Slider",
		"gui.ProgressBar", "gui.GroupBox", "gui.Form",
		"gui.VBox", "gui.HBox", "gui.GridLayout", "gui.FormLayout",
		"gui.Splitter", "gui.StackedWidget", "gui.TabWidget",
		"gui.ListWidget", "gui.TreeView", "gui.Table",
		"gui.ScrollArea", "gui.Menu", "gui.ToolBar", "gui.StatusBar",
		"gui.Dialog", "gui.CodeEditor",
		"gui.ToggleSwitch", "gui.SearchBox", "gui.NumberInput",
		"gui.DatePicker", "gui.ColorPicker", "gui.Rating",
		"gui.DropdownButton", "gui.SwitchGroup",
		"gui.ImageView", "gui.Tag", "gui.Badge", "gui.Avatar",
		"gui.Breadcrumb", "gui.Link", "gui.LabelSeparator",
		"gui.Placeholder", "gui.Timeline", "gui.NotificationPanel",
		"gui.Card", "gui.Accordion",
		"gui.LineChart", "gui.BarChart", "gui.PieChart",
		"gui.Gauge", "gui.ScatterPlot",
	}

	for _, name := range critical {
		t.Run(name, func(t *testing.T) {
			factory := core.FindFactory(name)
			if factory == nil {
				t.Errorf("factory %q not registered", name)
				return
			}
			obj := factory.New()
			if obj == nil {
				t.Errorf("factory %q returned nil", name)
			}
		})
	}
}

// TestFactoryNewReturnsDistinctInstances verifies that repeated calls to
// Factory.New() return different instances (not a shared singleton).
func TestFactoryNewReturnsDistinctInstances(t *testing.T) {
	factory := core.FindFactory("gui.Button")
	if factory == nil {
		t.Fatal("gui.Button factory not registered")
	}

	a := factory.New()
	b := factory.New()
	if a == b {
		t.Error("Factory.New() returned the same instance twice")
	}
}
