package gui

// The current BeginPaint/GDI presentation path is opaque. Capability-based
// painting also supports compositors where a transparency request is rejected.
func (w *Window) hasAlphaSurface() bool { return false }
