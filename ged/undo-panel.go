package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
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

// historyRow is one line of the 历史 panel. Row i stands for stack position i:
// row 0 is the state before any command ran, row i>0 the state just after
// cmds[i-1]. A row index is therefore the position a click has to reach, and
// the row for command i sits at index i+1.
type historyRow struct {
	// Text labels the row: the command's Text(), or the base row's label.
	Text string
	// Dimmed marks a row the stack has been moved back past — the command it
	// stands for is undone and still redoable.
	Dimmed bool
}

// historyRows renders stack as rows plus its current position. The stack is
// read whole on every call rather than tracked push by push: Push merges into
// the previous command when it can, and truncates the redo tail when it
// cannot, so neither the row count nor the labels follow from the pushes a
// panel happened to observe.
func historyRows(stack gui.IUndoStack) (rows []historyRow, current int) {
	if stack == nil {
		return nil, 0
	}
	count := stack.Count()
	current = stack.Current()
	rows = make([]historyRow, 0, count+1)
	rows = append(rows, historyRow{Text: "初始状态"})
	for i := 0; i < count; i++ {
		text := stack.Command(i).Text()
		if text == "" {
			text = "(未命名操作)"
		}
		rows = append(rows, historyRow{Text: text, Dimmed: i >= current})
	}
	return rows, current
}

// sameHistoryRows reports whether two row lists would draw identically.
func sameHistoryRows(a, b []historyRow) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// UndoPanel is a tool panel that displays the undo/redo history as a
// scrollable list. Clicking an entry undoes or redoes to that point.
// Commands before the current position are shown in normal text; commands
// at or after the current position (redo-able) are shown in gray/italic.
type UndoPanel struct {
	gui.Widget
	scene     *GedScene
	rows      []historyRow
	current   int // current position in the undo stack, and the marked row
	hoverIdx  int
	scrollY   float64
	rowHeight float64
	// Timer that re-reads the stack; see syncFromStack.
	pollTimer gui.Timer
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
	this.pollTimer.Start(200, this.syncFromStack)
}

// SetScene binds the panel to a GedScene's undo stack.
func (this *UndoPanel) SetScene(scene *GedScene) {
	this.scene = scene
	this.Rebuild()
}

// Scene reports the scene whose history the panel is showing.
func (this *UndoPanel) Scene() *GedScene {
	return this.scene
}

// Rebuild reads the undo stack and repopulates the row list.
func (this *UndoPanel) Rebuild() {
	if this.scene == nil {
		this.rows, this.current = nil, 0
	} else {
		this.rows, this.current = historyRows(this.scene.UndoStack())
	}
	this.Self().Update()
}

// syncFromStack re-reads the stack on a timer and repaints only when the rows
// actually moved. Nothing announces an undo-stack change — every tool, menu
// item and shortcut in the designer pushes commands straight onto the scene's
// stack — and the two signals the scene does raise fire for attach/detach
// only, so a move, a resize or a property edit would leave the list stale.
func (this *UndoPanel) syncFromStack() {
	if this.scene == nil {
		return
	}
	rows, current := historyRows(this.scene.UndoStack())
	if current == this.current && sameHistoryRows(rows, this.rows) {
		return
	}
	this.rows, this.current = rows, current
	this.Self().Update()
}

// GoTo moves the bound stack to position pos, walking there one command at a
// time through Undo/Redo. Every command still runs its own Undo/Redo in order;
// re-seating the position any other way would skip the work the commands do.
//
// Walking the stack pushes nothing, so the jump has to report the change itself
// — the same reason GedView.Undo does. A click here can rewind a dozen edits;
// without the report every listener keeps rendering the design they replaced.
func (this *UndoPanel) GoTo(pos int) {
	if this.scene == nil {
		return
	}
	stack := this.scene.UndoStack()
	if stack == nil {
		return
	}
	if pos < 0 {
		pos = 0
	}
	if pos > stack.Count() {
		pos = stack.Count()
	}
	start := stack.Current()
	for stack.Current() > pos && stack.CanUndo() {
		stack.Undo()
	}
	for stack.Current() < pos && stack.CanRedo() {
		stack.Redo()
	}
	if stack.Current() != start {
		this.scene.NotifyDesignChanged()
	}
	this.Rebuild()
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
	g.DrawText1(8, headerH-5, "历史")

	normalFont := paint.NewFont(t.Font.Family(), 12, false, false)
	boldFont := paint.NewFont(t.Font.Family(), 12, true, false)
	italicFont := paint.NewFont(t.Font.Family(), 12, false, true)
	rh := this.rowHeight
	startY := headerH - this.scrollY

	for i, row := range this.rows {
		rowY := startY + float64(i)*rh
		if rowY+rh < headerH || rowY > h {
			continue
		}

		isCurrent := i == this.current

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
		} else if row.Dimmed {
			// Future/redo-able: gray italic
			g.SetBrush1(paint.Color{160, 160, 170, 255})
			g.SetFont(italicFont)
		} else {
			g.SetBrush1(t.TextColor)
			g.SetFont(normalFont)
		}
		g.DrawText1(8, textY, row.Text)

		// Bottom separator
		if !isCurrent {
			g.SetPen1(paint.Color{230, 230, 235, 100}, 0.5)
			g.MoveTo(0, rowY+rh)
			g.LineTo(w, rowY+rh)
			g.Stroke()
		}
	}
}

// rowAtY maps a y coordinate to a row index, or -1 when it hits no row.
func (this *UndoPanel) rowAtY(y float64) int {
	headerH := 22.0
	if y < headerH {
		return -1
	}
	idx := int(math.Floor((y - headerH + this.scrollY) / this.rowHeight))
	if idx < 0 || idx >= len(this.rows) {
		return -1
	}
	return idx
}

// OnLeftDown handles click to undo/redo to a specific point.
func (this *UndoPanel) OnLeftDown(x, y float64) {
	idx := this.rowAtY(y)
	if idx < 0 {
		return
	}
	this.GoTo(idx)
}

// OnMouseMove handles hover highlighting.
func (this *UndoPanel) OnMouseMove(x, y float64) {
	idx := this.rowAtY(y)
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
	maxScroll := float64(len(this.rows))*this.rowHeight - (this.Height() - 22)
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
