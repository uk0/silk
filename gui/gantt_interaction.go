package gui

import "math"

type ganttHit struct {
	kind string
	row  int
}
type ganttGesture struct {
	kind, id                           string
	x, y, scrollX, scrollY, tableWidth float64
	model                              *GanttModel
	modelRevision                      uint64
	before                             []GanttTask
	task                               GanttTask
	preview                            *GanttModel
	lastDays                           int
	lastProgress                       float64
}

func (c *GanttChart) interactive() bool {
	for w := c.Self(); w != nil; w = w.Parent() {
		if !w.IsEnabled() || !w.IsVisible() {
			return false
		}
	}
	return true
}
func (c *GanttChart) hitTest(x, y float64) ganttHit {
	l := c.geometry()
	none := ganttHit{row: -1}
	if !ganttFinite(x) || !ganttFinite(y) || x < 0 || y < 0 || x >= l.w || y >= l.h {
		return none
	}
	if math.Abs(x-l.split) <= 3 && y < l.bottom {
		return ganttHit{"split", -1}
	}
	h, v := c.scrollThumbs(l)
	if h.Width > 0 && y >= l.bottom && y < l.bottom+10 && x >= l.split && x < l.w-10 {
		return ganttHit{"hscroll", -1}
	}
	if v.Height > 0 && x >= l.w-10 && y >= l.top && y < l.bottom {
		return ganttHit{"vscroll", -1}
	}
	if y < l.top || y >= l.bottom || x >= l.w-10 {
		return none
	}
	i := int((y - l.top + c.scrollY) / l.row)
	if i < 0 || i >= len(c.rows) {
		return none
	}
	r := c.rows[i]
	if x < l.split {
		if r.HasChildren {
			nameW := c.tableColumns(l)[0]
			indent := math.Min(float64(r.Depth)*14, math.Max(0, nameW-44))
			if x >= indent && x < indent+20 {
				return ganttHit{"collapse", i}
			}
		}
		return ganttHit{"select", i}
	}
	b := c.taskRect(r.Task, i, l)
	if !b.Contains(x, y) {
		return ganttHit{"select", i}
	}
	if c.readOnly {
		return ganttHit{"select", i}
	}
	if !r.HasChildren && !r.Task.Milestone {
		// Progress uses the distinct bottom strip, never the text hit region.
		if y >= b.Bottom()-5 {
			return ganttHit{"progress", i}
		}
		edge := math.Min(6, b.Width/3)
		if x < b.X+edge {
			return ganttHit{"start", i}
		}
		if x > b.Right()-edge {
			return ganttHit{"end", i}
		}
	}
	return ganttHit{"move", i}
}
func (c *GanttChart) OnLeftDown(x, y float64) {
	if !c.interactive() {
		return
	}
	c.CancelInteraction()
	c.SetFocus()
	if !c.interactive() || !c.HasFocus() {
		return
	}
	hit := c.hitTest(x, y)
	if hit.row >= 0 {
		id := c.rows[hit.row].Task.ID
		model, revision := c.model, c.model.Revision()
		c.SetSelectedTaskID(id)
		// Selection is a public SDK callback: the host may replace/collapse
		// tasks, open a dialog, or change geometry/read-only state. Never use
		// the old row index or start capture after that input became stale.
		if c.model != model || model.Revision() != revision || !c.interactive() || !c.HasFocus() || c.selectedID != id || c.hitTest(x, y) != hit {
			return
		}
	}
	if hit.kind == "collapse" {
		r := c.rows[hit.row]
		_ = c.SetCollapsed(r.Task.ID, !r.Task.Collapsed)
		return
	}
	if hit.kind == "" || hit.kind == "select" {
		return
	}
	d := &ganttGesture{kind: hit.kind, x: x, y: y, scrollX: c.scrollX, scrollY: c.scrollY, tableWidth: c.tableWidth, lastDays: 0, lastProgress: -1}
	if hit.row >= 0 {
		d.id = c.rows[hit.row].Task.ID
		d.task = c.rows[hit.row].Task
		d.model, d.modelRevision = c.model, c.model.Revision()
		d.before = c.Tasks()
	}
	c.drag = d
	c.PushCapture()
	if hit.kind == "progress" {
		c.previewGesture(x, y)
	}
	if hit.kind == "hscroll" || hit.kind == "vscroll" {
		c.scrollbarDown(x, y)
	}
	c.Update()
}
func (c *GanttChart) scrollbarDown(x, y float64) {
	l := c.geometry()
	h, v := c.scrollThumbs(l)
	if c.drag.kind == "hscroll" && !h.Contains(x, y) {
		if x < h.X {
			c.scrollX -= l.timelineW * .85
		} else {
			c.scrollX += l.timelineW * .85
		}
	}
	if c.drag.kind == "vscroll" && !v.Contains(x, y) {
		if y < v.Y {
			c.scrollY -= l.bodyH * .85
		} else {
			c.scrollY += l.bodyH * .85
		}
	}
	c.clampScroll()
	c.drag.scrollX = c.scrollX
	c.drag.scrollY = c.scrollY
}
func (c *GanttChart) OnMouseMove(x, y float64) {
	if c.drag != nil {
		if !c.HasCapture() || !c.interactive() {
			c.CancelInteraction()
			return
		}
		c.previewGesture(x, y)
		return
	}
	hit := c.hitTest(x, y)
	if hit != c.hover {
		c.hover = hit
		c.Update()
	}
}
func (c *GanttChart) previewGesture(x, y float64) {
	d := c.drag
	if d == nil || !ganttFinite(x) || !ganttFinite(y) {
		return
	}
	if d.model != nil && (c.model != d.model || c.model.Revision() != d.modelRevision) {
		c.CancelInteraction()
		return
	}
	days := int(ganttClamp(math.Round((x-d.x)/c.dayWidth), -3650000, 3650000))
	if (d.kind == "move" || d.kind == "start" || d.kind == "end") && days == d.lastDays {
		return
	}
	l := c.geometry()
	switch d.kind {
	case "split":
		c.tableWidth = ganttClamp(d.tableWidth+x-d.x, 100, math.Max(100, l.w-180))
		c.clampScroll()
		c.Update()
		return
	case "hscroll":
		h, _ := c.scrollThumbs(l)
		travel := l.timelineW - h.Width
		if travel > 0 {
			c.scrollX = d.scrollX + (x-d.x)*(c.contentWidth()-l.timelineW)/travel
		}
		c.clampScroll()
		c.Update()
		return
	case "vscroll":
		_, v := c.scrollThumbs(l)
		travel := l.bodyH - v.Height
		if travel > 0 {
			c.scrollY = d.scrollY + (y-d.y)*(float64(len(c.rows))*l.row-l.bodyH)/travel
		}
		c.clampScroll()
		c.Update()
		return
	}
	if c.readOnly {
		c.CancelInteraction()
		return
	}
	// Mouse motion within the same snapped day/percent is a no-op. Check
	// before copying and validating the entire task graph, not after it.
	t := d.task
	progress := 0.0
	if d.kind == "progress" {
		b := c.taskRect(t, c.rowIndex[d.id], l)
		progress = math.Round(ganttClamp((x-b.X)/b.Width, 0, 1)*100) / 100
		if progress == d.lastProgress {
			return
		}
	}
	// before is a snapshot of an already validated model. The edit makes
	// its own writable copy; do not validate and clone the baseline again.
	m := &GanttModel{tasks: d.before}
	var err error
	switch d.kind {
	case "move":
		err = m.MoveTask(d.id, days, c.cascadeDependencies)
	case "start":
		start := t.Start.AddDate(0, 0, days)
		if !start.Before(t.End) {
			start = t.End.AddDate(0, 0, -1)
		}
		err = m.ResizeTask(d.id, start, t.End, c.cascadeDependencies)
	case "end":
		end := t.End.AddDate(0, 0, days)
		if !end.After(t.Start) {
			end = t.Start.AddDate(0, 0, 1)
		}
		err = m.ResizeTask(d.id, t.Start, end, c.cascadeDependencies)
	case "progress":
		err = m.SetProgress(d.id, progress)
		d.lastProgress = progress
	}
	d.lastDays = days
	c.lastError = err
	if err == nil {
		d.preview = m
	} else {
		d.preview = nil
	}
	c.rebuildRows(false)
	c.Update()
}
func (c *GanttChart) OnLeftUp(x, y float64) {
	if c.drag == nil {
		return
	}
	if !c.HasCapture() || !c.interactive() || !ganttFinite(x) || !ganttFinite(y) {
		c.CancelInteraction()
		return
	}
	c.previewGesture(x, y)
	d := c.drag
	if d == nil {
		return
	}
	c.drag = nil
	c.PopCapture()
	// The preview already validated the proposed edit. Publish it as a
	// single history step only on the exact model revision it was based on.
	if d.preview != nil && c.model == d.model && c.model.Revision() == d.modelRevision &&
		c.model.commitPrepared(d.preview.tasks) {
		c.rebuildRows(false)
		c.commitNotification(d.before)
		return
	}
	c.rebuildRows(false)
	c.Update()
}

// CancelInteraction discards all uncommitted schedule changes and releases
// capture. Hosts may call it on native cancellation; Esc, hide, focus loss,
// disabling, replacing data, or a stolen capture also use this path.
func (c *GanttChart) CancelInteraction() {
	d := c.drag
	if d == nil {
		return
	}
	c.drag = nil
	c.PopCapture()
	if d.kind == "split" {
		c.tableWidth = d.tableWidth
	}
	c.lastError = nil
	c.rebuildRows(false)
	c.Update()
}
func (c *GanttChart) Cursor() *Cursor {
	if !c.interactive() {
		return DefaultCursor()
	}
	h := c.hover.kind
	if c.drag != nil {
		h = c.drag.kind
	}
	if c.readOnly && (h == "start" || h == "end" || h == "move" || h == "progress") {
		return DefaultCursor()
	}
	switch h {
	case "start", "end", "split":
		return cursorSizeWE
	case "move":
		return cursorSizeAll
	case "progress", "collapse":
		return cursorHand
	}
	return DefaultCursor()
}
func (c *GanttChart) OnMouseLeave() {
	if c.drag == nil {
		c.hover = ganttHit{row: -1}
		c.Update()
	}
}
func (c *GanttChart) OnMouseWheel(x, y, z float64) {
	c.wheel(x, z, IsKeyDown(KeyCtrl) || IsKeyDown(KeyLWin) || IsKeyDown(KeyRWin), IsKeyDown(KeyShift))
}
func (c *GanttChart) wheel(x, z float64, zoom, shift bool) {
	if !c.interactive() || !ganttFinite(z) || !ganttFinite(x) || z == 0 {
		return
	}
	c.CancelInteraction()
	l := c.geometry()
	if zoom {
		anchor := ganttClamp(x-l.split, 0, l.timelineW)
		day := (c.scrollX + anchor) / c.dayWidth
		c.dayWidth = ganttClamp(c.dayWidth*math.Exp(ganttClamp(z*.08, -4, 4)), 4, 120)
		c.scrollX = day*c.dayWidth - anchor
	} else if shift {
		c.scrollX -= z * c.dayWidth * 2
	} else {
		c.scrollY -= z * l.row * 3
	}
	c.clampScroll()
	c.Update()
}

// HandleShortcut prevents a focused embedded chart from undoing its host form.
// Even an empty local history consumes its own edit shortcut. Read-only charts
// deliberately make no editing shortcut claim.
func (c *GanttChart) HandleShortcut(key int, mods uint8) bool {
	if c.readOnly || !c.interactive() || mods&ModAction == 0 || mods&ModAlt != 0 {
		return false
	}
	switch key {
	case 'Z':
		if mods&ModShift != 0 {
			c.Redo()
		} else {
			c.Undo()
		}
		return true
	case 'Y':
		c.Redo()
		return true
	}
	return false
}
func (c *GanttChart) OnKeyDown(key int, repeat bool) {
	if !c.interactive() {
		return
	}
	if key == KeyEsc {
		c.CancelInteraction()
		return
	}
	if c.HandleShortcut(key, currentMods()) {
		return
	}
	l := c.geometry()
	i, ok := c.rowIndex[c.selectedID]
	switch key {
	case KeyUp, KeyDown:
		if len(c.rows) == 0 {
			return
		}
		if !ok {
			i = 0
		} else if key == KeyUp {
			i--
		} else {
			i++
		}
		i = max(0, min(len(c.rows)-1, i))
		c.SetSelectedTaskID(c.rows[i].Task.ID)
		top := float64(i) * l.row
		if top < c.scrollY {
			c.scrollY = top
		}
		if top+l.row > c.scrollY+l.bodyH {
			c.scrollY = top + l.row - l.bodyH
		}
	case KeyLeft, KeyRight:
		if ok && c.rows[i].HasChildren {
			_ = c.SetCollapsed(c.selectedID, key == KeyLeft)
		} else {
			delta := c.dayWidth * 3
			if key == KeyLeft {
				delta = -delta
			}
			c.scrollX += delta
		}
	case KeyPageUp:
		c.scrollY -= l.bodyH
	case KeyPageDown:
		c.scrollY += l.bodyH
	case KeyHome:
		c.scrollY = 0
	case KeyEnd:
		c.scrollY = float64(len(c.rows)) * l.row
	default:
		return
	}
	c.clampScroll()
	c.Update()
}
