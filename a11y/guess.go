package a11y

import (
	"reflect"
	"strings"
)

// guess.go: heuristic role inference for widgets that don't implement
// AccessibleRole. The strategy is deliberately conservative — peek at
// the runtime type name suffix and map it to a Role. False matches are
// rare because the gui package's naming is consistent (gui.Button,
// gui.CheckBox, gui.Slider, etc.), but anything we can't recognise
// returns RoleUnknown so the caller still sees the node.

var typeRoleMap = map[string]Role{
	// Inputs
	"Button":         RoleButton,
	"DropdownButton": RoleButton,
	"CheckBox":       RoleCheckBox,
	"RadioButton":    RoleRadioButton,
	"ToggleSwitch":   RoleSwitch,
	"SwitchGroup":    RoleSwitch,
	"Edit":           RoleEdit,
	"NumberInput":    RoleSpinBox,
	"SpinBox":        RoleSpinBox,
	"Slider":         RoleSlider,
	"ProgressBar":    RoleProgressBar,
	"ComboBox":       RoleComboBox,
	"SearchBox":      RoleSearchBox,
	"DatePicker":     RoleEdit,
	"ColorPicker":    RoleEdit,
	"Rating":         RoleSlider,
	"Link":           RoleLink,

	// Display
	"Label":          RoleLabel,
	"LabelSeparator": RoleSeparator,
	"Tag":            RoleLabel,
	"Badge":          RoleBadge,
	"Avatar":         RoleImage,
	"Breadcrumb":     RoleGroup,
	"ImageView":      RoleImage,
	"Placeholder":    RoleGroup,
	"Timeline":       RoleList,

	// Layout
	"VBox":        RoleGroup,
	"HBox":        RoleGroup,
	"GridLayout":  RoleGroup,
	"FormLayout":  RoleGroup,
	"GroupBox":    RoleGroup,
	"Card":        RoleCard,
	"Splitter":    RoleSplitter,
	"StackedWidget": RoleGroup,
	"TabWidget":   RoleTabPanel,
	"ScrollArea":  RoleGroup,
	"Accordion":   RoleAccordion,

	// Data
	"ListWidget":         RoleList,
	"TreeView":           RoleTree,
	"Table":              RoleTable,
	"NotificationPanel":  RoleNotification,

	// Charts (semantically images for a11y)
	"LineChart":   RoleImage,
	"BarChart":    RoleImage,
	"PieChart":    RoleImage,
	"Gauge":       RoleImage,
	"ScatterPlot": RoleImage,

	// Window chrome
	"Form":      RoleWindow,
	"Dialog":    RoleDialog,
	"Menu":      RoleMenu,
	"ToolBar":   RoleToolBar,
	"StatusBar": RoleStatusBar,
}

// guessRole inspects the runtime type name of w and looks up the
// matching Role in typeRoleMap. The lookup uses the un-pointer-stripped
// short name (e.g. "Button" from "*gui.Button"). Returns RoleUnknown
// when nothing matches.
func guessRole(w interface{}) Role {
	if w == nil {
		return RoleUnknown
	}
	t := reflect.TypeOf(w)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	name := t.Name()
	if name == "" {
		return RoleUnknown
	}
	if r, ok := typeRoleMap[name]; ok {
		return r
	}
	// Fallback: look for a known suffix. Some custom widgets use
	// patterns like "MyButton" or "TaggedLabel" that share semantics
	// with a base widget without re-exporting the same struct name.
	for suffix, role := range typeRoleMap {
		if strings.HasSuffix(name, suffix) {
			return role
		}
	}
	return RoleUnknown
}
