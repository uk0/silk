package gui

type processArrangeAction struct {
	button     *Button
	minimum    int
	pipe, bend bool
}

func (v *ProcessEditor) initArrange(owner *Button) {
	menu := NewPopupMenu()
	menu.SetParent(owner)
	owner.SetSubPopup(menu)
	add := func(label string, minimum int, fn func() error) {
		b := menu.AddButton1(label, nil)
		b.Action().BindFunc0(func() { v.setError(fn()); v.refreshActions(); v.Diagram.SetFocus() })
		v.arrangeItems = append(v.arrangeItems, processArrangeAction{button: b, minimum: minimum})
	}
	b := menu.AddButton1("Select all  (Ctrl/Cmd+A)", nil)
	b.Action().BindFunc0(func() { v.Diagram.SelectAll(); v.Diagram.SetFocus() })
	v.arrangeItems = append(v.arrangeItems, processArrangeAction{button: b})
	menu.AddSeparator()
	for _, item := range []struct{ label, mode string }{{"Align left", "left"}, {"Align right", "right"}, {"Align top", "top"}, {"Align bottom", "bottom"}, {"Align horizontal centers", "horizontal-center"}, {"Align vertical centers", "vertical-center"}} {
		mode := item.mode
		add(item.label, 2, func() error { return v.Diagram.AlignSelection(mode) })
	}
	add("Equal horizontal gaps", 3, func() error { return v.Diagram.DistributeSelection(true) })
	add("Equal vertical gaps", 3, func() error { return v.Diagram.DistributeSelection(false) })
	menu.AddSeparator()
	add("Bring to front", 1, func() error { return v.Diagram.ReorderSelection(true) })
	add("Send to back", 1, func() error { return v.Diagram.ReorderSelection(false) })
	menu.AddSeparator()
	add("Add pipe waypoint", 0, v.Diagram.AddSelectedPipeBend)
	v.arrangeItems[len(v.arrangeItems)-1].pipe = true
	add("Remove selected waypoint", 0, v.Diagram.RemoveSelectedPipeBend)
	v.arrangeItems[len(v.arrangeItems)-1].bend = true
}
func (v *ProcessEditor) refreshActions() {
	if v.undoButton == nil {
		return
	}
	enabled := !v.running && !v.locked
	count := len(v.Diagram.SelectedNodes())
	v.undoButton.SetEnabled(enabled && v.Diagram.CanUndo())
	v.redoButton.SetEnabled(enabled && v.Diagram.CanRedo())
	v.duplicateButton.SetEnabled(enabled && count > 0)
	v.deleteButton.SetEnabled(enabled && (count > 0 || v.Diagram.SelectedLink() != ""))
	for _, a := range v.arrangeItems {
		ok := enabled && count >= a.minimum
		if a.pipe {
			ok = enabled && v.Diagram.SelectedLink() != ""
		}
		if a.bend {
			ok = enabled && v.Diagram.selectedBend >= 0
		}
		a.button.SetEnabled(ok)
	}
}
