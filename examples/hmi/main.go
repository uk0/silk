// Silk 组态 / HMI — end-to-end example on the scada.Services runtime.
//
// Proves the WHOLE headless runtime loop wires up to a live screen the way the
// ged code generator now emits it: a hardware-free simulator feeds the shared
// real-time tag registry, and a single scada.BindScreen call walks the form and
// auto-binds every industrial widget + operator panel to the backend — this file
// writes ZERO per-widget gui.BindTag glue of its own.
//
//	sim.Sim (sine waveforms)
//	  -> driver.Poller               reads every 500ms
//	      -> services.Tags           core.TagDB — the one real-time point registry
//	          ├─ AlarmDB.Watch       -> alarm bridge -> event log + notify + AlarmPanel
//	          ├─ Historian.Record    -> SQLite sample store (TrendPanel range replay)
//	          ├─ Stats.Collector     -> StatsPanel (count / min / max / avg / last)
//	          └─ BindScreen tag bind:
//	                 Tank.SetTagName("level")           -> BindTagAnimated -> fill eases
//	                 DigitalDisplay.SetTagName("temp")  -> BindTagAnimated -> readout eases
//	              TrendPanel (TrendTag "temp")          -> owned LineChart scrolls live
//
// Everything below the tag registry is installed by services.Start() and
// scada.BindScreen(); the sim.Sim + driver.Poller stand in for a field device.
//
// Run (needs a desktop — Cairo + GLFW, cannot run headless):
//
//	go run ./examples/hmi
package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/driver"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/scada"
	"github.com/uk0/silk/sim"
)

func main() {
	// ── 1. Runtime container: historian / event-log / recipe stores live under
	//       a per-user config dir; TagMeta seeds the registry + its alarm limits.
	cfg := scada.DefaultConfig(hmiDir())
	cfg.TagMeta = map[string]core.Meta{
		"level": {Min: 0, Max: 100, Hi: 80, HiHi: 95, Unit: "%", Desc: "Tank level"},
		"temp":  {Min: 0, Max: 120, Hi: 90, Unit: "℃", Desc: "Process temperature"},
	}
	services, err := scada.New(cfg)
	if err != nil {
		core.Error("hmi: scada.New: ", err)
		os.Exit(1)
	}

	// ── 2. HMI screen: widgets carry design-time tag names, panels are backend-
	//       free. BindScreen (step 3) resolves both against the container. ─────
	form := gui.NewForm()
	form.SetTitle("Silk 组态 / HMI — sim → Poller → Services → screen")

	title := gui.NewLabel("Silk 组态 / HMI 演示 — 端到端运行时回路 (仿真驱动 · 报警 · 趋势 · 统计)")
	title.SetAlign(gui.AlignCenter)
	title.SetParent(form)
	title.SetBounds(20, 12, 860, 24)

	// Tank bound to "level" — auto-wired through its TagName seam, eases to each
	// new sample via BindTagAnimated.
	tank := gui.NewTank()
	tank.SetTagName("level")
	tank.SetParent(form)
	tank.SetBounds(40, 72, 120, 300)
	tankCap := gui.NewLabel("液位 level  0–100 %   (Hi 80 · HiHi 95)")
	tankCap.SetAlign(gui.AlignCenter)
	tankCap.SetParent(form)
	tankCap.SetBounds(10, 380, 180, 18)

	// Temperature 7-segment readout bound to "temp" — same TagName auto-wiring;
	// segments recolor at/above the Hi limit.
	temp := gui.NewDigitalDisplay()
	temp.SetTagName("temp")
	temp.SetFormat("%.1f")
	temp.SetUnit("℃")
	temp.SetLimits(0, 90)
	temp.SetParent(form)
	temp.SetBounds(210, 72, 300, 72)
	tempCap := gui.NewLabel("温度 temp  0–120 ℃   (Hi 90)")
	tempCap.SetAlign(gui.AlignCenter)
	tempCap.SetParent(form)
	tempCap.SetBounds(210, 150, 300, 18)

	// Operator panels — bound live to AlarmDB / Historian / Stats by BindScreen.
	alarms := gui.NewAlarmPanel()
	alarms.SetParent(form)
	alarms.SetBounds(540, 72, 340, 150)

	stats := gui.NewStatsPanel()
	stats.SetParent(form)
	stats.SetBounds(540, 232, 340, 132)

	trend := gui.NewTrendPanel()
	trend.SetParent(form)
	trend.SetBounds(210, 384, 670, 276)

	// ── 3. One call wires the whole screen to the container: TagName widgets to
	//       their tags, AlarmPanel to the AlarmDB, TrendPanel/StatsPanel to the
	//       historian + collector. EnableDrivers is false — the sim poller below
	//       is our field device. (Error is impossible with no DeviceComponents.)
	stop, _ := scada.BindScreen(services, form, scada.ScreenOptions{
		TrendTag:      "temp",
		StatsTags:     []string{"level", "temp"},
		EnableDrivers: false,
	})
	defer stop()

	// ── 4. Start the backend runtime: alarm bridge, per-tag limit watches,
	//       historian recording, stat tracking.
	services.Start()
	defer services.Stop()

	// ── 5. Field device: a hardware-free simulator polled into services.Tags
	//       every 500ms. Type must be Float64 — the zero value (TypeBool) would
	//       coerce the sine waveform to a boolean.
	drv := sim.NewSim()
	poller := driver.NewPoller(drv, []driver.TagPoint{
		{Tag: "level", Address: "sine:min=0,max=100,period=20s", Type: driver.TypeFloat64},
		{Tag: "temp", Address: "sine:min=20,max=110,period=30s", Type: driver.TypeFloat64},
	}, services.Tags, 500*time.Millisecond)
	poller.Start()
	defer poller.Stop()

	// ── 6. Show the window and run the event loop (drains gui.Post, ticks anims).
	form.AttachWindow(gui.WtForm)
	form.Window().SetSize(900, 690)
	form.Window().MoveToCenter()
	form.Show()
	core.EventLoop()
}

// hmiDir returns a per-user directory for the historian / event-log / recipe
// stores, creating it if needed and falling back to the OS temp dir.
func hmiDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "silk-hmi")
	os.MkdirAll(dir, 0o755)
	return dir
}
