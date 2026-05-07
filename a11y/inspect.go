package a11y

import "strconv"

// inspect.go: duck-typed reading of well-known method names so widgets
// that don't implement the Accessible interface still produce a
// useful Node. The 62-widget catalog already exposes these methods
// (Text/Title/IsChecked/etc.); this file is the only thing keeping
// the per-widget patch from spanning every file in gui/.

// readName returns the widget's primary user-facing label. Priority
// (matches QAccessible's name resolution order):
//
//   1. AccessibleName() if Accessible
//   2. Text() string — Buttons, Labels, Edits
//   3. Title() string — GroupBox, Card, Dialog, Form
//   4. Caption() string — older widget convention
//   5. Empty string
func readName(w interface{}) string {
	if a, ok := w.(Accessible); ok {
		return a.AccessibleName()
	}
	if t, ok := w.(interface{ Text() string }); ok {
		return t.Text()
	}
	if t, ok := w.(interface{ Title() string }); ok {
		return t.Title()
	}
	if t, ok := w.(interface{ Caption() string }); ok {
		return t.Caption()
	}
	return ""
}

// readDescription returns the longer help text, when the widget
// implements AccessibleDescription or one of the conventional fallbacks.
func readDescription(w interface{}) string {
	if a, ok := w.(AccessibleDescription); ok {
		return a.AccessibleDescription()
	}
	if t, ok := w.(interface{ Tooltip() string }); ok {
		return t.Tooltip()
	}
	if t, ok := w.(interface{ ToolTip() string }); ok {
		return t.ToolTip()
	}
	if t, ok := w.(interface{ HelpText() string }); ok {
		return t.HelpText()
	}
	return ""
}

// readValue extracts a human-readable value string from common widget
// shapes. Returns "" when the widget has no obvious value semantics
// (most labels, groups, separators).
//
//   - String() / Value() string — Edits, ComboBox text content
//   - Float64-Value() — Slider / SpinBox / NumberInput / ProgressBar
//   - Bool checked — CheckBox / RadioButton / ToggleSwitch
func readValue(w interface{}) string {
	if a, ok := w.(AccessibleValue); ok {
		return a.AccessibleValue()
	}
	if t, ok := w.(interface{ Value() string }); ok {
		return t.Value()
	}
	if t, ok := w.(interface{ Value() float64 }); ok {
		return formatFloat(t.Value())
	}
	if t, ok := w.(interface{ Value() int }); ok {
		return formatInt(t.Value())
	}
	if t, ok := w.(interface{ String() string }); ok {
		return t.String()
	}
	return ""
}

// readState collects a State bitmask from the widget's bool methods.
// Explicit AccessibleState() takes precedence; otherwise we read
// IsEnabled / IsVisible / IsChecked / HasFocus and translate.
func readState(w interface{}) State {
	if a, ok := w.(AccessibleState); ok {
		return a.AccessibleState()
	}
	var s State
	// Some widgets expose IsEnabled, others Enabled. Cover both.
	if t, ok := w.(interface{ IsEnabled() bool }); ok {
		if !t.IsEnabled() {
			s |= StateDisabled
		}
	} else if t, ok := w.(interface{ Enabled() bool }); ok {
		if !t.Enabled() {
			s |= StateDisabled
		}
	}
	if t, ok := w.(interface{ IsVisible() bool }); ok {
		if !t.IsVisible() {
			s |= StateHidden
		}
	}
	if t, ok := w.(interface{ HasFocus() bool }); ok {
		if t.HasFocus() {
			s |= StateFocused
		}
	}
	if t, ok := w.(interface{ IsChecked() bool }); ok {
		if t.IsChecked() {
			s |= StateChecked
		}
	} else if t, ok := w.(interface{ Checked() bool }); ok {
		if t.Checked() {
			s |= StateChecked
		}
	}
	if t, ok := w.(interface{ IsReadOnly() bool }); ok {
		if t.IsReadOnly() {
			s |= StateReadOnly
		}
	}
	if t, ok := w.(interface{ IsExpanded() bool }); ok {
		if t.IsExpanded() {
			s |= StateExpanded
		}
	}
	return s
}

// readBounds returns the widget's logical-coordinate rect (x, y, w, h).
// Most gui widgets implement Bounds() (x, y, w, h float64); falling
// back to individual X/Y/Width/Height keeps the helper useful for
// custom widget shapes.
func readBounds(w interface{}) (x, y, ww, hh float64) {
	if t, ok := w.(interface {
		Bounds() (float64, float64, float64, float64)
	}); ok {
		return t.Bounds()
	}
	if t, ok := w.(interface {
		X() float64
		Y() float64
		Width() float64
		Height() float64
	}); ok {
		return t.X(), t.Y(), t.Width(), t.Height()
	}
	return 0, 0, 0, 0
}

// formatFloat formats a float for screen-reader announcement. Exact
// integers print without a trailing ".0"; fractions use 6 significant
// digits which is plenty for sliders and progress fractions.
func formatFloat(v float64) string {
	if v != v {
		return "NaN"
	}
	if iv := int64(v); float64(iv) == v && iv > -1<<53 && iv < 1<<53 {
		return strconv.FormatInt(iv, 10)
	}
	return strconv.FormatFloat(v, 'g', 6, 64)
}

func formatInt(v int) string {
	return strconv.Itoa(v)
}
