package gui

import "fmt"

// Binding represents a connection between a data source and a widget property.
// It holds a value and notifies registered watchers when the value changes.
type Binding struct {
	value    interface{}
	watchers []func(interface{})
	updating bool // guard against recursive Set calls
}

// NewBinding creates a new data binding with an initial value.
func NewBinding(initial interface{}) *Binding {
	return &Binding{value: initial}
}

// Get returns the current value.
func (b *Binding) Get() interface{} { return b.value }

// GetString returns the value as string.
func (b *Binding) GetString() string {
	if s, ok := b.value.(string); ok {
		return s
	}
	return fmt.Sprint(b.value)
}

// GetFloat returns the value as float64.
func (b *Binding) GetFloat() float64 {
	switch v := b.value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case float32:
		return float64(v)
	}
	return 0
}

// GetBool returns the value as bool.
func (b *Binding) GetBool() bool {
	if v, ok := b.value.(bool); ok {
		return v
	}
	return false
}

// GetInt returns the value as int.
func (b *Binding) GetInt() int {
	switch v := b.value.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case float32:
		return int(v)
	}
	return 0
}

// Set updates the value and notifies all watchers.
// It guards against recursive calls caused by two-way bindings.
func (b *Binding) Set(v interface{}) {
	if b.updating {
		return
	}
	if b.value == v {
		return
	}
	b.updating = true
	b.value = v
	for _, fn := range b.watchers {
		fn(v)
	}
	b.updating = false
}

// Watch adds a callback that fires when the value changes.
func (b *Binding) Watch(fn func(interface{})) {
	b.watchers = append(b.watchers, fn)
}

// BindLabel connects a binding to a Label widget (one-way: binding -> label).
func BindLabel(label *Label, binding *Binding) {
	binding.Watch(func(v interface{}) {
		label.SetText(fmt.Sprint(v))
	})
	label.SetText(fmt.Sprint(binding.Get()))
}

// BindEdit connects a binding to an Edit widget (two-way).
func BindEdit(edit *Edit, binding *Binding) {
	// Binding -> Edit
	binding.Watch(func(v interface{}) {
		edit.SetText(fmt.Sprint(v))
	})
	edit.SetText(fmt.Sprint(binding.Get()))

	// Edit -> Binding (on text change)
	edit.SigTextChanged(func(_ interface{}, text string) {
		binding.Set(text)
	})
}

// BindProgressBar connects a binding to a ProgressBar (one-way: binding -> progress bar).
func BindProgressBar(pb *ProgressBar, binding *Binding) {
	binding.Watch(func(v interface{}) {
		pb.SetValue(binding.GetFloat())
	})
	pb.SetValue(binding.GetFloat())
}

// BindSlider connects a binding to a Slider (two-way).
func BindSlider(slider *Slider, binding *Binding) {
	binding.Watch(func(v interface{}) {
		slider.SetValue(binding.GetFloat())
	})
	slider.SetValue(binding.GetFloat())

	slider.SetValueChangedCallback(func(_ interface{}, val float64) {
		binding.Set(val)
	})
}

// BindCheckBox connects a binding to a CheckBox (two-way).
func BindCheckBox(cb *CheckBox, binding *Binding) {
	binding.Watch(func(v interface{}) {
		cb.SetChecked(binding.GetBool())
	})
	cb.SetChecked(binding.GetBool())

	cb.SigCheck(func(checked bool) {
		binding.Set(checked)
	})
}

// BindSpinBox connects a binding to a SpinBox (two-way).
func BindSpinBox(sp *SpinBox, binding *Binding) {
	binding.Watch(func(v interface{}) {
		sp.SetValue(binding.GetInt())
	})
	sp.SetValue(binding.GetInt())

	sp.SetValueChangedCallback(func(_ interface{}, val int) {
		binding.Set(val)
	})
}
