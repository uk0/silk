package a11y

import (
	"testing"
)

// TestRoleString covers the public string repr — used by debug tools
// and snapshot tests, must stay stable across releases.
func TestRoleString(t *testing.T) {
	cases := []struct {
		r Role
		s string
	}{
		{RoleUnknown, "Unknown"},
		{RoleButton, "Button"},
		{RoleCheckBox, "CheckBox"},
		{RoleRadioButton, "RadioButton"},
		{RoleEdit, "Edit"},
		{RoleSlider, "Slider"},
		{RoleProgressBar, "ProgressBar"},
		{RoleWindow, "Window"},
		{RoleDialog, "Dialog"},
		{RoleTabPanel, "TabPanel"},
	}
	for _, c := range cases {
		if got := c.r.String(); got != c.s {
			t.Errorf("Role(%d).String() = %q, want %q", c.r, got, c.s)
		}
	}
}

// TestStateBitmaskOps locks Has/With/Without semantics so they
// behave like a clean immutable bitmask API.
func TestStateBitmaskOps(t *testing.T) {
	s := State(0)
	s = s.With(StateFocused)
	if !s.Has(StateFocused) {
		t.Error("Has(Focused) false after With(Focused)")
	}
	if s.Has(StateChecked) {
		t.Error("Has(Checked) true without With(Checked)")
	}
	s = s.With(StateChecked)
	if !s.Has(StateFocused | StateChecked) {
		t.Error("composite Has(Focused|Checked) false after both With()")
	}
	s = s.Without(StateFocused)
	if s.Has(StateFocused) {
		t.Error("Has(Focused) still true after Without")
	}
	if !s.Has(StateChecked) {
		t.Error("Without(Focused) clobbered Checked")
	}
}

// fakeAccessible is a minimal struct that implements the explicit
// Accessible interface — used to verify the explicit path overrides
// duck-typed inference.
type fakeAccessible struct {
	role Role
	name string
	desc string
	val  string
	st   State
}

func (f *fakeAccessible) AccessibleRole() Role          { return f.role }
func (f *fakeAccessible) AccessibleName() string        { return f.name }
func (f *fakeAccessible) AccessibleDescription() string { return f.desc }
func (f *fakeAccessible) AccessibleValue() string       { return f.val }
func (f *fakeAccessible) AccessibleState() State        { return f.st }

func TestAccessibleExplicitInterfaceWins(t *testing.T) {
	f := &fakeAccessible{
		role: RoleSlider,
		name: "Volume",
		desc: "Master volume 0-100",
		val:  "42",
		st:   StateFocused,
	}
	n := Walk(f)
	if n == nil {
		t.Fatal("Walk returned nil")
	}
	if n.Role != RoleSlider {
		t.Errorf("Role = %v, want Slider", n.Role)
	}
	if n.Name != "Volume" || n.Description != "Master volume 0-100" || n.Value != "42" {
		t.Errorf("metadata mismatch: %+v", n)
	}
	if !n.State.Has(StateFocused) {
		t.Errorf("State missing Focused: %v", n.State)
	}
}

// duckButton emulates a gui-style Button without implementing
// Accessible. Provides Text() and IsEnabled() to exercise the
// duck-typed fallback path.
type duckButton struct {
	text    string
	enabled bool
}

func (b *duckButton) Text() string    { return b.text }
func (b *duckButton) IsEnabled() bool { return b.enabled }

func TestDuckTypedNameAndState(t *testing.T) {
	b := &duckButton{text: "Save", enabled: false}
	n := Walk(b)
	if n == nil {
		t.Fatal("Walk returned nil")
	}
	if n.Name != "Save" {
		t.Errorf("Name = %q, want Save", n.Name)
	}
	if !n.State.Has(StateDisabled) {
		t.Errorf("State should include Disabled: got %v", n.State)
	}
}

// duckButtonNamed embeds a "Button" suffix in its type name to verify
// guessRole's reflection-based suffix fallback.
type myButton struct {
	text string
}

func (b *myButton) Text() string { return b.text }

func TestRoleInferredFromTypeName(t *testing.T) {
	b := &myButton{text: "OK"}
	n := Walk(b)
	if n == nil {
		t.Fatal("Walk returned nil")
	}
	if n.Role != RoleButton {
		t.Errorf("Role = %v, want Button (inferred from type name 'myButton')", n.Role)
	}
}

// duckCheckBox covers checked-state inference.
type duckCheckBox struct {
	checked bool
}

func (c *duckCheckBox) IsChecked() bool { return c.checked }
func (c *duckCheckBox) Text() string    { return "Enable feature" }

func TestDuckTypedCheckedState(t *testing.T) {
	c := &duckCheckBox{checked: true}
	n := Walk(c)
	if !n.State.Has(StateChecked) {
		t.Errorf("State should include Checked: got %v", n.State)
	}
}

// duckSlider exercises Value()-as-float64 → string formatting.
type duckSlider struct {
	v float64
}

func (s *duckSlider) Value() float64 { return s.v }

func TestDuckTypedValueFloat(t *testing.T) {
	s := &duckSlider{v: 0.75}
	n := Walk(s)
	if n.Value != "0.75" {
		t.Errorf("Value = %q, want 0.75", n.Value)
	}
	// Integer-valued slider should print as integer.
	s.v = 50
	n = Walk(s)
	if n.Value != "50" {
		t.Errorf("integer slider Value = %q, want 50", n.Value)
	}
}

// composite is a synthetic container that exposes the
// childrenAdapter interface — used to verify Walk recurses correctly.
type composite struct {
	name     string
	hidden   bool
	children []interface{}
}

func (c *composite) Title() string                 { return c.name }
func (c *composite) IsVisible() bool               { return !c.hidden }
func (c *composite) AccessibleChildren() []interface{} { return c.children }

func TestWalkRecursesIntoChildren(t *testing.T) {
	root := &composite{
		name: "Form",
		children: []interface{}{
			&duckButton{text: "OK", enabled: true},
			&duckCheckBox{checked: false},
			&composite{
				name: "Inner",
				children: []interface{}{
					&duckButton{text: "Cancel", enabled: true},
				},
			},
		},
	}

	n := Walk(root)
	if n == nil {
		t.Fatal("nil root")
	}
	if got := len(n.Children); got != 3 {
		t.Fatalf("root.Children = %d, want 3", got)
	}
	if n.Children[0].Name != "OK" {
		t.Errorf("first child name = %q", n.Children[0].Name)
	}
	if n.Children[2].Name != "Inner" || len(n.Children[2].Children) != 1 {
		t.Errorf("inner composite shape wrong: %+v", n.Children[2])
	}
	if n.Children[2].Children[0].Name != "Cancel" {
		t.Errorf("nested grandchild name = %q", n.Children[2].Children[0].Name)
	}
}

// TestWalkSkipsHiddenWidgets verifies the default Walk filters hidden
// subtrees but WalkAll keeps them.
func TestWalkSkipsHiddenWidgets(t *testing.T) {
	visible := &composite{name: "V", children: []interface{}{&duckButton{text: "Visible", enabled: true}}}
	hidden := &composite{name: "H", hidden: true, children: []interface{}{&duckButton{text: "Hidden", enabled: true}}}

	root := &composite{
		name:     "Root",
		children: []interface{}{visible, hidden},
	}

	n := Walk(root)
	if got := len(n.Children); got != 1 {
		t.Errorf("Walk should drop hidden subtree; root has %d kids", got)
	}
	if n.Children[0].Name != "V" {
		t.Errorf("retained child name = %q", n.Children[0].Name)
	}

	all := WalkAll(root)
	if got := len(all.Children); got != 2 {
		t.Errorf("WalkAll should keep hidden subtree; root has %d kids", got)
	}
}

// TestWalkNilSafe protects the package from a nil root crashing the
// caller — common when bridges connect before any window is open.
func TestWalkNilSafe(t *testing.T) {
	if n := Walk(nil); n != nil {
		t.Errorf("Walk(nil) = %+v, want nil", n)
	}
}

// TestReadBoundsFromXYWH covers the secondary bounds-extraction path
// (X/Y/Width/Height accessors) when a widget doesn't return all four
// values from one Bounds() method.
type duckRect struct {
	x, y, w, h float64
}

func (r *duckRect) X() float64      { return r.x }
func (r *duckRect) Y() float64      { return r.y }
func (r *duckRect) Width() float64  { return r.w }
func (r *duckRect) Height() float64 { return r.h }

func TestReadBoundsFromXYWHAccessors(t *testing.T) {
	r := &duckRect{x: 10, y: 20, w: 100, h: 30}
	n := Walk(r)
	if n.X != 10 || n.Y != 20 || n.W != 100 || n.H != 30 {
		t.Errorf("bounds = (%v, %v, %v, %v); want (10, 20, 100, 30)", n.X, n.Y, n.W, n.H)
	}
}
