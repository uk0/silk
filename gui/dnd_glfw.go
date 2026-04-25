//go:build !windows

package gui

import (
	"silk/core"
	"silk/paint"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var privateDndData []interface{}
var dndActive bool // true during DoDragDrop loop, prevents re-entrant mouse handling

type dndContext struct {
	pa      DndAction
	action  DndAction
	from    interface{}
	formats []string
	data    map[string]interface{}
}

func (this *dndContext) Formats() (formats []string) {
	formats = append(formats, this.formats...)
	if privateDndData != nil {
		formats = append(formats, "[]interface{}")
	}
	return
}

func (this *dndContext) HasFormat(format string) bool {
	if format == "[]interface{}" {
		return privateDndData != nil
	}
	for _, f := range this.formats {
		if f == format {
			return true
		}
	}
	return false
}

func (this *dndContext) Data(format string) (data interface{}) {
	if format == "[]interface{}" {
		return privateDndData
	}
	if this.data != nil {
		return this.data[format]
	}
	return nil
}

func (this *dndContext) PosibleActions() DndAction {
	return this.pa
}

func (this *dndContext) Action() DndAction {
	return this.action
}

func (this *dndContext) SetAction(act DndAction) {
	if this.pa&act == 0 {
		this.action = DndIgnore
		return
	}
	this.action = act
}

func (this *dndContext) From() interface{} {
	return this.from
}

// DoDragDrop implements an interactive drag-and-drop loop for GLFW.
// It polls GLFW events and tracks the mouse to deliver drag enter/move/leave/drop
// events to widgets that implement IOnDrop, matching the Windows COM-based behavior.
func (this *Window) DoDragDrop(from interface{},
	content paint.Pixmap,
	availableActions DndAction,
	data ...interface{}) DndAction {

	privateDndData = nil

	if len(data) == 0 {
		core.Warn("drag nothing")
		return DndIgnore
	}

	// Store data for in-process access
	var formats []string
	dataMap := make(map[string]interface{})

	for _, d := range data {
		switch d.(type) {
		case string:
			// Only store the first string to avoid overwriting earlier values
			if _, exists := dataMap["text/plain"]; !exists {
				formats = append(formats, "text/plain")
				dataMap["text/plain"] = d
			}
		default:
			privateDndData = append(privateDndData, d)
		}
	}

	if content == nil {
		content = paint.NewPixmap(32, 16)
		g := content.NewPainter()
		g.SetBrush1(paint.Color{0, 0, 0, 127})
		g.Rectangle(0, 0, 32, 16)
		g.Fill()
	}

	ctx := &dndContext{
		pa:      availableActions,
		action:  DndIgnore,
		from:    from,
		formats: formats,
		data:    dataMap,
	}

	// Set override cursor for drag visual feedback
	cursors := GenerateDropCursors(content)
	if len(cursors) > 0 {
		SetOverrideCursor(cursors[0])
	}

	// Interactive drag loop: poll events and track mouse until button released
	result := DndIgnore
	var lastDropWidget IWidget
	dragging := true
	dndActive = true
	defer func() { dndActive = false }()

	for dragging {
		glfw.PollEvents()

		// Get current global mouse position
		mx, my := MousePosition()

		// Find the widget under the cursor
		targetWidget := FindWidgetGlobal(mx, my)

		// Walk up the widget tree to find the nearest IOnDrop handler
		var dropTarget IWidget
		for w := targetWidget; w != nil; w = w.Parent() {
			if _, ok := w.(IOnDrop); ok {
				dropTarget = w
				break
			}
		}

		// Handle drag enter/move/leave transitions
		if dropTarget != lastDropWidget {
			// Left the previous drop target
			if lastDropWidget != nil {
				if il, ok := lastDropWidget.(IOnDragLeave); ok {
					il.OnDragLeave()
				}
			}
			// Entered a new drop target
			if dropTarget != nil {
				if id, ok := dropTarget.(IOnDrop); ok {
					ctx.SetAction(DndIgnore)
					wx, wy := dropTarget.MapFromGlobal(mx, my)
					id.OnDragEnter(wx, wy, ctx)
				}
			}
			lastDropWidget = dropTarget
		} else if dropTarget != nil {
			// Still over the same drop target - send move
			if id, ok := dropTarget.(IOnDrop); ok {
				wx, wy := dropTarget.MapFromGlobal(mx, my)
				id.OnDragMove(wx, wy, ctx)
			}
		}

		// Update cursor based on current action
		if len(cursors) >= 4 {
			switch ctx.Action() {
			case DndMove:
				SetOverrideCursor(cursors[1])
			case DndCopy:
				SetOverrideCursor(cursors[2])
			case DndLink:
				SetOverrideCursor(cursors[3])
			default:
				SetOverrideCursor(cursors[0])
			}
		}

		// Check if mouse button released = drop (only once!)
		if !IsMouseLeftDown() && dragging {
			dragging = false
			if dropTarget != nil && ctx.Action() != DndIgnore {
				if id, ok := dropTarget.(IOnDrop); ok {
					wx, wy := dropTarget.MapFromGlobal(mx, my)
					id.OnDrop(wx, wy, ctx)
					result = ctx.Action()
				}
			}
		}

		// Repaint dirty windows during drag to keep UI responsive
		for _, win := range winMap {
			if win.dirty && win.IsVisible() && win.wt != WtPopup {
				win.paint()
			}
		}
		for _, win := range winMap {
			if win.dirty && win.IsVisible() && win.wt == WtPopup {
				win.paint()
			}
		}

		glfw.PostEmptyEvent()
		time.Sleep(time.Millisecond)
	}

	// Clean up drag leave on the last target
	if lastDropWidget != nil {
		if il, ok := lastDropWidget.(IOnDragLeave); ok {
			il.OnDragLeave()
		}
	}

	SetOverrideCursor(nil)
	privateDndData = nil
	return result
}
