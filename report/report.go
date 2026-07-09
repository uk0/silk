// Package report turns silk's persisted tag history into FameView-style 报表
// (interval reports): it slices a time range into fixed buckets, reduces each
// tag's samples in every bucket with a chosen aggregate, and renders the grid
// as CSV or HTML. A screen operator picks tags, a window, an interval and an
// aggregate; Build fills the table one Query per tag per bucket.
package report

import (
	"encoding/csv"
	"fmt"
	"html"
	"io"
	"math"
	"strconv"
	"time"

	"github.com/uk0/silk/historian"
)

// Aggregate names the reducer applied to the samples inside one bucket.
type Aggregate int

const (
	Avg Aggregate = iota // arithmetic mean of the bucket's values
	Min                  // smallest value
	Max                  // largest value
	Sum                  // running total
	Count                // number of samples
	First                // earliest sample (Query returns ascending ts)
	Last                 // latest sample
)

// String renders the aggregate's display name (report column/title text).
func (a Aggregate) String() string {
	switch a {
	case Avg:
		return "Avg"
	case Min:
		return "Min"
	case Max:
		return "Max"
	case Sum:
		return "Sum"
	case Count:
		return "Count"
	case First:
		return "First"
	case Last:
		return "Last"
	default:
		return "Aggregate(" + strconv.Itoa(int(a)) + ")"
	}
}

// aggregate is the pure reducer over one bucket's samples. Samples arrive in
// ascending timestamp order (as historian.Query returns them), so First is the
// earliest and Last the latest. An empty bucket has no defined mean or extreme:
// Sum and Count return 0, every other aggregate returns NaN.
func aggregate(samples []historian.Sample, a Aggregate) float64 {
	if len(samples) == 0 {
		switch a {
		case Sum, Count:
			return 0
		default:
			return math.NaN()
		}
	}
	switch a {
	case Avg:
		sum := 0.0
		for _, s := range samples {
			sum += s.Value
		}
		return sum / float64(len(samples))
	case Min:
		m := samples[0].Value
		for _, s := range samples[1:] {
			if s.Value < m {
				m = s.Value
			}
		}
		return m
	case Max:
		m := samples[0].Value
		for _, s := range samples[1:] {
			if s.Value > m {
				m = s.Value
			}
		}
		return m
	case Sum:
		sum := 0.0
		for _, s := range samples {
			sum += s.Value
		}
		return sum
	case Count:
		return float64(len(samples))
	case First:
		return samples[0].Value
	case Last:
		return samples[len(samples)-1].Value
	default:
		return math.NaN()
	}
}

// Bucket is one report row: a half-open time slice [From, To) and, per tag, the
// aggregated value of that tag's samples in the slice. A tag with no samples in
// the bucket is absent from Values (renders as an empty cell), never stored as
// NaN.
type Bucket struct {
	From, To time.Time
	Values   map[string]float64
}

// Report is the finished grid: the ordered tag columns and the ordered buckets.
type Report struct {
	Tags    []string
	Buckets []Bucket
}

// Build aggregates history for tags over [from, to), one bucket per interval.
// For each bucket it Queries each tag's samples in the half-open slice, reduces
// them with a, and stores the result under Values[tag]; a bucket with no
// samples for a tag omits that tag (empty cell). The final bucket is clamped so
// the report covers exactly [from, to). interval must be positive.
func Build(h *historian.Historian, tags []string, from, to time.Time, interval time.Duration, a Aggregate) (*Report, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("report: interval must be positive, got %v", interval)
	}
	r := &Report{Tags: append([]string(nil), tags...)}
	for bf := from; bf.Before(to); bf = bf.Add(interval) {
		bt := bf.Add(interval)
		if bt.After(to) {
			bt = to
		}
		b := Bucket{From: bf, To: bt, Values: make(map[string]float64)}
		// Query is inclusive [from,to]; timestamps are integer nanoseconds, so
		// ending one nanosecond before bt makes the bucket half-open [bf, bt) and
		// keeps a sample landing exactly on a boundary out of two buckets.
		qEnd := bt.Add(-time.Nanosecond)
		for _, tag := range tags {
			samples, err := h.Query(tag, bf, qEnd)
			if err != nil {
				return nil, err
			}
			if len(samples) == 0 {
				continue // empty bucket for this tag: omit the key
			}
			b.Values[tag] = aggregate(samples, a)
		}
		r.Buckets = append(r.Buckets, b)
	}
	return r, nil
}

// WriteCSV writes the report as CSV: a "time" + tag-names header, then one row
// per bucket keyed by the bucket start. Absent tag values render as empty cells.
func (r *Report) WriteCSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	header := append([]string{"time"}, r.Tags...)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, b := range r.Buckets {
		row := make([]string, 1+len(r.Tags))
		row[0] = b.From.Format(time.RFC3339Nano)
		for i, tag := range r.Tags {
			if v, ok := b.Values[tag]; ok {
				row[i+1] = strconv.FormatFloat(v, 'g', -1, 64)
			}
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteHTML writes the report as an HTML <table>: a "time" + tag-names header
// row, then one row per bucket keyed by the bucket start. Absent tag values
// render as empty cells.
func (r *Report) WriteHTML(w io.Writer) error {
	var b []byte
	b = append(b, "<table>\n<thead><tr><th>time</th>"...)
	for _, tag := range r.Tags {
		b = append(b, "<th>"...)
		b = append(b, html.EscapeString(tag)...)
		b = append(b, "</th>"...)
	}
	b = append(b, "</tr></thead>\n<tbody>\n"...)
	for _, bk := range r.Buckets {
		b = append(b, "<tr><td>"...)
		b = append(b, html.EscapeString(bk.From.Format(time.RFC3339Nano))...)
		b = append(b, "</td>"...)
		for _, tag := range r.Tags {
			b = append(b, "<td>"...)
			if v, ok := bk.Values[tag]; ok {
				b = append(b, strconv.FormatFloat(v, 'g', -1, 64)...)
			}
			b = append(b, "</td>"...)
		}
		b = append(b, "</tr>\n"...)
	}
	b = append(b, "</tbody>\n</table>\n"...)
	_, err := w.Write(b)
	return err
}
