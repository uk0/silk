package gui

// The same track solver is used by SizeHintsForWidth and Layout. In particular,
// a tall wrapped cell must not be measured as one row and then receive an equal
// fraction of the grid's height that is smaller than that row's requirement.
func (g *GridLayout) ownsCell(cell GridCell) bool {
	return cell.widget != nil && cell.widget.Parent() == g.Self()
}

func (g *GridLayout) activeCell(cell GridCell) bool {
	return g.ownsCell(cell) && cell.widget.IsVisible()
}

func raiseGridSpan(tracks, fixed []float64, start, span int, gap, required float64) {
	end := min(len(tracks), start+span)
	if start < 0 || start >= end {
		return
	}
	used := float64(end-start-1) * gap
	flexible := 0
	for i := start; i < end; i++ {
		used += tracks[i]
		if i >= len(fixed) || fixed[i] <= 0 {
			flexible++
		}
	}
	if required > used && flexible > 0 {
		extra := (required - used) / float64(flexible)
		for i := start; i < end; i++ {
			if i >= len(fixed) || fixed[i] <= 0 {
				tracks[i] += extra
			}
		}
	}
}

func gridSpanSize(tracks []float64, start, span int, gap float64) float64 {
	end := min(len(tracks), start+span)
	if start < 0 || start >= end {
		return 0
	}
	width := float64(end-start-1) * gap
	for i := start; i < end; i++ {
		width += tracks[i]
	}
	return width
}

func (g *GridLayout) measureColumns(available float64, constrained bool, count int, buffer []float64) []float64 {
	columns := growF64(buffer, count)
	var fixedWidth float64
	flexible := 0
	for i := range columns {
		if i < len(g.colWidths) && g.colWidths[i] > 0 {
			columns[i] = g.colWidths[i]
			fixedWidth += columns[i]
		} else {
			flexible++
		}
	}
	if constrained {
		width := max(0, available-fixedWidth-float64(max(0, count-1))*g.spacing) / float64(max(1, flexible))
		for i := range columns {
			if columns[i] == 0 {
				columns[i] = width
			}
		}
	} else {
		for _, cell := range g.cells {
			if g.activeCell(cell) {
				hint := MeasureForWidth(cell.widget, 0)
				raiseGridSpan(columns, g.colWidths, cell.col, cell.colSpan, g.spacing, max(hint.Width, hint.MinWidth))
			}
		}
	}
	return columns
}

func (g *GridLayout) measureRows(columns []float64, count int, buffer []float64) ([]float64, SizePolicy) {
	rows := growF64(buffer, count)
	for i := range rows {
		if i < len(g.rowHeights) && g.rowHeights[i] > 0 {
			rows[i] = g.rowHeights[i]
		}
	}
	policy := GrowHorizontal | GrowVertical
	for _, cell := range g.cells {
		if !g.activeCell(cell) {
			continue
		}
		hint := MeasureForWidth(cell.widget, gridSpanSize(columns, cell.col, cell.colSpan, g.spacing))
		raiseGridSpan(rows, g.rowHeights, cell.row, cell.rowSpan, g.spacing, layoutHintHeight(hint, 0))
		policy |= hint.Policy & ShrinkHorizontal
	}
	return rows, policy
}

func (g *GridLayout) HeightForWidth(width float64) float64 {
	return g.SizeHintsForWidth(width).Height
}

func (g *GridLayout) SizeHintsForWidth(width float64) SizeHints {
	if hint, ok := inheritedNaturalHints(g, width); ok {
		return hint
	}
	return preserveInheritedHints(g, g.measureGridWidth(width))
}

func (g *GridLayout) measureGridWidth(width float64) SizeHints {
	if g.layoutDisabled {
		return SizeHints{Width: g.Width(), Height: g.Height()}
	}
	rows, columns := g.gridDimensions()
	if rows == 0 || columns == 0 {
		return SizeHints{}
	}
	scratch := acquireGridScratch()
	defer releaseGridScratch(scratch)
	_, _, available, _ := applyLayoutPadding(g.padding, layoutDimension(width), 0)
	scratch.colW = g.measureColumns(available, width > 0, columns, scratch.colW)
	var policy SizePolicy
	scratch.rowH, policy = g.measureRows(scratch.colW, rows, scratch.rowH)
	return SizeHints{
		Width:  gridSpanSize(scratch.colW, 0, columns, g.spacing) + g.padding.L + g.padding.R,
		Height: gridSpanSize(scratch.rowH, 0, rows, g.spacing) + g.padding.T + g.padding.B,
		Policy: policy,
	}
}
