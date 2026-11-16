package gui

import "math"

// MeasureForWidth returns a widget's preferred size for an allocated width.
// It never installs probe bounds or creates a native window. Implementations
// may update measurement caches, but must not mutate authored/UI state.
// Widgets without width-dependent hints retain their existing SizeHints API.
// Nonpositive/nonfinite widths mean unconstrained measurement, as for a widget
// that has not been laid out yet. Custom invalid heights fall back to SizeHints.
func MeasureForWidth(widget IWidget, width float64) SizeHints {
	if widget == nil {
		return SizeHints{}
	}
	width = layoutDimension(width)
	if measured, ok := widget.(ISizeHintsForWidth); ok {
		hint := measured.SizeHintsForWidth(width)
		if validLayoutHeight(hint.Height) {
			return hint
		}
	}
	hint := widget.SizeHints()
	if measured, ok := widget.(IHeightForWidth); ok && width > 0 {
		if height := measured.HeightForWidth(width); validLayoutHeight(height) {
			hint.Height = height
		}
	}
	return hint
}

func validLayoutHeight(height float64) bool {
	return height >= 0 && !math.IsNaN(height) && !math.IsInf(height, 0)
}

// An embedded built-in's promoted width method must not erase a derived SDK
// widget's SizeHints override (stretch, policy and explicit constraints). For
// natural queries dispatch directly to that override, avoiding duplicate walks
// through deeply nested derived containers. Built-ins take the zero-cost path.
func inheritedNaturalHints(base IWidget, width float64) (SizeHints, bool) {
	self := base.NakedWidget().Self()
	if layoutDimension(width) == 0 && self != nil && self != base {
		return self.SizeHints(), true
	}
	return SizeHints{}, false
}

func preserveInheritedHints(base IWidget, measured SizeHints) SizeHints {
	if self := base.NakedWidget().Self(); self != nil && self != base {
		own := self.SizeHints()
		measured.Width = own.Width
		measured.Height = max(measured.Height, layoutDimension(own.Height))
		measured.MinWidth, measured.MaxWidth = own.MinWidth, own.MaxWidth
		measured.MinHeight, measured.MaxHeight = own.MinHeight, own.MaxHeight
		measured.Stretch, measured.Policy = own.Stretch, own.Policy
	}
	return measured
}

func layoutHintHeight(hint SizeHints, fallback float64) float64 {
	height := layoutDimension(hint.Height)
	if height <= 0 {
		height = layoutDimension(hint.MinHeight)
	}
	if height <= 0 {
		height = fallback
	}
	if hint.MinHeight > 0 {
		height = max(height, hint.MinHeight)
	}
	if hint.MaxHeight > 0 {
		height = min(height, hint.MaxHeight)
	}
	return height
}

func vboxChildWidth(hint SizeHints, available float64) float64 {
	if hint.Width > 0 && hint.Width < available && hint.Policy&(GrowHorizontal|ExpandHorizontal) == 0 {
		return hint.Width
	}
	return available
}

func vboxChildHints(child IWidget, available float64) SizeHints {
	hint := MeasureForWidth(child, available)
	if width := vboxChildWidth(hint, available); width != available {
		return MeasureForWidth(child, width)
	}
	return hint
}

// Share exactly the same horizontal allocation between measurement and layout.
// Only explicitly shrinkable children surrender space; minimum widths and fixed
// siblings remain authoritative. A wrap label opts in, unlike a normal label.
func resolveHBoxWidths(hints []SizeHints, available, spacing float64, sizes []float64) []float64 {
	sizes = growF64(sizes, len(hints))
	if len(hints) == 0 {
		return sizes
	}
	var fixed float64
	stretch := 0
	for _, hint := range hints {
		if hint.Stretch > 0 {
			stretch += hint.Stretch
		} else {
			width := hint.Width
			if width <= 0 {
				width = hint.MinWidth
			}
			if width <= 0 {
				width = 32
			}
			fixed += width
		}
	}
	gaps := float64(len(hints)-1) * spacing
	remaining := max(0, available-fixed-gaps)
	var total, shrinkCapacity float64
	for i, hint := range hints {
		width := hint.Width
		if hint.Stretch > 0 {
			width = remaining * float64(hint.Stretch) / float64(max(1, stretch))
		} else if width <= 0 {
			width = hint.MinWidth
			if width <= 0 {
				width = 32
			}
		}
		if hint.MinWidth > 0 {
			width = max(width, hint.MinWidth)
		}
		if hint.MaxWidth > 0 {
			width = min(width, hint.MaxWidth)
		}
		sizes[i] = width
		total += width
		if hint.Policy&ShrinkHorizontal != 0 {
			shrinkCapacity += max(0, width-layoutDimension(hint.MinWidth))
		}
	}
	if excess := total + gaps - available; excess > 0 && shrinkCapacity > 0 {
		ratio := min(1, excess/shrinkCapacity)
		for i, hint := range hints {
			if hint.Policy&ShrinkHorizontal != 0 {
				sizes[i] -= max(0, sizes[i]-layoutDimension(hint.MinWidth)) * ratio
			}
		}
	}
	return sizes
}
