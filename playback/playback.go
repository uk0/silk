// Package playback replays recorded historian history into a LineChart: it
// loads a tag's samples over a time range, optionally decimates them to a point
// budget, and feeds them into a rolling series so the chart's time axis shows
// the persisted trend instead of only the live tail.
package playback

import (
	"time"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/historian"
)

// LoadRange is a thin, stable wrapper over Historian.Query: it returns tag's
// samples whose timestamp is in [from, to] inclusive, ordered ascending. The
// indirection lets callers depend on playback rather than the historian query
// signature directly.
func LoadRange(h *historian.Historian, tag string, from, to time.Time) ([]historian.Sample, error) {
	return h.Query(tag, from, to)
}

// Downsample evenly decimates samples down to about maxPoints points, always
// keeping the first and the last. It is pure and allocation-light: when
// maxPoints < 1, or there are no more than maxPoints samples, the input slice
// is returned unchanged (so empty and single-element inputs pass through).
func Downsample(samples []historian.Sample, maxPoints int) []historian.Sample {
	if maxPoints < 1 || len(samples) <= maxPoints {
		return samples
	}
	// len(samples) > maxPoints >= 1, so there are at least two samples.
	if maxPoints == 1 {
		// A one-point budget cannot hold both ends; keep the two endpoints.
		return []historian.Sample{samples[0], samples[len(samples)-1]}
	}
	last := len(samples) - 1
	out := make([]historian.Sample, maxPoints)
	// Map i in [0, maxPoints-1] onto an index in [0, last]; i==0 -> first and
	// i==maxPoints-1 -> last, with the interior points evenly spaced. Because
	// last >= maxPoints, the step exceeds 1 and the picked indices are strictly
	// increasing (no duplicates).
	for i := 0; i < maxPoints; i++ {
		out[i] = samples[int(int64(i)*int64(last)/int64(maxPoints-1))]
	}
	return out
}

// FeedChart replays samples into a rolling series named seriesName on chart: it
// sizes the series' ring buffer to hold exactly the samples, then pushes each
// (TS, Value) so the chart's time axis spans the loaded history rather than
// dropping the oldest points.
func FeedChart(chart *gui.LineChart, seriesName string, samples []historian.Sample) {
	chart.EnableRolling(seriesName, len(samples))
	for _, s := range samples {
		chart.AddSample(seriesName, s.TS, s.Value)
	}
}

// PlayInto loads tag's history in [from, to], decimates it to at most maxPoints
// points, and feeds the result into chart's seriesName series. It returns any
// error from the underlying query; on success chart holds the range's trend.
func PlayInto(chart *gui.LineChart, h *historian.Historian, seriesName, tag string, from, to time.Time, maxPoints int) error {
	samples, err := LoadRange(h, tag, from, to)
	if err != nil {
		return err
	}
	FeedChart(chart, seriesName, Downsample(samples, maxPoints))
	return nil
}
