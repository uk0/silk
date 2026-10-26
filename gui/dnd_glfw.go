//go:build !windows

package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var privateDndData []interface{}
var dndActive bool // true during DoDragDrop loop, prevents re-entrant mouse handling
var dndCancelRequested bool

const dragIdleWait = time.Second / 60

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

	if dndActive {
		return DndIgnore
	}
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

	ctx := &dndContext{
		pa:      availableActions,
		action:  DndIgnore,
		from:    from,
		formats: formats,
		data:    dataMap,
	}

	// Set override cursor for drag visual feedback
	cursors := GenerateDropCursors(content)
	previousCursor := SetOverrideCursor(cursors[0])
	defer func() { SetOverrideCursor(previousCursor); ReleaseDropCursors(cursors); privateDndData = nil }()

	// Interactive drag loop: poll events and track mouse until button released
	result := DndIgnore
	var lastDropWidget IWidget
	defer func() { dndDragLeave(lastDropWidget) }()
	// Cursor position the drag was last offered to the widget tree at. OLE calls
	// DragOver off real mouse-move messages, and dndDragOver reports no holder
	// for a target that refuses
	// the drag. Only revisit the widget tree when the pointer moves, not on
	// every idle wakeup while a refusing target is under a stationary cursor.
	walked := false
	var walkX, walkY float64
	dragging := true
	dndActive, dndCancelRequested = true, false
	defer func() {
		dndActive, dndCancelRequested = false, false
		forgetMouseAfterModalLoop(this)
	}()

	for dragging {
		glfw.PollEvents()
		serviceUIFrame()
		if dndCancelRequested || shouldQuit || this.glfwWin == nil {
			break
		}

		// Use the latest delivered pointer event, including during this loop.
		mx, my := MousePosition()

		// Find the widget under the cursor
		targetWidget := FindWidgetGlobal(mx, my)
		wx, wy := 0.0, 0.0
		if targetWidget != nil {
			wx, wy = targetWidget.MapFromGlobal(mx, my)
		}
		if !walked || mx != walkX || my != walkY {
			lastDropWidget = dndDragOver(targetWidget, lastDropWidget, wx, wy, ctx)
			walkX, walkY, walked = mx, my, true
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
		if !IsMouseLeftDown() {
			dragging = false
			// OLE only reaches IDropTarget::Drop when the last DragOver accepted;
			// gate the drop the same way so releasing over a target that refused
			// the drag does nothing here either.
			if ctx.Action() != DndIgnore {
				dndDrop(targetWidget, wx, wy, ctx)
				result = ctx.Action()
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

		// Native input/UI wakeups interrupt the wait immediately. Do not wake
		// ourselves 1000 times/second while a stationary drag is held.
		if dragging {
			glfw.WaitEventsTimeout(dragIdleWait.Seconds())
		}
	}

	return result
}

// The nested loop consumes button-up while ordinary mouse dispatch is suspended.
// Retire both armed and active capture, or the next move captures the old source
// and a subsequent toolbar click starts another drag instead of its own action.
func forgetMouseAfterModalLoop(win *Window) {
	captureStack = captureStack[:0]
	releaseCapture()
	setMouseHoverWidget(nil)
	lastMouseWidget = nil
	mouseMoving = false
	if win != nil {
		win.autoCaptured = false
		win.toCapture = false
	}
}
