package gui

import "testing"

func groupBoxKeyboard(t *testing.T, group *GroupBox) IEventKeyDown {
	t.Helper()
	handler, ok := interface{}(group).(IEventKeyDown)
	if !ok {
		t.Fatal("checkable GroupBox has no native keyboard handler")
	}
	return handler
}

func TestGroupBoxKeyboardSpaceIgnoresRepeatAndKeepsAuthoredState(t *testing.T) {
	isolateEnabledTest(t)
	group := NewGroupBox("Options")
	group.SetCheckable(true)
	content := NewVBox()
	field, disabled := NewEdit(), NewEdit()
	disabled.SetEnabled(false)
	content.AddWidget(field)
	content.AddWidget(disabled)
	group.SetContent(content)
	var toggled []bool
	group.SigToggled(func(value bool) { toggled = append(toggled, value) })
	key := groupBoxKeyboard(t, group)
	key.OnKeyDown(KeySpace, false)
	if group.IsChecked() || field.IsEnabled() || !field.IsExplicitlyEnabled() {
		t.Fatal("Space did not gate nested content without changing authored state")
	}
	key.OnKeyDown(KeySpace, true)
	key.OnKeyDown(KeyEnter, false)
	key.OnKeyDown(KeyLeft, false)
	if group.IsChecked() || len(toggled) != 1 {
		t.Fatal("repeat, Enter or an unrelated key toggled the header")
	}
	key.OnKeyDown(KeySpace, false)
	if !group.IsChecked() || !field.IsEnabled() || disabled.IsEnabled() || len(toggled) != 2 || toggled[0] || !toggled[1] {
		t.Fatalf("keyboard recovery changed explicit state or callback count: %v", toggled)
	}
	group.SetEnabled(false)
	key.OnKeyDown(KeySpace, false)
	group.SetEnabled(true)
	group.SetCheckable(false)
	key.OnKeyDown(KeySpace, false)
	if !group.IsChecked() || len(toggled) != 2 {
		t.Fatal("disabled/plain GroupBox accepted a keyboard toggle")
	}
}

func TestGroupBoxKeyboardTabVisitsHeaderAndEligibleContent(t *testing.T) {
	isolateEnabledTest(t)
	root := NewVBox()
	before, after := NewEdit(), NewEdit()
	group := NewGroupBox("Options")
	inside := NewEdit()
	group.SetContent(inside)
	root.AddWidget(before)
	root.AddWidget(group)
	root.AddWidget(after)
	if isTabFocusable(group) {
		t.Fatal("a plain GroupBox added an extra automatic Tab stop")
	}
	group.SetCheckable(true)
	if got := nextFocusable(root, before, true); got != group {
		t.Fatalf("Tab skips the checkable header: got %T", got)
	}
	if got := nextFocusable(root, group, true); got != inside {
		t.Fatal("checked content is missing from the Tab chain")
	}
	win := inputScopeWindow(root, WtForm, nil)
	group.SetFocus()
	if win.keyTarget() != group {
		t.Fatal("native key admission does not target the focused header")
	}
	groupBoxKeyboard(t, group).OnKeyDown(KeySpace, false)
	if !group.HasFocus() || win.keyTarget() != group || nextFocusable(root, group, true) != after {
		t.Fatal("unchecked header lost focus or retained a disabled content Tab stop")
	}
	if nextFocusable(root, after, false) != group {
		t.Fatal("reverse Tab cannot reach the unchecked header")
	}
	group.SetFocusPolicy(NoFocus)
	if isTabFocusable(group) {
		t.Fatal("checkable header overrode explicit NoFocus")
	}
	group.SetCheckable(false)
	group.SetFocusPolicy(TabFocus)
	if !isTabFocusable(group) {
		t.Fatal("plain frame overrode an explicit TabFocus policy")
	}
}

func TestGroupBoxEditableCheckStateProperties(t *testing.T) {
	isolateEnabledTest(t)
	group := NewGroupBox("Options")
	properties := new(recordingPropertyList)
	group.EnumProperties(properties)
	checkable, ok := properties.find("Checkable")
	if !ok {
		t.Fatal("GroupBox does not expose checkability to the property sheet")
	}
	checked, ok := properties.find("Checked")
	if !ok {
		t.Fatal("GroupBox does not expose its authored checked state")
	}
	checkable.set.(func(bool))(true)
	checked.set.(func(bool))(false)
	if !checkable.get.(func() bool)() || checked.get.(func() bool)() {
		t.Fatal("editable check-state properties disagree with the SDK")
	}
}

func TestGroupBoxKeyboardDisabledAncestorCannotToggleHeader(t *testing.T) {
	isolateEnabledTest(t)
	parent := NewGroupBox("Outer")
	parent.SetCheckable(true)
	group := NewGroupBox("Inner")
	group.SetCheckable(true)
	parent.SetContent(group)
	parent.SetChecked(false)
	groupBoxKeyboard(t, group).OnKeyDown(KeySpace, false)
	if !group.IsChecked() || isTabFocusable(group) {
		t.Fatal("an ancestor gate allowed header input or Tab admission")
	}
}
