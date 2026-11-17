package gui

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/httpx"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/point"
	pointsql "github.com/uk0/silk/point/sqlite"
)

func TestProcessIntegerHTTPMappingDurabilityColorsAndRecovery(t *testing.T) {
	if !pointsql.StoreAvailable() {
		t.Skip("SQLite persistence requires cgo")
	}
	isolateTagBridge(t)
	var mode, calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.Header.Get("X-Device") != "R001" || string(body) != `{"device":"R001"}` {
			t.Error("request configuration changed", r.Method, r.Header, string(body))
		}
		switch mode.Load() {
		case 1:
			io.WriteString(w, `{"data":{"word":8,"counter":9223372036854775807}}`)
		case 2:
			io.WriteString(w, `{"data":{"word":1,"counter":9007199254740992}}`)
		default:
			io.WriteString(w, `{"data":{"word":165,"counter":9007199254740993}}`)
		}
	}))
	defer server.Close()
	p := hmi.Project{Version: hmi.Version, Title: "Exact status words", Width: 900, Height: 360,
		Points: []hmi.Point{
			{Definition: point.Definition{Name: "BI_COUNTER", Kind: point.Int, Category: "BI"}, Initial: int64(0)},
			{Definition: point.Definition{Name: "BI_WORD", Kind: point.Int, Category: "BI"}, Initial: int64(0)},
			{Definition: point.Definition{Name: "DI_RUN", Kind: point.Bool, Category: "DI"}, Initial: false},
		}, Nodes: []hmi.Node{
			{ID: "counter", Kind: "value", Label: "Counter", X: 20, Y: 20, Width: 450, Height: 100, Tag: "BI_COUNTER", Unit: "pcs", Steps: []point.Step{{Expr: "x-2"}}, Colors: []hmi.ColorRule{{When: "x > 9007199254740992", Color: "#ff0000"}, {When: "true", Color: "#00aa00"}}},
			{ID: "word", Kind: "value", Label: "Upper nibble", X: 20, Y: 160, Width: 300, Height: 100, Tag: "BI_WORD", Steps: []point.Step{{Expr: "(x >> 4) & 0xf"}}},
			{ID: "pump", Kind: "pump", Label: "Pump", X: 520, Y: 20, Width: 180, Height: 160, Tag: "DI_RUN"},
		}, Sources: []hmi.Source{{ID: "status", URL: server.URL, Method: "POST", Headers: map[string]string{"X-Device": "R001"}, Body: `{"device":"R001"}`, IntervalMS: 1000, TimeoutMS: 1000, Enabled: true, RepeatWrites: true, Mappings: []point.Mapping{
			{Name: "BI_COUNTER", Pointer: "/data/counter", Kind: point.Int, Steps: []point.Step{{Expr: "x+2"}}},
			{Name: "BI_WORD", Pointer: "/data/word", Kind: point.Int},
			{Name: "DI_RUN", Pointer: "/data/word", Kind: point.Bool, Steps: []point.Step{{Expr: "(x & 1) != 0"}}},
		}}},
	}
	view := NewProcessDiagram()
	defer view.Close()
	if err := view.SetProject(p); err != nil {
		t.Fatal(err)
	}
	before := view.ProjectJSON()
	// Authored snapshot parsing must preserve the expressions without acquisition.
	copy, err := hmi.Parse([]byte(before))
	if err != nil || copy.Sources[0].Mappings[0].Steps[0].Expr != "x+2" || calls.Load() != 0 {
		t.Fatal("staging was not lossless/inert", err, calls.Load())
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "points.db")
	store, err := pointsql.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, d := range p.Points {
		if err := store.Define(ctx, d.Definition); err != nil {
			t.Fatal(err)
		}
	}
	db := core.NewTagDB()
	for _, d := range p.Points {
		db.GetOrCreate(d.Name, core.Meta{}).Observe(core.Value{Raw: d.Initial, Quality: core.QualityUncertain, Time: time.Now()})
	}
	cancel, err := view.BindTags(db)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	historyFrom := time.Now().Add(-time.Second)
	s := p.Sources[0]
	source, err := point.NewHTTPSource(nil, httpx.Request{URL: s.URL, Method: s.Method, Header: http.Header{"X-Device": {"R001"}}, Body: []byte(s.Body)}, s.Mappings, db, store)
	if err != nil {
		t.Fatal(err)
	}
	poll := func(wantError bool) {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- source.Poll(ctx) }()
		select {
		case err := <-done:
			if (err != nil) != wantError {
				t.Fatal("poll result", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("poll did not finish")
		}
		drainUITasks()
	}
	counter := view.views["counter"].widget.(*ProcessValue)
	word := view.views["word"].widget.(*ProcessValue)
	counter.SetPrecision(0)
	poll(false)
	if v, ok := counter.IntegerValue(); !ok || v != 9007199254740993 || counter.DisplayText() != "9007199254740993 pcs" || counter.DisplayColor() != (paint.Color{R: 255, A: 255}) {
		t.Fatal("HTTP-to-card integer/color lost", v, ok, counter.DisplayText(), counter.BindingError())
	}
	if v, ok := word.IntegerValue(); !ok || v != 10 {
		t.Fatal("bit extraction lost", v, ok)
	}
	mode.Store(1)
	poll(true)
	if _, ok := counter.IntegerValue(); ok || counter.DisplayText() != "—" {
		t.Fatal("overflow presented a valid counter", counter.DisplayText())
	}
	di, _ := db.Get("DI_RUN")
	if di.Value().Quality != core.QualityGood || di.Value().Raw != false {
		t.Fatal("overflow poisoned valid sibling", di.Value())
	}
	reader, err := pointsql.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	stored, err := reader.Latest(ctx, "BI_COUNTER")
	if err != nil || stored.Value.Quality != core.QualityBad || stored.Value.Raw != int64(9007199254740995) {
		t.Fatal("independent bad-quality readback", stored, err)
	}
	mode.Store(2)
	poll(false)
	if v, ok := counter.IntegerValue(); !ok || v != 1<<53 || counter.DisplayColor() != (paint.Color{G: 170, A: 255}) {
		t.Fatal("integer/color recovery failed", v, ok, counter.DisplayColor(), counter.BindingError())
	}
	history, err := reader.History(ctx, "BI_COUNTER", historyFrom, time.Now().Add(time.Hour), 10)
	if err != nil || len(history) != 3 || history[0].Value.Raw != int64(9007199254740995) || history[1].Value.Quality != core.QualityBad || history[2].Value.Raw != int64(9007199254740994) {
		t.Fatal("integer history changed", history, err)
	}
	if view.ProjectJSON() != before || calls.Load() != 3 || math.IsNaN(counter.Fraction()) {
		t.Fatal("runtime modified authored state, duplicated HTTP or invalid geometry", calls.Load())
	}
}
