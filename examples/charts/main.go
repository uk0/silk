// Silk Chart Widget Gallery
//
// Demonstrates all five chart widget types: LineChart, BarChart,
// PieChart, Gauge, and ScatterPlot arranged in a single window.
//
// Build:
//
//	CGO_CFLAGS="$(pkg-config --cflags cairo)" go build -o charts ./examples/charts/
//
// Run:
//
//	./charts
package main

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"math"
	"math/rand"
)

func main() {
	form := gui.NewForm()
	form.SetTitle("Silk Chart Gallery")

	// ---------------------------------------------------------------
	// 1. Line Chart (top-left)
	// ---------------------------------------------------------------
	lc := gui.NewLineChart()
	lc.SetTitle("Temperature (24h)")
	lc.SetShowGrid(true)
	lc.SetShowLegend(true)

	// Generate sample data
	indoor := make([]float64, 24)
	outdoor := make([]float64, 24)
	for i := 0; i < 24; i++ {
		indoor[i] = 21 + 3*math.Sin(float64(i)*math.Pi/12) + rand.Float64()
		outdoor[i] = 15 + 10*math.Sin(float64(i)*math.Pi/12) + rand.Float64()*2
	}
	lc.AddSeries("Indoor", paint.Color{R: 65, G: 131, B: 215, A: 255}, indoor)
	lc.AddSeries("Outdoor", paint.Color{R: 228, G: 77, B: 66, A: 255}, outdoor)

	lc.SetParent(form)
	lc.SetBounds(10, 10, 380, 240)

	// ---------------------------------------------------------------
	// 2. Bar Chart (top-right)
	// ---------------------------------------------------------------
	bc := gui.NewBarChart()
	bc.SetTitle("Monthly Revenue")
	bc.SetShowValues(true)
	bc.AddBar("Jan", 120, paint.Color{R: 65, G: 131, B: 215, A: 255})
	bc.AddBar("Feb", 95, paint.Color{R: 228, G: 77, B: 66, A: 255})
	bc.AddBar("Mar", 150, paint.Color{R: 90, G: 185, B: 102, A: 255})
	bc.AddBar("Apr", 180, paint.Color{R: 249, G: 168, B: 37, A: 255})
	bc.AddBar("May", 135, paint.Color{R: 148, G: 103, B: 189, A: 255})
	bc.AddBar("Jun", 200, paint.Color{R: 64, G: 196, B: 188, A: 255})

	bc.SetParent(form)
	bc.SetBounds(400, 10, 380, 240)

	// ---------------------------------------------------------------
	// 3. Pie Chart (bottom-left)
	// ---------------------------------------------------------------
	pc := gui.NewPieChart()
	pc.SetTitle("Market Share")
	pc.SetShowLabels(true)
	pc.SetShowPercent(true)
	pc.AddSlice("Chrome", 64, paint.Color{R: 65, G: 131, B: 215, A: 255})
	pc.AddSlice("Safari", 19, paint.Color{R: 90, G: 185, B: 102, A: 255})
	pc.AddSlice("Firefox", 4, paint.Color{R: 228, G: 77, B: 66, A: 255})
	pc.AddSlice("Edge", 4, paint.Color{R: 249, G: 168, B: 37, A: 255})
	pc.AddSlice("Other", 9, paint.Color{R: 148, G: 103, B: 189, A: 255})

	pc.SetParent(form)
	pc.SetBounds(10, 260, 250, 250)

	// ---------------------------------------------------------------
	// 4. Gauge (bottom-center)
	// ---------------------------------------------------------------
	ga := gui.NewGauge()
	ga.SetTitle("CPU Usage")
	ga.SetUnit("%")
	ga.SetRange(0, 100)
	ga.AddZone(0, 60, paint.Color{R: 90, G: 185, B: 102, A: 255})  // green
	ga.AddZone(60, 80, paint.Color{R: 249, G: 168, B: 37, A: 255}) // yellow
	ga.AddZone(80, 100, paint.Color{R: 228, G: 77, B: 66, A: 255}) // red
	ga.SetValue(67)

	ga.SetParent(form)
	ga.SetBounds(270, 260, 250, 190)

	// ---------------------------------------------------------------
	// 5. Scatter Plot (bottom-right)
	// ---------------------------------------------------------------
	sp := gui.NewScatterPlot()
	sp.SetTitle("Height vs Weight")
	sp.SetShowGrid(true)
	sp.SetShowLegend(true)

	male := make([]gui.ScatterPoint, 30)
	female := make([]gui.ScatterPoint, 30)
	for i := 0; i < 30; i++ {
		male[i] = gui.ScatterPoint{
			X: 165 + rand.Float64()*25,
			Y: 65 + rand.Float64()*30,
		}
		female[i] = gui.ScatterPoint{
			X: 150 + rand.Float64()*20,
			Y: 45 + rand.Float64()*25,
		}
	}
	sp.AddSeries("Male", paint.Color{R: 65, G: 131, B: 215, A: 255}, male)
	sp.AddSeries("Female", paint.Color{R: 228, G: 77, B: 66, A: 255}, female)

	sp.SetParent(form)
	sp.SetBounds(530, 260, 250, 250)

	// ---------------------------------------------------------------
	// Show
	// ---------------------------------------------------------------
	form.AttachWindow(gui.WtForm)
	form.Window().SetSize(800, 530)
	form.Window().MoveToCenter()
	form.Show()
	core.EventLoop()
}
