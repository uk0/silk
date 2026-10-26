package prop

import (
	"reflect"

	"github.com/uk0/silk/gui"
)

// One continuous field edit keeps live model updates but has one scene undo
// record. This nonzero-sized identity is local to its editor; no timer, global
// counter, background work or native child window is needed.
type propertyEditSession struct{ closed bool }

// MergeWidth is the existing UndoStack protocol (including its legacy spelling).
// Keep the first command's per-object baseline and the last requested value.
// Push executes next.Redo after merging; that transfers its actual readback
// here, including any normalization performed by the widget's setter.
func (cmd *PropertyCommand) MergeWidth(next gui.ICommand) bool {
	other, ok := next.(*PropertyCommand)
	if !ok || !cmd.isUndo || cmd.session == nil || cmd.session.closed || cmd.session != other.session || cmd.prop != other.prop {
		return false
	}
	// A host may update the model without refreshing the inspector first. Do
	// not absorb that external value into the previous typing transaction.
	if !reflect.DeepEqual(cmd.after, cmd.prop.setValues()) {
		return false
	}
	cmd.val = other.val
	other.mergedInto = cmd
	return true
}

func (e *TextEdit) finishEdit() {
	if e.editSession != nil {
		e.editSession.closed = true
		e.editSession = nil
	}
}
func (e *TextEdit) canEditProperty() bool {
	if e.item == nil || e.item.retired || e.item.updatingControl || e.item.IsReadOnly() || e.IsReadOnly() {
		return false
	}
	for w := e.Self(); w != nil; w = w.Parent() {
		if !w.IsEnabled() || !w.IsVisible() {
			return false
		}
	}
	return true
}
func (e *TextEdit) setEditedValue(value interface{}) {
	if !e.canEditProperty() {
		return
	}
	if !e.item.IsMultiValue() && reflect.DeepEqual(value, e.item.GetValue()) {
		return
	}
	if e.editSession == nil || e.editSession.closed {
		e.editSession = &propertyEditSession{}
	}
	e.item.PropertyItem.setValue(value, e.editSession)
}
func (e *TextEdit) OnFocusChanged(next, previous gui.IWidget) {
	e.Widget.OnFocusChanged(next, previous)
	if previous != e.Self() || next == e.Self() {
		return
	}
	// An owned edit context menu is still part of the same field interaction.
	for w := next; w != nil; w = w.Parent() {
		if w == e.Self() {
			return
		}
	}
	if control, ok := e.Self().(IPropertyControl); ok {
		control.Deactivate()
	}
	e.finishEdit()
}
func (e *TextEdit) OnHide() { e.finishEdit() }
func (e *TextEdit) SetReadOnly(readOnly bool) {
	if readOnly {
		e.finishEdit()
	}
	e.Edit.SetReadOnly(readOnly)
}
func (e *TextEdit) SetEnabled(enabled bool) {
	if !enabled {
		e.finishEdit()
	}
	e.Widget.SetEnabled(enabled)
}
func (e *TextEdit) OnKeyUp(key int) {
	if key == gui.KeyUp || key == gui.KeyDown {
		e.finishEdit()
	}
}
