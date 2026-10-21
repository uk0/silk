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
