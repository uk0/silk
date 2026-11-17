package gui

// Cancellation is not a synthetic release: it must never click a button,
// commit a preview, or move a caret/value to a made-up release coordinate.
func (b *Button) OnPointerCancel() {
	b.pushed = false
	b.PopCapture()
	b.Update()
}

func (b *CheckBox) OnPointerCancel() {
	b.pushed = false
	b.PopCapture()
	b.Update()
}

func (b *ToggleSwitch) OnPointerCancel() {
	b.pushed = false
	b.PopCapture()
	b.Update()
}

func (s *Slider) OnPointerCancel() {
	s.dragging = false
	s.PopCapture()
	s.Update()
}

func (s *ScrollBar) OnPointerCancel() {
	s.timer.Stop()
	s.pushed, s.part, s.tc, s.mouseOffset = false, 0, 0, 0
	s.PopCapture()
	s.Update()
}

func (s *Splitter) OnPointerCancel() {
	active := s.dragging >= 0 || s.hoverHandle >= 0
	s.dragging, s.hoverHandle, s.lastClickHandle = -1, -1, -1
	s.PopCapture()
	if active {
		SetOverrideCursor(nil)
	}
	s.Update()
}

func (t *Table) OnPointerCancel() {
	active := t.resizeCol >= 0 || t.hoverBoundary >= 0
	t.resizeCol, t.hoverBoundary, t.lastClickRow, t.lastClickCol = -1, -1, -1, -1
	t.PopCapture()
	if active {
		SetOverrideCursor(nil)
	}
	t.Update()
}

func (e *Edit) OnPointerCancel() {
	e.mouseDown, e.clickSelection = false, false
	e.PopCapture()
	e.Update()
}

func (e *CodeEditor) OnPointerCancel() {
	e.mouseDown, e.minimapDragging = false, false
	e.PopCapture()
	e.Update()
}

func (c *GanttChart) OnPointerCancel() {
	c.CancelInteraction()
	c.PopCapture()
}
