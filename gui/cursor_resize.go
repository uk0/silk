package gui

// HorizontalResizeCursor returns the platform cursor for an inline horizontal
// resize affordance. A custom widget can return it from its Cursor method.
func HorizontalResizeCursor() *Cursor { return cursorSizeWE }
