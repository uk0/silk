package gui

import (
	"testing"

	"github.com/uk0/silk/core"
)

func isolateEnabledTest(t *testing.T) {
	t.Helper()
	oldFocus, oldMouse, oldHover, oldCapture := focusWidget, lastMouseWidget, mouseHoverWidget, captureStack
	oldActive := activeForm
	focusWidget, lastMouseWidget, mouseHoverWidget, captureStack = nil, nil, nil, nil
	resetUIQueue()
	t.Cleanup(func() {
		releaseCapture()
		focusWidget, lastMouseWidget, mouseHoverWidget, captureStack = oldFocus, oldMouse, oldHover, oldCapture
		activeForm = oldActive
		resetUIQueue()
	})
}

func TestEnabledInheritancePreservesExplicitFlags(t *testing.T) {
	isolateEnabledTest(t)
	root, inner := NewVBox(), NewVBox()
	button := NewButton()
	inner.SetParent(root)
	button.SetParent(inner)
	root.SetEnabled(false)
	if root.IsEnabled() || inner.IsEnabled() || button.IsEnabled() || !inner.IsExplicitlyEnabled() || !button.IsExplicitlyEnabled() {
		t.Fatal("disabled ancestor did not gate effective state independently of authored flags")
	}
	button.SetEnabled(false)
	root.SetEnabled(true)
	if !inner.IsEnabled() || button.IsEnabled() || button.IsExplicitlyEnabled() {
		t.Fatal("reenabling ancestor overwrote an explicitly disabled child")
	}
	button.SetEnabled(true)
	if !button.IsEnabled() {
		t.Fatal("explicit reenabling failed")
	}
}

func TestEnabledIndependentNativeBoundaries(t *testing.T) {
	isolateEnabledTest(t)
	for _, kind := range []WindowType{WtForm, WtPopup, WtChild} {
		root := NewVBox()
		child := NewVBox()
		child.SetParent(root)
		button := NewButton()
		button.SetParent(child)
		// Metadata-only boundary: set it after parenting, which otherwise
		// legitimately creates a native window (main-thread-only on macOS).
		child.wt = kind
		root.SetEnabled(false)
		want := kind != WtChild
		if child.IsEnabled() != want || button.IsEnabled() != want {
			t.Fatalf("native boundary %v inherited wrong input state", kind)
		}
	}
}

func TestEnabledDerivedParentActionParticipates(t *testing.T) {
	isolateEnabledTest(t)
	parent, child := NewButton(), NewButton()
	child.SetParent(parent)
	parent.Action().SetEnabled(false)
	if child.IsEnabled() || !child.IsExplicitlyEnabled() {
		t.Fatal("derived parent's disabled Action was ignored")
	}
	parent.Action().SetEnabled(true)
	if !child.IsEnabled() {
		t.Fatal("derived parent recovery failed")
	}
}

func TestEnabledPropertiesPersistOnlyLocalFlags(t *testing.T) {
	isolateEnabledTest(t)
	switchWidget := new(ToggleSwitch)
	switchWidget.Init(switchWidget)
	widgets := []IWidget{NewButton(), NewCheckBox(), NewComboBox(), switchWidget}
	for _, widget := range widgets {
		parent := NewVBox()
		widget.SetParent(parent)
		parent.SetEnabled(false)
		properties := new(recordingPropertyList)
		widget.(interface{ EnumProperties(core.IPropertyList) }).EnumProperties(properties)
		property, ok := properties.find("可用")
		if !ok || !property.get.(func() bool)() || widget.IsEnabled() {
			t.Fatalf("%T persisted an inherited disabled state", widget)
		}
		property.set.(func(bool))(false)
		parent.SetEnabled(true)
		if property.get.(func() bool)() || widget.IsEnabled() {
			t.Fatalf("%T did not retain explicit disabled state", widget)
		}
	}
}

func TestGroupBoxGateNestedInputAndExplicitState(t *testing.T) {
	isolateEnabledTest(t)
	group, inner := NewGroupBox("Control"), NewVBox()
	button, explicit := NewButton(), NewButton()
	explicit.SetEnabled(false)
	inner.AddWidget(button)
	inner.AddWidget(explicit)
	group.SetContent(inner)
	group.SetCheckable(true)
	fired := 0
	button.Action().BindFunc0(func() { fired++ })
	group.SetChecked(false)
	button.OnKeyDown(KeySpace, false)
	if fired != 0 || button.IsEnabled() || inner.IsEnabled() || !button.IsExplicitlyEnabled() || !group.IsEnabled() {
		t.Fatal("unchecked content was not gated separately from the header/local flags")
	}
	group.SetChecked(true)
	button.OnKeyDown(KeySpace, false)
	if fired != 1 || !button.IsEnabled() || explicit.IsEnabled() {
		t.Fatal("checked state did not restore only eligible children")
	}
	group.SetChecked(false)
	group.SetCheckable(false)
	if !button.IsEnabled() || explicit.IsEnabled() {
		t.Fatal("removing the gate overwrote local flags")
	}
}

func TestGroupBoxGateReplacementAndReparent(t *testing.T) {
	isolateEnabledTest(t)
	group := NewGroupBox("Control")
	group.SetCheckable(true)
	old := NewButton()
	group.SetContent(old)
	old.SetParent(NewVBox())
	group.SetChecked(false)
	if !old.IsEnabled() {
		t.Fatal("former owner changed detached content")
	}
	next := NewButton()
	group.SetContent(next)
	if next.IsEnabled() || !next.IsExplicitlyEnabled() || !old.IsEnabled() {
		t.Fatal("replacement did not inherit only the current owner's gate")
	}
	if group.IsChildEnabled(nil) {
		t.Fatal("nil is not an eligible child")
	}
}

func TestGroupBoxFactoryInitDefaultsChecked(t *testing.T) {
	group := new(GroupBox)
	group.Init(group)
	if !group.IsChecked() {
		t.Fatal("factory Init disagrees with the public constructor")
	}
}

func TestEnabledTemporaryGateKeepsFocusButFencesPaste(t *testing.T) {
	isolateEnabledTest(t)
	group, inner, edit := NewGroupBox("Edit"), NewVBox(), NewCodeEditor()
	inner.AddWidget(edit)
	group.SetContent(inner)
	group.SetCheckable(true)
	edit.SetText("safe")
	edit.SetFocus()
	reader := new(fakeClipboardRead)
	edit.clipboardReader = reader.read
	edit.pasteClipboard()
	group.SetChecked(false)
	if edit.PastePending() || reader.ctx.Err() == nil || CurrentFocusWidget() != edit || edit.IsEnabled() || edit.HasFocus() {
		t.Fatal("gate must cancel admitted paste, not forget remembered focus")
	}
	group.SetChecked(true)
	reader.done("late", nil)
	if edit.Text() != "safe" || !edit.IsEnabled() || CurrentFocusWidget() != edit || !edit.HasFocus() {
		t.Fatal("reenabling revived stale clipboard work or lost focus")
	}
}

func TestEnabledIndependentNativePasteIsNotCancelled(t *testing.T) {
	isolateEnabledTest(t)
	root, edit := NewVBox(), NewCodeEditor()
	edit.SetParent(root)
	edit.wt = WtForm
	edit.SetText("safe")
	reader := new(fakeClipboardRead)
	edit.clipboardReader = reader.read
	edit.pasteClipboard()
	root.SetEnabled(false)
	if !edit.PastePending() || reader.ctx.Err() != nil || !edit.IsEnabled() {
		t.Fatal("owner's gate canceled an independent native form")
	}
	reader.done("ok", nil)
	if edit.Text() != "oksafe" {
		t.Fatal("independent native delivery failed")
	}
}

func TestEnabledDisabledRootHasNoKeyboardFallback(t *testing.T) {
	isolateEnabledTest(t)
	root := NewVBox()
	win := &Window{widget: root}
	root.SetEnabled(false)
	if win.keyTarget() != nil || win.textInputTarget() != nil {
		t.Fatal("disabled root accepted fallback input")
	}
}

func TestEnabledCycleRejectionPreservesPendingInput(t *testing.T) {
	isolateEnabledTest(t)
	root, edit := NewVBox(), NewCodeEditor()
	edit.SetParent(root)
	reader := new(fakeClipboardRead)
	edit.clipboardReader = reader.read
	edit.pasteClipboard()
	root.SetParent(edit)
	edit.SetParent(edit)
	if root.Parent() != nil || edit.Parent() != root || !edit.PastePending() || reader.ctx.Err() != nil {
		t.Fatal("invalid parent request changed tree or input before rejection")
	}
	edit.CancelPendingPaste()
}

func TestEnabledCaptureCancellationRetiresOnlyGesture(t *testing.T) {
	isolateEnabledTest(t)
	root := NewVBox()
	slider := NewSlider(0, 100)
	slider.SetParent(root)
	slider.SetSize(200, 30)
	slider.OnLeftDown(70, 15)
	value := slider.Value()
	if !slider.dragging || curCapture() != slider {
		t.Fatal("fixture did not start a captured slider gesture")
	}
	root.SetEnabled(false)
	if slider.dragging || curCapture() != nil || lastMouseWidget == slider {
		t.Fatal("disabled ancestor left a captured gesture active")
	}
	root.SetEnabled(true)
	slider.OnMouseMove(180, 15)
	if slider.Value() != value {
		t.Fatal("reenabling resumed an old slider drag without a new press")
	}
}

func TestEnabledBuiltinCancellationDoesNotCommit(t *testing.T) {
	isolateEnabledTest(t)
	button := NewButton()
	fired := 0
	button.Action().BindFunc0(func() { fired++ })
	button.pushed = true
	button.OnPointerCancel()
	if button.pushed || fired != 0 {
		t.Fatal("cancel synthesized a button release/action")
	}
	check := NewCheckBox()
	check.pushed = true
	check.OnPointerCancel()
	toggle := new(ToggleSwitch)
	toggle.Init(toggle)
	toggle.pushed = true
	toggle.OnPointerCancel()
	if check.pushed || check.IsChecked() || toggle.pushed || toggle.IsChecked() {
		t.Fatal("cancel toggled a checkable control")
	}
	edit := NewEdit()
	edit.SetText("keep")
	edit.mouseDown, edit.clickSelection = true, true
	edit.OnPointerCancel()
	code := NewCodeEditor()
	code.SetText("keep")
	code.mouseDown, code.minimapDragging = true, true
	code.OnPointerCancel()
	if edit.mouseDown || edit.clickSelection || code.mouseDown || code.minimapDragging || edit.Text() != "keep" || code.Text() != "keep" {
		t.Fatal("cancel changed text or retained editor pointer state")
	}
}

func TestEnabledScrollbarLateTimerCannotStep(t *testing.T) {
	isolateEnabledTest(t)
	root, scroll := NewVBox(), NewScrollBar()
	scroll.SetParent(root)
	scroll.SetRange(0, 100)
	scroll.SetValue(40)
	scroll.pushed, scroll.part = true, 5
	lastMouseWidget = scroll // native PRESS arms before the first MOVE captures
	root.SetEnabled(false)
	scroll.onTimer()
	if scroll.Value() != 40 || scroll.pushed || scroll.part != 0 {
		t.Fatal("disabled repeater processed a late timer or retained press state")
	}
}

type enabledMoveOnCancel struct {
	Button
	destination IWidget
	calls       int
}

func (b *enabledMoveOnCancel) OnPointerCancel() {
	b.calls++
	b.pushed = false
	b.SetParent(b.destination)
	// The gesture is retired, but backend capture cleanup is the dispatcher's
	// job. Reparenting must not make this old capture a new eligible gesture.
}

func TestEnabledCancellationReparentCannotReviveCapture(t *testing.T) {
	isolateEnabledTest(t)
	root, destination := NewVBox(), NewVBox()
	button := new(enabledMoveOnCancel)
	button.Init(button)
	button.SetText("Reparent on cancel")
	button.destination = destination
	button.SetParent(root)
	button.pushed = true
	button.PushCapture()
	lastMouseWidget = button
	root.SetEnabled(false)
	if button.calls != 1 || button.Parent() != destination || !button.IsEnabled() || curCapture() != nil || lastMouseWidget == button {
		t.Fatal("reentrant callback revived old capture or was canceled twice")
	}
}

type enabledGateFixture struct {
	VBox
	open bool
}

func (g *enabledGateFixture) IsChildEnabled(IWidget) bool { return g.open }

func TestEnabledCustomGateAndProgrammaticAction(t *testing.T) {
	isolateEnabledTest(t)
	gate := new(enabledGateFixture)
	gate.Init(gate)
	gate.open = true
	button := NewButton1("Action", nil)
	button.SetParent(gate)
	fired := 0
	button.Action().BindFunc0(func() { fired++ })
	button.pushed = true
	button.PushCapture()
	gate.open = false
	button.InvalidateEnabledState()
	if button.IsEnabled() || button.pushed || curCapture() != nil || !button.IsExplicitlyEnabled() {
		t.Fatal("custom gate did not synchronously retire input without changing authored state")
	}
	// Calling an enabled Action explicitly is model code, not simulated input.
	button.Action().Trigger(nil)
	if fired != 1 {
		t.Fatal("a container's input gate changed the independent Action model")
	}
	gate.open = true
	button.InvalidateEnabledState()
	button.OnKeyDown(KeySpace, false)
	if fired != 2 || !button.IsEnabled() {
		t.Fatal("custom gate did not recover")
	}
}

func TestEnabledEditorRetiresTransientRepliesNotDocument(t *testing.T) {
	isolateEnabledTest(t)
	root, editor := NewVBox(), NewCodeEditor()
	editor.SetParent(root)
	editor.SetText("package main")
	editor.SetFocus()
	editor.SetExternalCompletions([]ExternalCompletion{{Label: "candidate"}})
	editor.signatureHintVisible = true
	beforeText, beforeHint := editor.TextRevision(), editor.LanguageHintRevision()
	root.SetEnabled(false)
	if len(editor.externalCompletions) != 0 || editor.signatureHintVisible || editor.LanguageHintRevision() == beforeHint || editor.CompletionIndexLoading() {
		t.Fatal("disabled editor retained transient language UI or accepted-reply token")
	}
	root.SetEnabled(true)
	if editor.Text() != "package main" || editor.TextRevision() != beforeText || !editor.HasFocus() || len(editor.externalCompletions) != 0 {
		t.Fatal("reenabling lost document/focus or revived old candidates")
	}
}

type enabledPointerProbe struct {
	Widget
	downs, ups int
}

func TestEnabledReparentIntoDisabledRetiresGesture(t *testing.T) {
	isolateEnabledTest(t)
	root, disabled := NewVBox(), NewVBox()
	disabled.SetEnabled(false)
	slider := NewSlider(0, 100)
	slider.SetParent(root)
	slider.SetSize(200, 30)
	slider.OnLeftDown(70, 15)
	value := slider.Value()
	slider.SetParent(disabled)
	if slider.IsEnabled() || slider.dragging || curCapture() != nil {
		t.Fatal("reparent into a disabled hierarchy retained an old gesture")
	}
	slider.SetParent(root)
	slider.OnMouseMove(180, 15)
	if slider.Value() != value {
		t.Fatal("returning to an enabled parent revived an old drag")
	}
}

func TestEnabledCaptureStackClearsRetiredReferences(t *testing.T) {
	isolateEnabledTest(t)
	parent, child := NewButton1("parent", nil), NewButton1("child", nil)
	child.SetParent(parent)
	captureStack = []IWidget{parent, child}
	backing := captureStack[:cap(captureStack)]
	child.PopCapture()
	if curCapture() != parent || backing[1] != nil {
		t.Fatal("restoring capture retained the retired child in the backing array")
	}
	parent.PopCapture()
	if curCapture() != nil || backing[0] != nil {
		t.Fatal("empty capture stack retained its former owner")
	}
	captureStack = []IWidget{parent, child}
	backing = captureStack[:cap(captureStack)]
	other := NewButton1("independent", nil)
	other.PushCapture()
	if curCapture() != other || backing[1] != nil {
		t.Fatal("replacing capture retained a discarded subtree")
	}
}

// Older SDK controls can override IsEnabled with only their local state.
// Native input must still enforce container gates around such a control.
type enabledLegacyProbe struct {
	enabledPointerProbe
	input, cancelled int
}

func (p *enabledLegacyProbe) IsEnabled() bool    { return true }
func (p *enabledLegacyProbe) OnTextInput(string) { p.input++ }
func (p *enabledLegacyProbe) OnPointerCancel()   { p.cancelled++; p.PopCapture() }

func TestEnabledLegacyOverrideCannotBypassInputScope(t *testing.T) {
	isolateEnabledTest(t)
	root := NewVBox()
	win := inputScopeWindow(root, WtForm, nil)
	probe := new(enabledLegacyProbe)
	probe.Init(probe)
	probe.SetParent(root)
	probe.SetFocus()
	probe.PushCapture()
	root.SetEnabled(false)
	if target := win.textInputTarget(); target != nil {
		target.OnTextInput("must not arrive")
	}
	if probe.input != 0 || probe.cancelled != 1 || curCapture() != nil || probe.HasFocus() {
		t.Fatal("legacy local-only IsEnabled override bypassed a disabled ancestor")
	}
	root.SetEnabled(true)
	if win.textInputTarget() != probe || !probe.HasFocus() || curCapture() != nil {
		t.Fatal("recovery lost remembered input or revived an old capture")
	}
	probe.Widget.SetEnabled(false)
	if win.textInputTarget() != nil || probe.HasFocus() {
		t.Fatal("local override bypassed an explicit base disable")
	}
}

func TestEnabledLegacyOverrideCannotBypassGroupGate(t *testing.T) {
	isolateEnabledTest(t)
	group := NewGroupBox("Legacy SDK control")
	probe := new(enabledLegacyProbe)
	probe.Init(probe)
	probe.SetFocusPolicy(TabFocus)
	group.SetContent(probe)
	group.SetCheckable(true)
	probe.SetFocus()
	probe.PushCapture()
	group.SetChecked(false)
	if inputEnabled(probe) || probe.HasFocus() || curCapture() != nil || probe.cancelled != 1 || isTabFocusable(probe) {
		t.Fatal("legacy override bypassed a compound control's child gate")
	}
	group.SetChecked(true)
	if !inputEnabled(probe) || !probe.HasFocus() || !isTabFocusable(probe) {
		t.Fatal("removing the gate did not restore eligible legacy input")
	}
}

func (p *enabledPointerProbe) OnLeftDown(float64, float64)   { p.downs++ }
func (p *enabledPointerProbe) OnLeftUp(float64, float64)     { p.ups++ }
func (p *enabledPointerProbe) OnRightDown(float64, float64)  { p.downs++ }
func (p *enabledPointerProbe) OnRightUp(float64, float64)    { p.ups++ }
func (p *enabledPointerProbe) OnMiddleDown(float64, float64) { p.downs++ }
func (p *enabledPointerProbe) OnMiddleUp(float64, float64)   { p.ups++ }
