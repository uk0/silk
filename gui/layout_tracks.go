package gui

import "github.com/uk0/silk/geom"

// LayoutTrack is a visible row or column in parent-local logical pixels.
// Index is the authored row/column number. Form layouts can omit empty rows
// without renumbering the active ones.
type LayoutTrack struct {
	Index       int
	Start, Size float64
}

// LayoutCellTracks is a snapshot of solved tracks, excluding padding and gaps.
// The slices belong to the caller; modifying them cannot change a layout.
type LayoutCellTracks struct {
	Content       geom.Rect
	Rows, Columns []LayoutTrack
}

// LayoutCellTrackSource is implemented by tabular layout managers that can
// expose their solved tracks for design-time selection or drop feedback.
type LayoutCellTrackSource interface {
	CellTracks() LayoutCellTracks
}
