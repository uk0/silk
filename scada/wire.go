package scada

import (
	"sync"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/device"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/report"
)

// ScreenOptions carries the per-screen configuration the backend-free gui panels
// deliberately do NOT hold: which tag feeds the trend chart, which tags a recipe
// captures, which tags the statistics table tracks, how far back the event log
// and reports reach, and whether the field-device drivers (which open sockets)
// are started. Zero-valued fields fall back to the defaults applied by
// normalize; only EnableDrivers has no default (it must be opt-in).
type ScreenOptions struct {
	TrendTag    string   // live/history tag feeding a TrendPanel's chart
	TrendSeries string   // chart series name (default "trend")
	RecipeTags  []string // tags a RecipePanel captures into a new recipe
	StatsTags   []string // tags a StatsPanel tracks and displays

	EventWindow time.Duration // how far back an EventLogPanel queries (default 1h)

	ReportTags      []string         // columns a ReportView builds
	ReportWindow    time.Duration    // report time span (default 1h)
	ReportInterval  time.Duration    // report bucket width (default 5m)
	ReportAggregate report.Aggregate // report bucket reducer (default Avg)
	ReportDir       string           // dir for exported CSV/HTML (no file written when empty)

	MaxPlaybackPoints int // Downsample budget for history playback (default 500)

	AnimateDur time.Duration // ease duration for float industrial widgets (default 300ms)

	EnableDrivers bool // start DeviceComponents (opens device sockets) when true
}

// screen-wiring defaults, applied by normalize for any zero-valued option.
const (
	defaultTrendSeries       = "trend"
	defaultEventWindow       = time.Hour
	defaultReportWindow      = time.Hour
	defaultReportInterval    = 5 * time.Minute
	defaultMaxPlaybackPoints = 500
	defaultAnimateDur        = 300 * time.Millisecond
)

// normalize fills zero-valued options with their defaults, returning a ready-to-
// use copy so callers may pass a bare ScreenOptions{}.
func (o ScreenOptions) normalize() ScreenOptions {
	if o.TrendSeries == "" {
		o.TrendSeries = defaultTrendSeries
	}
	if o.EventWindow <= 0 {
		o.EventWindow = defaultEventWindow
	}
	if o.ReportWindow <= 0 {
		o.ReportWindow = defaultReportWindow
	}
	if o.ReportInterval <= 0 {
		o.ReportInterval = defaultReportInterval
	}
	if o.MaxPlaybackPoints <= 0 {
		o.MaxPlaybackPoints = defaultMaxPlaybackPoints
	}
	if o.AnimateDur <= 0 {
		o.AnimateDur = defaultAnimateDur
	}
	return o
}

// BindScreen wires a live screen (rooted at root) to the container's shared
// stores. It walks the widget tree once and, per widget:
//
//   - a tag-bound industrial widget (the TagName() seam) is bound to its tag —
//     eased float sinks via gui.BindTagAnimated, boolean sinks via a posted
//     gui.BindTag;
//   - a *gui.AlarmPanel is bound live to the AlarmDB and its ack intent routed
//     back to AlarmDB.Ack;
//   - each of the five operator panels (Recipe / Report / EventLog / Trend /
//     Stats) is wired through a helper that speaks only the panel's Set*/Sig*
//     API and marshals async backend results back with gui.Post;
//   - a *device.DeviceComponent has its point tag names registered now and is
//     started LAST (only when opt.EnableDrivers), so binding never races the
//     first poll.
//
// The returned stop func is idempotent and aggregates every unsubscribe / stop
// in reverse. A device start failure rolls back everything already wired and
// returns the error.
func BindScreen(s *Services, root gui.IWidget, opt ScreenOptions) (stop func(), err error) {
	opt = opt.normalize()

	var cleanup []func()
	add := func(c func()) {
		if c != nil {
			cleanup = append(cleanup, c)
		}
	}
	rollback := func() {
		for i := len(cleanup) - 1; i >= 0; i-- {
			cleanup[i]()
		}
	}

	var devices []*device.DeviceComponent

	var walk func(w gui.IWidget)
	walk = func(w gui.IWidget) {
		if w == nil {
			return
		}
		switch t := w.(type) {
		case *gui.AlarmPanel:
			add(wireAlarmPanel(s, t))
			return
		case *gui.RecipePanel:
			add(wireRecipePanel(s, t, opt))
			return
		case *gui.ReportView:
			add(wireReportView(s, t, opt))
			return
		case *gui.EventLogPanel:
			add(wireEventLogPanel(s, t, opt))
			return
		case *gui.TrendPanel:
			add(wireTrendPanel(s, t, opt))
			return
		case *gui.StatsPanel:
			add(wireStatsPanel(s, t, opt))
			return
		case *device.DeviceComponent:
			devices = append(devices, t) // started last, below
			return
		default:
			add(bindIndustrial(s, w, opt))
		}
		for _, c := range w.Children() {
			walk(c)
		}
	}
	walk(root)

	// Devices come up last, and only when explicitly enabled: register their
	// point tags first so a binding resolving the same name reuses the tag, then
	// start polling. A failure unwinds everything wired so far.
	if opt.EnableDrivers {
		for _, d := range devices {
			registerDevicePoints(s, d)
			if err := d.Start(s.Tags); err != nil {
				rollback()
				return nil, err
			}
			dev := d
			add(func() { dev.Stop() })
		}
	}

	var once sync.Once
	return func() { once.Do(rollback) }, nil
}

// registerDevicePoints pre-creates a tag for each of the component's parsed
// points, so a widget or panel that binds the same name shares the one tag the
// poller will drive. A malformed point list is skipped here — d.Start surfaces
// the parse error when the device actually comes up.
func registerDevicePoints(s *Services, d *device.DeviceComponent) {
	points, err := device.ParsePoints(d.Points())
	if err != nil {
		return
	}
	for _, p := range points {
		s.Tags.GetOrCreate(p.Tag, core.Meta{})
	}
}

// wireAlarmPanel binds p live to the shared AlarmDB and routes the operator's
// ack intent back to AlarmDB.Ack. BindAlarmDB seeds and subscribes (posting its
// own refreshes); the returned func unsubscribes and detaches the ack callback.
func wireAlarmPanel(s *Services, p *gui.AlarmPanel) func() {
	cancel := p.BindAlarmDB(s.Alarms)
	p.SigAckRequested(func(tag string) { s.Alarms.Ack(tag) })
	return func() {
		cancel()
		p.SigAckRequested(nil)
	}
}

// bindIndustrial binds a tag-driven industrial widget to its tag through the
// structural TagName() seam. Only the eight industrial widgets expose TagName(),
// so any other widget (containers, charts, inputs) falls through as a no-op.
// Float sinks ease to each new sample via gui.BindTagAnimated; boolean sinks
// snap via a posted gui.BindTag. Each widget exposes exactly one of these
// setters, so the type switch is unambiguous.
func bindIndustrial(s *Services, w gui.IWidget, opt ScreenOptions) func() {
	namer, ok := w.(interface{ TagName() string })
	if !ok {
		return nil
	}
	name := namer.TagName()
	if name == "" {
		return nil
	}
	tag := s.Tags.GetOrCreate(name, core.Meta{})

	switch sink := w.(type) {
	case interface{ SetLevel(float64) }: // Tank
		return gui.BindTagAnimated(tag, sink.SetLevel, opt.AnimateDur)
	case interface{ SetValue(float64) }: // DigitalDisplay, Thermometer, ValueBar
		return gui.BindTagAnimated(tag, sink.SetValue, opt.AnimateDur)
	case interface{ SetOn(bool) }: // Indicator
		return bindBool(tag, sink.SetOn)
	case interface{ SetState(bool) }: // Valve
		return bindBool(tag, sink.SetState)
	case interface{ SetRunning(bool) }: // Pump
		return bindBool(tag, sink.SetRunning)
	case interface{ SetActive(bool) }: // Pipe
		return bindBool(tag, sink.SetActive)
	}
	return nil
}

// bindBool drives a boolean widget setter from a tag's truthiness. The tag
// subscriber fires on a poll goroutine, so the setter is marshalled onto the UI
// thread with gui.Post before it touches widget state.
func bindBool(tag *core.Tag, set func(bool)) func() {
	return gui.BindTag(tag, func(v interface{}) {
		b := gui.TagBool(v)
		gui.Post(func() { set(b) })
	})
}
