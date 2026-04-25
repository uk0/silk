package ged

import (
	"silk/core"
	"silk/gui"
	"silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("ged.UndoPanel", gui.TypeOf(UndoPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.UndoPanel",
		Name: "历史",
		Icon: "edit-undo",
		Desc: "撤销/重做历史记录",
	})
}

// UndoPanel is a tool panel that displays the undo/redo history as a
// scrollable list. Clicking an entry undoes or redoes to that point.
// Commands before the current position are shown in normal text; commands
// at or after the current position (redo-able) are shown in gray/italic.
type UndoPanel struct {
	gui.Widget
	scene      *GedScene
	items      []string // command descriptions
	currentIdx int      // current position in the undo stack
	hoverIdx   int
	scrollY    float64
	rowHeight  float64
}

func NewUndoPanel() *UndoPanel {
	p := new(UndoPanel)
	p.Init(p)
	return p
}

func (this *UndoPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 24
	this.hoverIdx = -1
	this.currentIdx = 0
}

// SetScene binds the panel to a GedScene's undo stack.
func (this *UndoPanel) SetScene(scene *GedScene) {
	this.scene = scene
	this.Rebuild()
}

// Rebuild reads the undo stack and populates the items list.
func (this *UndoPanel) Rebuild() {
	this.items = nil
	this.currentIdx = 0
	if this.scene == nil {
		return
	}
	stack := this.scene.UndoStack()
	if stack == nil {
		return
	}
	count := stack.Count()
	this.currentIdx = stack.Current()

	for i := 0; i < count; i++ {
		cmd := stack.Command(i)
		text := cmd.Text()
		if text == "" {
			text = "(unknown)"
		}
		this.items = append(this.items, text)
	}
	this.Self().Update()
}

// Draw renders the undo history list.
func (this *UndoPanel) Draw(g paint.Painter) {
	w, h := this.Size()
	t := gui.Theme()

	// Background
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Header
	headerH := 22.0
	g.SetBrush1(paint.Color{235, 238, 245, 255})
	g.Rectangle(0, 0, w, headerH)
	g.Fill()
	g.SetPen1(paint.Color{200, 200, 210, 255}, 1)
	g.MoveTo(0, headerH)
	g.LineTo(w, headerH)
	g.Stroke()

	g.SetFont(paint.NewFont(t.Font.Family(), 12, true, false))
	g.SetBrush1(t.TextColor)
	g.DrawText1(8, headerH-5, "Undo History")

	// "Initial State" row at the top
	normalFont := paint.NewFont(t.Font.Family(), 12, false, false)
	boldFont := paint.NewFont(t.Font.Family(), 12, true, false)
	italicFont := paint.NewFont(t.Font.Family(), 12, false, true)
	rh := this.rowHeight
	startY := headerH - this.scrollY

	// Initial state row (index -1 means "before any command")
	initRowY := startY
	if this.currentIdx == 0 {
		// Current position is at initial state
		g.SetBrush1(paint.Color{51, 120, 215, 255})
		g.Rectangle(0, initRowY, w, rh)
		g.Fill()
		// Blue left bar
		g.SetBrush1(paint.Color{30, 90, 200, 255})
		g.Rectangle(0, initRowY, 3, rh)
		g.Fill()
		g.SetBrush1(paint.Color{255, 255, 255, 255})
		g.SetFont(boldFont)
	} else if this.hoverIdx == -1 {
		g.SetBrush1(paint.Color{230, 235, 245, 255})
		g.Rectangle(0, initRowY, w, rh)
		g.Fill()
		g.SetBrush1(t.TextColor)
		g.SetFont(normalFont)
	} else {
		g.SetBrush1(t.TextColor)
		g.SetFont(normalFont)
	}
	g.DrawText1(8, initRowY+rh-7, "Initial State")

	// Separator
	g.SetPen1(paint.Color{230, 230, 235, 100}, 0.5)
	g.MoveTo(0, initRowY+rh)
	g.LineTo(w, initRowY+rh)
	g.Stroke()

	// Command rows
	for i, text := range this.items {
		rowY := startY + float64(i+1)*rh // +1 because of initial state row
		if rowY+rh < headerH || rowY > h {
			continue
		}

		isDone := i < this.currentIdx
		isCurrent := i == this.currentIdx-1

		// Hover highlight
		if i == this.hoverIdx && !isCurrent {
			g.SetBrush1(paint.Color{230, 235, 245, 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}

		// Current position highlight
		if isCurrent {
			g.SetBrush1(paint.Color{51, 120, 215, 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
			// Blue left bar
			g.SetBrush1(paint.Color{30, 90, 200, 255})
			g.Rectangle(0, rowY, 3, rh)
			g.Fill()
		}

		// Text
		textY := rowY + rh - 7
		if isCurrent {
			g.SetBrush1(paint.Color{255, 255, 255, 255})
			g.SetFont(boldFont)
		} else if isDone {
			g.SetBrush1(t.TextColor)
			g.SetFont(normalFont)
		} else {
			// Future/redo-able: gray italic
			g.SetBrush1(paint.Color{160, 160, 170, 255})
			g.SetFont(italicFont)
		}
		g.DrawText1(8, textY, text)

		// Bottom separator
		if !isCurrent {
			g.SetPen1(paint.Color{230, 230, 235, 100}, 0.5)
			g.MoveTo(0, rowY+rh)
			g.LineTo(w, rowY+rh)
			g.Stroke()
		}
	}
}

// OnLeftDown handles click to undo/redo to a specific point.
func (this *UndoPanel) OnLeftDown(x, y float64) {
	if this.scene == nil {
		return
	}
	stack := this.scene.UndoStack()
	if stack == nil {
		return
	}

	headerH := 22.0
	if y < headerH {
		return
	}

	// Determine which row was clicked
	idx := int(math.Floor((y - headerH + this.scrollY) / this.rowHeight))
	// idx 0 = initial state, idx 1 = first command, etc.

	if idx < 0 {
		return
	}

	// Target position: clicking "Initial State" (idx=0) -> target=0
	// Clicking command i (idx=i+1) -> target=i+1
	var targetPos int
	if idx == 0 {
		targetPos = 0
	} else {
		cmdIdx := idx - 1
		if cmdIdx >= len(this.items) {
			return
		}
		targetPos = cmdIdx + 1
	}

	current := stack.Current()
	if targetPos < current {
		// Undo to target
		for stack.Current() > targetPos && stack.CanUndo() {
			stack.Undo()
		}
	} else if targetPos > current {
		// Redo to target
		for stack.Current() < targetPos && stack.CanRedo() {
			stack.Redo()
		}
	}

	this.Rebuild()
}

// OnMouseMove handles hover highlighting.
func (this *UndoPanel) OnMouseMove(x, y float64) {
	headerH := 22.0
	if y < headerH {
		if this.hoverIdx != -1 {
			this.hoverIdx = -1
			this.Self().Update()
		}
		return
	}
	idx := int(math.Floor((y-headerH+this.scrollY)/this.rowHeight)) - 1
	if idx < -1 || idx >= len(this.items) {
		idx = -2 // invalid
	}
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave resets hover state.
func (this *UndoPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel handles scrolling.
func (this *UndoPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3
	totalRows := float64(len(this.items) + 1) // +1 for initial state
	maxScroll := totalRows*this.rowHeight - (this.Height() - 22)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}
