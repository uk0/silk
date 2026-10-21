package gui

// SetDockTitle overrides a tool view's dock caption without altering its own
// content title or the shared factory registry. The caption follows redocking.
// An empty string restores the view's native title.
func (w *Widget) SetDockTitle(title string) {
	if w.dockTitle == title {
		return
	}
	w.dockTitle = title
	if d, ok := w.Parent().(*Dock); ok {
		d.Layout()
		d.Update()
	}
}

func (w *Widget) DockTitle() string { return w.dockTitle }
