// Package a11y is Silk's accessibility surface — the equivalent of
// Qt's QAccessible. Widgets advertise their semantic role, name, value
// and state so screen-readers, automated test harnesses, and other
// tooling can enumerate the UI without poking at private fields.
//
// Two participation modes:
//
//  1. Explicit: widgets implement the Accessible interface (and
//     optionally AccessibleDescription / AccessibleValue /
//     AccessibleState) for full control over the reported metadata.
//
//  2. Duck-typed: widgets that don't implement Accessible still
//     produce a useful Node via DescribeViaInterfaces, which inspects
//     well-known method names (Text / Title / IsChecked / Value /
//     IsEnabled / IsVisible / HasFocus / Bounds) on the widget. This
//     keeps the 62-widget catalog covered without modifying every
//     widget file at once.
//
// Walk(root) delivers a flat *Node tree rooted at the given widget.
// Hosts marshal the tree to whatever sink they want — a screen-reader
// bridge, an integration-test snapshot, or a debug overlay. The
// package itself talks to no OS API; platform-specific bridges
// (VoiceOver / NVDA / AT-SPI) belong in separate packages.
package a11y

// Role identifies the semantic kind of a UI element. Values map roughly
// to ARIA roles and QAccessible::Role so OS-specific bridges have a
// clean translation table.
type Role int

const (
	// RoleUnknown is the default for widgets that haven't declared a role
	// and don't match any inference heuristic.
	RoleUnknown Role = iota

	RoleApplication
	RoleWindow
	RoleDialog
	RoleGroup
	RoleSeparator
	RoleStatusBar
	RoleToolBar
	RoleMenuBar
	RoleMenu
	RoleMenuItem

	RoleButton
	RoleCheckBox
	RoleRadioButton
	RoleSwitch
	RoleEdit
	RoleSearchBox
	RoleSpinBox
	RoleSlider
	RoleProgressBar
	RoleComboBox
	RoleLabel
	RoleLink
	RoleImage

	RoleList
	RoleListItem
	RoleTree
	RoleTreeItem
	RoleTable
	RoleCell
	RoleTab
	RoleTabPanel

	RoleScrollBar
	RoleSplitter
	RoleCard
	RoleAccordion
	RoleNotification
	RoleTooltip
	RoleBadge
)

// String returns a stable human-readable name. Used by tests and debug
// dumps; bridges should switch on Role rather than parse the string.
func (r Role) String() string {
	switch r {
	case RoleApplication:
		return "Application"
	case RoleWindow:
		return "Window"
	case RoleDialog:
		return "Dialog"
	case RoleGroup:
		return "Group"
	case RoleSeparator:
		return "Separator"
	case RoleStatusBar:
		return "StatusBar"
	case RoleToolBar:
		return "ToolBar"
	case RoleMenuBar:
		return "MenuBar"
	case RoleMenu:
		return "Menu"
	case RoleMenuItem:
		return "MenuItem"
	case RoleButton:
		return "Button"
	case RoleCheckBox:
		return "CheckBox"
	case RoleRadioButton:
		return "RadioButton"
	case RoleSwitch:
		return "Switch"
	case RoleEdit:
		return "Edit"
	case RoleSearchBox:
		return "SearchBox"
	case RoleSpinBox:
		return "SpinBox"
	case RoleSlider:
		return "Slider"
	case RoleProgressBar:
		return "ProgressBar"
	case RoleComboBox:
		return "ComboBox"
	case RoleLabel:
		return "Label"
	case RoleLink:
		return "Link"
	case RoleImage:
		return "Image"
	case RoleList:
		return "List"
	case RoleListItem:
		return "ListItem"
	case RoleTree:
		return "Tree"
	case RoleTreeItem:
		return "TreeItem"
	case RoleTable:
		return "Table"
	case RoleCell:
		return "Cell"
	case RoleTab:
		return "Tab"
	case RoleTabPanel:
		return "TabPanel"
	case RoleScrollBar:
		return "ScrollBar"
	case RoleSplitter:
		return "Splitter"
	case RoleCard:
		return "Card"
	case RoleAccordion:
		return "Accordion"
	case RoleNotification:
		return "Notification"
	case RoleTooltip:
		return "Tooltip"
	case RoleBadge:
		return "Badge"
	}
	return "Unknown"
}

// State is a bitmask of UI state flags. Multiple flags can be set
// simultaneously (e.g. a focused-but-disabled control).
type State uint32

const (
	StateFocused State = 1 << iota
	StateChecked
	StateDisabled
	StateHidden
	StateReadOnly
	StatePressed
	StateExpanded
	StateSelected
	StateRequired
	StateInvalid
)

// Has reports whether all of mask is set in s. Bitwise convenience —
// reads better than s & mask == mask at every call site.
func (s State) Has(mask State) bool { return s&mask == mask }

// With returns s with mask added. Useful in builder-style state
// composition.
func (s State) With(mask State) State { return s | mask }

// Without returns s with mask cleared.
func (s State) Without(mask State) State { return s &^ mask }

// Node is a flat record describing one accessible element. Trees of
// Nodes mirror the widget hierarchy. All fields are public so tests
// and bridges can read directly; mutation is allowed but only on
// freshly-Walked trees (the package never holds onto Nodes after they
// leave Walk).
type Node struct {
	Role        Role
	Name        string
	Description string
	Value       string
	State       State
	X, Y, W, H  float64
	Children    []*Node
}

// Accessible is the explicit opt-in interface. Widgets that implement
// it advertise role + name directly; anything missing falls back to
// DescribeViaInterfaces.
type Accessible interface {
	AccessibleRole() Role
	AccessibleName() string
}

// AccessibleDescription is an optional refinement returning a longer
// help text. Most widgets leave this empty; tooltips and form fields
// with explanatory copy implement it.
type AccessibleDescription interface {
	AccessibleDescription() string
}

// AccessibleValue is implemented by widgets whose state has a
// human-readable value distinct from its name — slider position
// number, edit content, progress percent, etc.
type AccessibleValue interface {
	AccessibleValue() string
}

// AccessibleState is implemented by widgets that want to report a
// State bitmask explicitly. Widgets that don't implement this still
// get state inferred from their bool methods (IsEnabled / IsVisible /
// HasFocus / IsChecked).
type AccessibleState interface {
	AccessibleState() State
}
