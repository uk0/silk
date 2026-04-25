package ged

import (
	"silk/core"
	"silk/gui"
	"silk/paint"
)

// DesignerMode represents a top-level mode of the Silk Designer,
// similar to Qt Creator's mode selector on the left sidebar.
type DesignerMode int

const (
	ModeDesign DesignerMode = iota // UI Design mode (widget palette + canvas + properties)
	ModeEdit                       // IDE/Code mode (file explorer + multi-tab editor)
	ModeDebug                      // Debug mode (reserved for future use)
)

func init() {
	core.RegisterFactory("ged.ModeSelector", gui.TypeOf(ModeSelector{}))
}

// modeEntry describes one selectable mode in the sidebar.
type modeEntry struct {
	mode     DesignerMode
	name     string
	icon     string
	shortcut string
}

// ModeSelector is a narrow vertical sidebar on the far left of the frame.
// It displays mode buttons (icon + label) and lets the user switch between
// design mode and edit/code mode, similar to Qt Creator's mode selector.
type ModeSelector struct {
	gui.Widget
	currentMode   DesignerMode
	hoverMode     int // index into modes, -1 = none
	cbModeChanged func(DesignerMode)
	modes         []modeEntry
}

// NewModeSelector creates a mode selector widget pre-populated with the
// standard designer modes.
func NewModeSelector() *ModeSelector {
	p := new(ModeSelector)
	p.Init(p)
	return p
}

func (this *ModeSelector) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.hoverMode = -1
	this.modes = []modeEntry{
		{ModeDesign, "设计", "design", "Ctrl+1"},
		{ModeEdit, "代码", "edit", "Ctrl+2"},
	}
}

// SetMode switches to the given mode and fires the change callback.
func (this *ModeSelector) SetMode(mode DesignerMode) {
	if this.currentMode == mode {
		return
	}
	this.currentMode = mode
	if this.cbModeChanged != nil {
		this.cbModeChanged(mode)
	}
	this.Self().Update()
}

// CurrentMode returns the active mode.
func (this *ModeSelector) CurrentMode() DesignerMode {
	return this.currentMode
}

// SigModeChanged registers a callback invoked whenever the mode changes.
func (this *ModeSelector) SigModeChanged(fn func(DesignerMode)) {
	this.cbModeChanged = fn
}

// SizeHints returns a fixed width of 60 pixels, stretching vertically.
func (this *ModeSelector) SizeHints() gui.SizeHints {
	return gui.SizeHints{
		Width:    60,
		MinWidth: 60,
	}
}

// Draw renders the mode selector as a dark vertical bar with icon+text buttons.
func (this *ModeSelector) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark sidebar background
	g.SetBrush1(paint.Color{R: 40, G: 42, B: 48, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	btnH := 72.0 // height per mode button
	nameFont := paint.NewFont(gui.Theme().Font.Family(), 12, false, false)
	shortcutFont := paint.NewFont(gui.Theme().Font.Family(), 9, false, false)

	for i, entry := range this.modes {
		y := float64(i) * btnH
		isActive := entry.mode == this.currentMode
		isHover := i == this.hoverMode

		// Active: bright left border accent + lighter background
		if isActive {
			// Accent bar on the left edge
			g.SetBrush1(paint.Color{R: 66, G: 133, B: 244, A: 255})
			g.Rectangle(0, y, 3, btnH)
			g.Fill()

			// Lighter background for active button
			g.SetBrush1(paint.Color{R: 55, G: 58, B: 66, A: 255})
			g.Rectangle(3, y, w-3, btnH)
			g.Fill()
		} else if isHover {
			// Subtle hover highlight
			g.SetBrush1(paint.Color{R: 48, G: 50, B: 58, A: 255})
			g.Rectangle(0, y, w, btnH)
			g.Fill()
		}

		// Mode name (centered)
		if isActive {
			g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
		} else {
			g.SetBrush1(paint.Color{R: 160, G: 165, B: 175, A: 255})
		}
		g.SetFont(nameFont)
		nfe := nameFont.FontExtents()
		nameY := y + 24 + nfe.Ascent
		// Approximate centering: draw at x = (w - estimatedTextWidth) / 2
		// Since we lack precise text width measurement, use a fixed offset
		g.DrawText1(w/2-12, nameY, entry.name)

		// Shortcut text in small gray font
		g.SetBrush1(paint.Color{R: 100, G: 105, B: 115, A: 255})
		g.SetFont(shortcutFont)
		sfe := shortcutFont.FontExtents()
		shortcutY := nameY + nfe.Descent + 4 + sfe.Ascent
		g.DrawText1(w/2-16, shortcutY, entry.shortcut)
	}

	// Bottom separator line
	g.SetPen1(paint.Color{R: 30, G: 32, B: 38, A: 255}, 1)
	g.Line(w-1, 0, w-1, h)
	g.Stroke()
}

// OnLeftDown handles mouse clicks to switch modes.
func (this *ModeSelector) OnLeftDown(x, y float64) {
	btnH := 72.0
	idx := int(y / btnH)
	if idx >= 0 && idx < len(this.modes) {
		this.SetMode(this.modes[idx].mode)
	}
}

// OnMouseMove tracks hover state for visual feedback.
func (this *ModeSelector) OnMouseMove(x, y float64) {
	btnH := 72.0
	idx := int(y / btnH)
	if idx < 0 || idx >= len(this.modes) {
		idx = -1
	}
	if idx != this.hoverMode {
		this.hoverMode = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears hover state.
func (this *ModeSelector) OnMouseLeave() {
	if this.hoverMode != -1 {
		this.hoverMode = -1
		this.Self().Update()
	}
}
