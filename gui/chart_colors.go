package gui

import "github.com/uk0/silk/paint"

// chartColors is the default color palette used by chart widgets when
// the caller does not supply an explicit color.
var chartColors = []paint.Color{
	{R: 65, G: 131, B: 215, A: 255},  // blue
	{R: 228, G: 77, B: 66, A: 255},   // red
	{R: 90, G: 185, B: 102, A: 255},  // green
	{R: 249, G: 168, B: 37, A: 255},  // orange
	{R: 148, G: 103, B: 189, A: 255}, // purple
	{R: 64, G: 196, B: 188, A: 255},  // teal
}

// chartColor returns the palette color at index i (wrapping around).
func chartColor(i int) paint.Color {
	return chartColors[i%len(chartColors)]
}
