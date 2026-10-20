package gui

import "github.com/uk0/silk/win32"

// GetMessage normally prefers posted messages over input, paint and timers.
// Continuation wakeups from a large gui.Post burst must not starve those lower
// priority classes. While UI work remains, reserve one turn per class, plus a
// normal GetMessage turn so WM_CLOSE/WM_END_MODAL/application messages progress.
// Without pending work we retain the ordinary blocking pump (no idle polling).
// https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-peekmessagew
var windowsMessageTurn uint32
var peekUIMessage = win32.PeekMessage
var getUIMessage = win32.GetMessageResult

func nextWindowsUIMessage(msg *win32.MSG) (bool, error) {
	uiTaskMu.Lock()
	pending := len(uiTasks) != 0
	uiTaskMu.Unlock()
	if pending {
		for range 4 {
			turn := windowsMessageTurn % 4
			windowsMessageTurn++
			if turn == 3 {
				break
			}
			first, last, flags := uint32(0), uint32(0), uint32(win32.PM_REMOVE)
			switch turn {
			case 0:
				flags |= 0x0407 << 16 // PM_QS_INPUT
			case 1:
				first, last = win32.WM_PAINT, win32.WM_PAINT
			case 2:
				first, last = win32.WM_TIMER, win32.WM_TIMER
			}
			// Explicit ranges also force generation of due WM_TIMER/WM_PAINT.
			// A QS_TIMER mask alone did NOT bypass an already-posted wake in Win10
			// runtime QA; that path returned no timer and fell back to GetMessage.
			if peekUIMessage(msg, 0, first, last, flags) {
				return msg.Message != win32.WM_QUIT, nil
			}
		}
	}
	return getUIMessage(msg, 0, 0, 0)
}
