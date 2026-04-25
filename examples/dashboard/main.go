// Silk System Monitor Dashboard
//
// A monitoring dashboard with progress bars for CPU, Memory, and Disk
// usage. Labels display percentages. A background goroutine updates
// values randomly every second using data bindings.
//
// Build:
//
//	CGO_CFLAGS="$(pkg-config --cflags cairo)" go build -o dashboard ./examples/dashboard/
//
// Run:
//
//	./dashboard
package main

import (
	"silk/core"
	"silk/gui"
	"silk/paint"
	"fmt"
	"math/rand"
	"time"
)

func main() {
	form := gui.NewForm()
	form.SetTitle("Silk System Monitor")

	// Title
	title := gui.NewLabel("System Monitor Dashboard")
	title.SetFont(paint.NewFont(gui.Theme().Font.Family(), 16, true, false))
	title.SetAlign(gui.AlignCenter)
	title.SetParent(form)
	title.SetBounds(20, 10, 360, 28)

	// Shared bindings
	cpuBinding := gui.NewBinding(0.25)
	memBinding := gui.NewBinding(0.45)
	diskBinding := gui.NewBinding(0.60)

	// CPU row
	cpuLabel := gui.NewLabel("CPU:")
	cpuLabel.SetFont(paint.NewFont(gui.Theme().Font.Family(), 12, true, false))
	cpuLabel.SetParent(form)
	cpuLabel.SetBounds(20, 55, 60, 22)

	cpuBar := gui.NewProgressBar()
	cpuBar.SetParent(form)
	cpuBar.SetBounds(85, 53, 230, 22)
	cpuBar.SetShowText(true)
	gui.BindProgressBar(cpuBar, cpuBinding)

	cpuPct := gui.NewLabel("25%")
	cpuPct.SetParent(form)
	cpuPct.SetBounds(320, 55, 60, 22)
	cpuBinding.Watch(func(v interface{}) {
		cpuPct.SetText(fmt.Sprintf("%.0f%%", cpuBinding.GetFloat()*100))
	})

	// Memory row
	memLabel := gui.NewLabel("Memory:")
	memLabel.SetFont(paint.NewFont(gui.Theme().Font.Family(), 12, true, false))
	memLabel.SetParent(form)
	memLabel.SetBounds(20, 90, 60, 22)

	memBar := gui.NewProgressBar()
	memBar.SetParent(form)
	memBar.SetBounds(85, 88, 230, 22)
	memBar.SetShowText(true)
	gui.BindProgressBar(memBar, memBinding)

	memPct := gui.NewLabel("45%")
	memPct.SetParent(form)
	memPct.SetBounds(320, 90, 60, 22)
	memBinding.Watch(func(v interface{}) {
		memPct.SetText(fmt.Sprintf("%.0f%%", memBinding.GetFloat()*100))
	})

	// Disk row
	diskLabel := gui.NewLabel("Disk:")
	diskLabel.SetFont(paint.NewFont(gui.Theme().Font.Family(), 12, true, false))
	diskLabel.SetParent(form)
	diskLabel.SetBounds(20, 125, 60, 22)

	diskBar := gui.NewProgressBar()
	diskBar.SetParent(form)
	diskBar.SetBounds(85, 123, 230, 22)
	diskBar.SetShowText(true)
	gui.BindProgressBar(diskBar, diskBinding)

	diskPct := gui.NewLabel("60%")
	diskPct.SetParent(form)
	diskPct.SetBounds(320, 125, 60, 22)
	diskBinding.Watch(func(v interface{}) {
		diskPct.SetText(fmt.Sprintf("%.0f%%", diskBinding.GetFloat()*100))
	})

	// Status label
	statusLabel := gui.NewLabel("Monitoring active...")
	statusLabel.SetParent(form)
	statusLabel.SetBounds(20, 165, 300, 22)
	statusLabel.SetFont(paint.NewFont(gui.Theme().Font.Family(), 11, false, true))

	// Update counter
	updateCount := gui.NewLabel("Updates: 0")
	updateCount.SetParent(form)
	updateCount.SetBounds(20, 190, 200, 22)

	// Timer goroutine to update values
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		count := 0
		for range ticker.C {
			count++
			// Simulate CPU: random fluctuations
			cpu := 0.1 + rand.Float64()*0.7
			cpuBinding.Set(cpu)

			// Simulate Memory: gradual changes around 40-70%
			mem := 0.3 + rand.Float64()*0.4
			memBinding.Set(mem)

			// Simulate Disk: slow drift around 50-70%
			disk := 0.5 + rand.Float64()*0.2
			diskBinding.Set(disk)

			updateCount.SetText(fmt.Sprintf("Updates: %d", count))
		}
	}()

	form.AttachWindow(gui.WtForm)
	form.Window().SetSize(400, 225)
	form.Window().MoveToCenter()
	form.Show()
	core.EventLoop()
}
