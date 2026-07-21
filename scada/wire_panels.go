package scada

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/eventlog"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/playback"
	"github.com/uk0/silk/recipe"
	"github.com/uk0/silk/report"
	"github.com/uk0/silk/stats"
)

// errNoRecipePath is reported when a recipe Save/Load fires but Config left the
// recipe path empty.
var errNoRecipePath = errors.New("scada: no recipe path configured")

// --- Recipe panel ---------------------------------------------------------

// wireRecipePanel connects a RecipePanel to the shared recipe book: it seeds the
// name list and routes the four operator intents (Apply / Capture / Save / Load)
// to the recipe package. All four fire on the UI thread from a button click and
// touch only fast in-memory / small-file operations, so none needs gui.Post.
// The returned func detaches every callback.
func wireRecipePanel(s *Services, p *gui.RecipePanel, opt ScreenOptions) func() {
	p.SetRecipes(s.Recipes.List())

	p.SigApply(func(name string) {
		if r, ok := s.Recipes.Get(name); ok {
			recipe.Apply(r, s.Tags)
			s.RecordEvent(eventlog.KindWrite, name, "recipe applied")
		}
	})
	p.SigCapture(func(name string) {
		r := recipe.Capture(name, s.Tags, opt.RecipeTags)
		s.Recipes.Add(r)
		p.SetRecipes(s.Recipes.List())
		s.RecordEvent(eventlog.KindWrite, name, "recipe captured")
	})
	p.SigSave(func() {
		if s.cfg.RecipePath == "" {
			s.reportError(errNoRecipePath)
			return
		}
		if err := s.Recipes.Save(s.cfg.RecipePath); err != nil {
			s.reportError(err)
			return
		}
		s.RecordEvent(eventlog.KindSystem, "recipe", "book saved")
	})
	p.SigLoad(func() {
		if s.cfg.RecipePath == "" {
			s.reportError(errNoRecipePath)
			return
		}
		book, err := recipe.Load(s.cfg.RecipePath)
		if err != nil {
			s.reportError(err)
			return
		}
		*s.Recipes = *book
		p.SetRecipes(s.Recipes.List())
		s.RecordEvent(eventlog.KindSystem, "recipe", "book loaded")
	})

	return func() {
		p.SigApply(nil)
		p.SigCapture(nil)
		p.SigSave(nil)
		p.SigLoad(nil)
	}
}

// --- Report view ----------------------------------------------------------

// wireReportView fills a ReportView from historian-backed interval reports and
// serialises an export on demand. Both report.Build calls run on a background
// goroutine and the display result is marshalled back with gui.Post; the export
// writes a file (when ReportDir is set) and books the action as an event. The
// returned func detaches the export callback.
func wireReportView(s *Services, p *gui.ReportView, opt ScreenOptions) func() {
	build := func() (*report.Report, error) {
		to := time.Now()
		from := to.Add(-opt.ReportWindow)
		return report.Build(s.Hist, opt.ReportTags, from, to, opt.ReportInterval, opt.ReportAggregate)
	}

	// Seed the table asynchronously so the panel fills without blocking wiring.
	go func() {
		r, err := build()
		if err != nil {
			s.reportError(err)
			return
		}
		headers, rows := reportToTable(r)
		gui.Post(func() { p.SetTable(headers, rows) })
	}()

	p.SigExport(func(format string) {
		go func() {
			r, err := build()
			if err != nil {
				s.reportError(err)
				return
			}
			if err := exportReport(r, format, opt.ReportDir); err != nil {
				s.reportError(err)
				return
			}
			s.RecordEvent(eventlog.KindWrite, "report", "export "+format)
		}()
	})

	return func() { p.SigExport(nil) }
}

// reportToTable flattens a report grid into the ReportView's plain view-model: a
// "time" + tag-name header and one string row per bucket, empty cells for tags
// absent in a bucket.
func reportToTable(r *report.Report) (headers []string, rows [][]string) {
	headers = append([]string{"time"}, r.Tags...)
	rows = make([][]string, len(r.Buckets))
	for i, b := range r.Buckets {
		row := make([]string, 1+len(r.Tags))
		row[0] = b.From.Format("15:04:05")
		for j, tag := range r.Tags {
			if v, ok := b.Values[tag]; ok {
				row[j+1] = strconv.FormatFloat(v, 'g', -1, 64)
			}
		}
		rows[i] = row
	}
	return headers, rows
}

// exportReport serialises r to dir/report.<format>. A "csv" or "html" format is
// honoured; an empty dir skips the file write (the caller still books the export
// event); any other format is an error.
func exportReport(r *report.Report, format, dir string) error {
	if dir == "" {
		return nil
	}
	var ext string
	switch format {
	case "csv", "html":
		ext = format
	default:
		return errors.New("scada: unknown report format " + format)
	}
	f, err := os.Create(filepath.Join(dir, "report."+ext))
	if err != nil {
		return err
	}
	defer f.Close()
	if format == "csv" {
		return r.WriteCSV(f)
	}
	return r.WriteHTML(f)
}

// --- Event log panel ------------------------------------------------------

// wireEventLogPanel fills an EventLogPanel from the event log over the last
// EventWindow and re-queries when the operator picks a kind tab. Every query
// runs on a background goroutine and the rows are marshalled back with gui.Post;
// the panel's own client-side filter is set in step so a fast click reads right
// before the query returns. The returned func detaches the filter callback.
func wireEventLogPanel(s *Services, p *gui.EventLogPanel, opt ScreenOptions) func() {
	refresh := func(kind string) {
		to := time.Now()
		from := to.Add(-opt.EventWindow)
		go func() {
			var (
				events []eventlog.Event
				err    error
			)
			if kind == "" {
				events, err = s.Events.Query(from, to)
			} else {
				events, err = s.Events.Query(from, to, eventlog.Kind(kind))
			}
			if err != nil {
				s.reportError(err)
				return
			}
			rows := eventsToRows(events)
			gui.Post(func() { p.SetEvents(rows) })
		}()
	}

	refresh("") // seed with the full window

	p.SigFilter(func(kind string) {
		p.SetKindFilter(kind)
		refresh(kind)
	})

	return func() { p.SigFilter(nil) }
}

// eventsToRows renders backend events into the panel's flat EventRow view-model,
// host-formatting the timestamp the panel never parses.
func eventsToRows(events []eventlog.Event) []gui.EventRow {
	rows := make([]gui.EventRow, len(events))
	for i, e := range events {
		rows[i] = gui.EventRow{
			Time:    e.TS.Format("15:04:05"),
			Kind:    string(e.Kind),
			Source:  e.Source,
			Message: e.Message,
		}
	}
	return rows
}

// --- Trend panel ----------------------------------------------------------

// wireTrendPanel feeds a TrendPanel's owned chart: the live TrendTag streams in
// (each sample posted onto the UI thread) and a range pick replays that tag's
// persisted history through playback, loaded on a background goroutine and fed
// back with gui.Post. Play / pause / mode toggles are booked as system events.
// The returned func stops the live feed and detaches every callback.
func wireTrendPanel(s *Services, p *gui.TrendPanel, opt ScreenOptions) func() {
	chart := p.Chart()
	series := opt.TrendSeries
	chart.EnableRolling(series, opt.MaxPlaybackPoints)

	var liveCancel func()
	if opt.TrendTag != "" {
		tag := s.Tags.GetOrCreate(opt.TrendTag, core.Meta{})
		liveCancel = tag.Subscribe(func(v core.Value) {
			f := v.Float()
			gui.Post(func() { chart.AddSample(series, time.Now(), f) })
		})
	}

	p.SigRange(func(d time.Duration) {
		if opt.TrendTag == "" {
			return
		}
		to := time.Now()
		from := to.Add(-d)
		go func() {
			samples, err := playback.LoadRange(s.Hist, opt.TrendTag, from, to)
			if err != nil {
				s.reportError(err)
				return
			}
			samples = playback.Downsample(samples, opt.MaxPlaybackPoints)
			gui.Post(func() { playback.FeedChart(chart, series, samples) })
		}()
	})
	p.SigPlay(func() { s.RecordEvent(eventlog.KindSystem, "trend", "play") })
	p.SigPause(func() { s.RecordEvent(eventlog.KindSystem, "trend", "pause") })
	p.SigModeChanged(func(live bool) {
		mode := "history"
		if live {
			mode = "live"
		}
		s.RecordEvent(eventlog.KindSystem, "trend", "mode "+mode)
	})

	return func() {
		if liveCancel != nil {
			liveCancel()
		}
		p.SigRange(nil)
		p.SigPlay(nil)
		p.SigPause(nil)
		p.SigModeChanged(nil)
	}
}

// --- Stats panel ----------------------------------------------------------

// wireStatsPanel tracks the StatsTags in the shared collector and pushes a fresh
// snapshot into a StatsPanel: it seeds synchronously at wire time, refreshes on
// every subsequent tag change (marshalled onto the UI thread with gui.Post,
// since the change fires on a poll goroutine), and clears a tag on SigReset. The
// returned func drops the change subscriptions and detaches the reset callback;
// the tags stay tracked in the shared collector until Services.Stop.
func wireStatsPanel(s *Services, p *gui.StatsPanel, opt ScreenOptions) func() {
	for _, name := range opt.StatsTags {
		s.Stats.Track(name)
	}

	refresh := func() { p.SetStats(statRows(s.Stats, opt.StatsTags)) }

	var cancels []func()
	for _, name := range opt.StatsTags {
		tag := s.Tags.GetOrCreate(name, core.Meta{})
		cancels = append(cancels, tag.Subscribe(func(core.Value) { gui.Post(refresh) }))
	}

	refresh() // synchronous initial seed (the Post path needs the GL event loop)

	p.SigReset(func(tag string) {
		s.Stats.Reset(tag)
		refresh()
	})

	return func() {
		for _, cancel := range cancels {
			cancel()
		}
		p.SigReset(nil)
	}
}

// statRows snapshots the collector's per-tag aggregates into the panel's flat
// StatRow view-model, in the given tag order. A not-yet-folded tag is skipped.
func statRows(c *stats.Collector, names []string) []gui.StatRow {
	rows := make([]gui.StatRow, 0, len(names))
	for _, name := range names {
		st, ok := c.Get(name)
		if !ok {
			continue
		}
		rows = append(rows, gui.StatRow{
			Tag:   name,
			Count: st.Count,
			Min:   st.Min,
			Max:   st.Max,
			Avg:   st.Avg(),
			Last:  st.Last,
		})
	}
	return rows
}
