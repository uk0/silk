// Silk SCADA / 组态 HMI — end-to-end tag-driven water-tank demo.
//
// Proves the whole 组态 loop actually runs on top of Silk:
//
//	background sim goroutine
//	    -> core.Tag.SetValue          (goroutine-safe real-time point)
//	        -> core.Tag fan-out --------------> gui bridge (marshals via gui.Post)
//	        |                                       -> BindTagAnimated  -> Tank / Gauge ease
//	        |                                       -> BindTag          -> DigitalDisplay / Valve
//	        |                                       -> ThresholdColor    -> Tank liquid recolors
//	        -> core.AlarmDB.Watch (limit engine)
//	                -> AlarmDB.Subscribe -> gui.Post -> Indicator turns red + alarm banner
//
// Watch it run: the tank fills and drains smoothly (eased), the temperature
// gauge needle sweeps, the trend chart scrolls left in real time, and the pump
// lamp + banner flip red the instant the level enters the HiHi band.
//
// Build:
//
//	CGO_CFLAGS="$(pkg-config --cflags cairo)" go build -o scada ./examples/scada/
//
// Run:
//
//	go run ./examples/scada
package main

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

var (
	colBlue  = paint.Color{R: 33, G: 150, B: 243, A: 255}
	colRed   = paint.Color{R: 239, G: 83, B: 80, A: 255}
	colGreen = paint.Color{R: 76, G: 175, B: 80, A: 255}
)

func main() {
	// ── 1. Tag database: the real-time point model with engineering metadata ──
	// Meta drives the tank %, the gauge span AND the alarm limits (LoLo/Lo/Hi/HiHi).
	tags := core.NewTagDB()
	level := tags.GetOrCreate("level", core.Meta{
		Unit: "%", Min: 0, Max: 100,
		LoLo: 5, Lo: 20, Hi: 80, HiHi: 95,
		Desc: "Tank level",
	})
	temp := tags.GetOrCreate("temp", core.Meta{
		Unit: "C", Min: 0, Max: 120, Hi: 90,
		Desc: "Process temperature",
	})
	pump := tags.GetOrCreate("pump", core.Meta{Desc: "Feed pump run"})

	// Prime with sane initial values so every bound widget paints live data the
	// instant it subscribes (Subscribe primes with the current sample).
	level.SetValue(50.0)
	temp.SetValue(25.0)
	pump.SetValue(true)

	// ── 2. Alarm engine: auto-evaluate each new sample against its Meta limits ──
	alarms := core.NewAlarmDB()
	alarms.Watch(level)
	alarms.Watch(temp)

	// ── 3. HMI screen ──────────────────────────────────────────────────────
	form := gui.NewForm()
	form.SetTitle("Silk SCADA HMI")

	fam := gui.Theme().Font.Family()
	title := gui.NewLabel("Silk SCADA / 组态 HMI — 水箱监控 (tag → widget → animation + alarm + trend)")
	title.SetFont(paint.NewFont(fam, 16, true, false))
	title.SetAlign(gui.AlignCenter)
	title.SetParent(form)
	title.SetBounds(20, 12, 860, 26)

	// Tank (level) — left column.
	tank := gui.NewTank()
	tank.SetColor(colBlue)
	tank.SetParent(form)
	tank.SetBounds(40, 70, 120, 300)
	tankCap := gui.NewLabel("液位 Level  (0–100 %)")
	tankCap.SetAlign(gui.AlignCenter)
	tankCap.SetParent(form)
	tankCap.SetBounds(24, 376, 150, 18)

	legend := gui.NewLabel(
		"实时点位 Tags\n" +
			"  level  0–100 %\n" +
			"    Lo 20  Hi 80\n" +
			"    HiHi 95 → RED\n" +
			"  temp   0–120 ℃\n" +
			"    Hi 90\n" +
			"  pump   bool\n\n" +
			"每 500ms 仿真\nsim @ 500 ms")
	legend.SetFont(paint.NewFont(fam, 11, false, false))
	legend.SetParent(form)
	legend.SetBounds(28, 402, 150, 220)

	// Temperature gauge (needle) + 7-seg readout — middle column.
	gauge := gui.NewGauge()
	gauge.SetTitle("温度 Temp")
	gauge.SetUnit("℃")
	gauge.SetRange(0, 120)
	gauge.AddZone(0, 90, colGreen)
	gauge.AddZone(90, 120, colRed)
	gauge.SetParent(form)
	gauge.SetBounds(190, 70, 340, 210)

	digital := gui.NewDigitalDisplay()
	digital.SetFormat("%.1f")
	digital.SetUnit("℃")
	digital.SetLimits(0, 90) // segments go red at/above Hi
	digital.SetParent(form)
	digital.SetBounds(190, 292, 340, 66)

	// Pump status lamp + feed valve — right column.
	pumpLamp := gui.NewIndicator()
	pumpLamp.SetColor(colGreen)
	pumpLamp.SetParent(form)
	pumpLamp.SetBounds(610, 78, 58, 58)
	pumpCap := gui.NewLabel("PUMP 泵 / HiHi 报警")
	pumpCap.SetParent(form)
	pumpCap.SetBounds(560, 140, 200, 18)

	valve := gui.NewValve()
	valve.SetParent(form)
	valve.SetBounds(600, 172, 120, 58)
	valveCap := gui.NewLabel("VALVE 进料阀 (=pump)")
	valveCap.SetParent(form)
	valveCap.SetBounds(560, 234, 200, 18)

	banner := gui.NewLabel("系统正常  Normal")
	banner.SetFont(paint.NewFont(fam, 13, true, false))
	banner.SetTextColor(colGreen)
	banner.SetParent(form)
	banner.SetBounds(560, 272, 330, 90)

	// Live rolling trend of temperature — full-width bottom.
	trend := gui.NewLineChart()
	trend.SetTitle("温度趋势 Temp trend (live)")
	trend.EnableRolling("temp", 240) // 240 × 0.5s ≈ 2 min ring
	trend.SetTimeWindow(30 * time.Second)
	trend.SetGPUAccelerated(true) // draw the high-rate trend line via the GL fast-path
	trend.SetParent(form)
	trend.SetBounds(190, 375, 700, 255)

	// ── 4. Bindings: tag change → widget property ────────────────────────────
	// Tank + gauge EASE to each new setpoint (BindTagAnimated marshals onto the
	// UI thread and drives a gui.Animation internally — no gui.Post needed here).
	gui.BindTagAnimated(level, func(v float64) { tank.SetLevel(v / 100) }, 450*time.Millisecond)
	gui.BindTagAnimated(temp, gauge.SetValue, 450*time.Millisecond)

	// The remaining helpers fire on the sim goroutine, so the host marshals every
	// widget mutation onto the UI thread with gui.Post (per the tagbinding seam).
	gui.BindTag(temp, func(v interface{}) {
		f := gui.TagFloat(v)
		gui.Post(func() { digital.SetValue(f) })
	})
	gui.BindTag(pump, func(v interface{}) {
		open := gui.TagBool(v)
		gui.Post(func() { valve.SetState(open) })
	})

	// Tank liquid recolors from the level's threshold band: red once in HiHi.
	gui.ThresholdColorBinding(gui.WrapTag(level),
		[]gui.ColorRange{
			{Min: 95, Max: math.MaxFloat64, Color: colRed}, // HiHi and above
			{Min: -math.MaxFloat64, Max: 95, Color: colBlue},
		},
		func(c paint.Color) { gui.Post(func() { tank.SetColor(c) }) })

	// Live trend: subscribe straight to the tag so the sample's own timestamp
	// drives the X axis; marshal the widget mutation with gui.Post.
	temp.Subscribe(func(v core.Value) {
		f, ts := v.Float(), v.Time
		gui.Post(func() { trend.AddSample("temp", ts, f) })
	})

	// ── 5. Alarm → UI: pump lamp turns red on level HiHi; banner lists alarms ──
	// Recomputed on the UI thread from the thread-safe AlarmDB / Tag snapshots,
	// so pump-state and alarm-state updates never race over the one lamp.
	refreshAlarmUI := func() {
		gui.Post(func() {
			act := alarms.Active()
			levelHiHi := false
			parts := make([]string, 0, len(act))
			for _, a := range act {
				parts = append(parts, fmt.Sprintf("%s %s=%.0f", a.Tag, a.Severity, a.Value))
				if a.Tag == "level" && a.Severity == core.HighHigh {
					levelHiHi = true
				}
			}
			if len(act) == 0 {
				banner.SetText("系统正常  Normal — no active alarms")
				banner.SetTextColor(colGreen)
			} else {
				banner.SetText("报警 ALARM\n" + strings.Join(parts, "\n"))
				banner.SetTextColor(colRed)
			}
			// Lamp: lit while the pump runs; forced red + lit while level is HiHi.
			if levelHiHi {
				pumpLamp.SetColor(colRed)
				pumpLamp.SetOn(true)
			} else {
				pumpLamp.SetColor(colGreen)
				pumpLamp.SetOn(pump.Value().Bool())
			}
		})
	}
	alarms.Subscribe(func(core.AlarmState) { refreshAlarmUI() })
	pump.Subscribe(func(core.Value) { refreshAlarmUI() })
	refreshAlarmUI() // prime the lamp + banner from the initial state

	// ── 6. Simulator: a background goroutine feeds NEW values every 500 ms ────
	// It only calls SetValue (goroutine-safe); all widget/animation work happens
	// on the UI thread through the bridge above.
	go func() {
		start := time.Now()
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for range tick.C {
			el := time.Since(start).Seconds()
			lv := 52 + 47*math.Sin(el*2*math.Pi/16)                     // 5 → 99, period 16 s
			tp := 55 + 40*math.Sin(el*2*math.Pi/12) + (rand.Float64()-0.5)*4 // ~15 → 95 + noise
			level.SetValue(lv)
			temp.SetValue(tp)
			pump.SetValue(lv < 95) // feed pump trips off when the tank overfills
		}
	}()

	// ── 7. Show + run the event loop (drains gui.Post + ticks the animations) ──
	form.AttachWindow(gui.WtForm)
	form.Window().SetSize(900, 650)
	form.Window().MoveToCenter()
	form.Show()
	core.EventLoop()
}
