package gui

// editorStatusSlots returns non-overlapping text origins. Position has priority,
// then wrap/language/encoding; counts only use space left over. Hidden fields
// have x=-1. Widths are measured font advances in logical coordinates.
func editorStatusSlots(width float64, widths [5]float64) (x [5]float64) {
	for i := range x {
		x[i] = -1
	}
	const margin, gap = 10.0, 16.0
	left, right := margin, width-margin
	if widths[0] > right-left {
		return
	}
	x[0] = left
	left += widths[0] + gap
	for _, i := range [...]int{4, 3, 2} {
		if right-widths[i] >= left {
			x[i] = right - widths[i]
			right = x[i] - gap
		}
	}
	if right-left >= widths[1] {
		x[1] = left
	}
	return
}

// Indentation is optional metadata: never displace the existing caret, counts,
// encoding, language or operation status to display it in a narrow pane.
func editorStatusSlotsWithIndent(width float64, widths [6]float64) (x [6]float64) {
	old := editorStatusSlots(width, [5]float64{widths[0], widths[1], widths[2], widths[3], widths[4]})
	copy(x[:5], old[:])
	x[5] = -1
	if x[0] < 0 {
		return
	}
	left, right := x[0]+widths[0]+16, width-10
	if x[1] >= 0 {
		left = x[1] + widths[1] + 16
	}
	for _, i := range [...]int{2, 3, 4} {
		if x[i] >= 0 {
			right = min(right, x[i]-16)
		}
	}
	if widths[5] > 0 && right-left >= widths[5] {
		x[5] = right - widths[5]
	}
	return
}
