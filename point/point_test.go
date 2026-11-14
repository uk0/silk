package point_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/httpx"
	. "github.com/uk0/silk/point"
	pointsql "github.com/uk0/silk/point/sqlite"
)

func TestStrictConversionsAndBoundedExpressions(t *testing.T) {
	for _, tc := range []struct {
		v    any
		k    Kind
		want any
	}{{" 92.8 ", Float, 92.8}, {"9223372036854775807", Int, int64(math.MaxInt64)}, {json.Number("-9223372036854775808"), Int, int64(math.MinInt64)}, {"false", Bool, false}, {1, Bool, true}, {true, Float, 1.0}, {int64(42), String, "42"}} {
		v, e := Convert(tc.v, tc.k)
		if e != nil || v != tc.want {
			t.Fatalf("%v %s: %v %v", tc.v, tc.k, v, e)
		}
	}
	for _, tc := range []struct {
		v any
		k Kind
	}{{"broken", Float}, {"-9223372036854775808.9", Int}, {json.Number("9007199254740993.1"), Int}, {math.NaN(), Float}, {math.Inf(1), String}, {1.5, Int}, {"9223372036854775808", Int}, {"-9223372036854775809", Int}, {json.Number("-9223372036854775809"), Int}, {2, Bool}, {"yes", Bool}, {nil, Float}, {int64(1 << 54), Float}, {int(1 << 54), Float}} {
		if v, e := Convert(tc.v, tc.k); e == nil {
			t.Fatalf("invalid conversion accepted %v -> %v", tc, v)
		}
	}
	for _, source := range []string{"x*0.1+2", "x >= 80 && x < 100", "false && 1/0>0", "true || 1/0>0", "x == 928"} {
		p, e := Compile(source)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = p.Evaluate(928.0); e != nil {
			t.Fatal(source, e)
		}
	}
	for _, source := range []string{"os.Exit(1)", "x[0]", "make([]int,5)", "y+1", "x%2", strings.Repeat("x+", 130) + "1", strings.Repeat("x", 1025)} {
		if _, e := Compile(source); e == nil {
			t.Fatal("unsafe/unbounded expression", source)
		}
	}
	for _, source := range []string{"x/0", "x*1e308", "!x", "x && true"} {
		p, e := Compile(source)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = p.Evaluate(92.8); e == nil {
			t.Fatal("invalid runtime expression", source)
		}
	}
	p, e := ParsePipeline(`[{"type":"float"},{"expr":"x*0.1"},{"expr":"x>=80"}]`)
	if e != nil {
		t.Fatal(e)
	}
	v, e := p.Apply("928")
	if e != nil || v != true {
		t.Fatal(v, e)
	}
	for _, s := range []string{`[{"type":"unknown"}]`, `[{"expr":"x","type":"float"}]`, `[{"typo":"float"}]`, `[] []`, `{}`} {
		if _, e := ParsePipeline(s); e == nil {
			t.Fatal("invalid pipeline", s)
		}
	}
}
func TestStoreTypedPersistenceQualityHistoryAndRollback(t *testing.T) {
	if !pointsql.StoreAvailable() {
		t.Skip("SQLite storage requires cgo")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "points.db")
	s, e := pointsql.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, d := range []Definition{{"DI11", Bool, "DI", "", "valve"}, {"AI101", Float, "AI", "°C", "temperature"}, {"counter", Int, "BI", "", ""}, {"label", String, "", "", ""}} {
		if e = s.Define(ctx, d); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.Define(ctx, Definition{Name: "DI11", Kind: Float}); e == nil {
		t.Fatal("changed type")
	}
	for name, raw := range map[string]any{"DI11": true, "AI101": 92.8, "counter": int64(math.MaxInt64), "label": "运行"} {
		if e = s.Publish(ctx, name, core.Value{Raw: raw, Quality: core.QualityGood, Time: now}); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.Publish(ctx, "AI101", core.Value{Raw: "oops", Quality: core.QualityGood, Time: now}); e == nil {
		t.Fatal("malformed number accepted")
	}
	v, e := s.Latest(ctx, "AI101")
	if e != nil || v.Value.Raw != 92.8 {
		t.Fatal("failed publish changed latest", v, e)
	}
	if e = s.Publish(ctx, "AI101", core.Value{Raw: 92.8, Quality: core.QualityBad, Time: now.Add(time.Second)}); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = pointsql.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	v, e = s.Latest(ctx, "counter")
	if e != nil || v.Value.Raw != int64(math.MaxInt64) {
		t.Fatal("int64 precision lost", v, e)
	}
	history, e := s.History(ctx, "AI101", now.Add(-time.Second), now.Add(2*time.Second), 100)
	if e != nil || len(history) != 2 || history[1].Value.Quality != core.QualityBad || !history[0].Value.Time.Equal(now) {
		t.Fatal(history, e)
	}
	defs, e := s.Definitions(ctx)
	if e != nil || len(defs) != 4 {
		t.Fatal(defs, e)
	}
	if e = s.Publish(ctx, "missing", core.Value{Raw: 1., Quality: core.QualityGood, Time: now}); e == nil {
		t.Fatal("undeclared point accepted")
	}
	if e = s.Publish(ctx, "AI101", core.Value{Raw: 1., Quality: core.QualityGood}); e == nil {
		t.Fatal("missing time")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if e := s.Publish(ctx, "AI101", core.Value{Raw: float64(i), Quality: core.QualityGood, Time: now.Add(3 * time.Second)}); e != nil {
				t.Error(e)
			}
		}(i)
	}
	wg.Wait()
}
func TestHTTPSourceActualTransportMappingFailureAndPersistence(t *testing.T) {
	if !pointsql.StoreAvailable() {
		t.Skip("SQLite storage requires cgo")
	}
	var calls atomic.Int32
	var mode atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.Header.Get("X-Test") != "runtime" {
			t.Error("request config lost")
		}
		var body map[string]string
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil || body["device"] != "R001" {
			t.Error("request body lost", e)
		}
		switch mode.Load() {
		case 1:
			w.WriteHeader(503)
			w.Write([]byte(`{"error":"down"}`))
		case 2:
			w.Write([]byte(`{"data":{"temp":"broken","open":false}}`))
		default:
			w.Write([]byte(`{"data":{"temp":"928","open":true}}`))
		}
	}))
	defer server.Close()
	db := core.NewTagDB()
	s, e := pointsql.Open(filepath.Join(t.TempDir(), "points.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for _, d := range []Definition{{Name: "AI101", Kind: Float}, {Name: "DI11", Kind: Bool}} {
		if e = s.Define(context.Background(), d); e != nil {
			t.Fatal(e)
		}
	}
	req := httpx.Request{URL: server.URL, Method: "POST", Header: http.Header{"X-Test": {"runtime"}}, Body: []byte(`{"device":"R001"}`)}
	source, e := NewHTTPSource(nil, req, []Mapping{{Name: "AI101", Pointer: "/data/temp", Kind: Float, Steps: []Step{{Type: Float}, {Expr: "x*0.1"}}}, {Name: "DI11", Pointer: "/data/open", Kind: Bool}}, db, s)
	if e != nil {
		t.Fatal(e)
	}
	req.Body[0] = 'x'
	req.Header.Set("X-Test", "mutated") // construction snapshots config
	if e = source.Poll(context.Background()); e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 1 {
		t.Fatal("per-widget HTTP requests", calls.Load())
	}
	temp, _ := db.Get("AI101")
	if math.Abs(temp.Value().Float()-92.8) > 1e-10 {
		t.Fatal(temp.Value())
	}
	first := temp.Value().Time
	if e = source.Poll(context.Background()); e != nil {
		t.Fatal(e)
	}
	if !temp.Value().Time.After(first) {
		t.Fatal("unchanged acquisition did not refresh timestamp")
	}
	mode.Store(1)
	if e = source.Poll(context.Background()); e == nil {
		t.Fatal("503 hidden")
	}
	if temp.Value().Quality != core.QualityBad || math.Abs(temp.Value().Float()-92.8) > 1e-10 {
		t.Fatal("failed request manufactured zero", temp.Value())
	}
	mode.Store(2)
	if e = source.Poll(context.Background()); e == nil {
		t.Fatal("parse error hidden")
	}
	di, _ := db.Get("DI11")
	if di.Value().Quality != core.QualityGood || di.Value().Raw != false {
		t.Fatal("valid sibling field lost")
	}
	mode.Store(0)
	if e = source.Poll(context.Background()); e != nil || temp.Value().Quality != core.QualityGood {
		t.Fatal("recovery", e)
	}
	stored, e := s.Latest(context.Background(), "AI101")
	if e != nil || stored.Value.Raw != temp.Value().Raw || stored.Value.Quality != temp.Value().Quality || !stored.Value.Time.Equal(temp.Value().Time) {
		t.Fatal("persist before live failed", stored, e)
	}
	before := calls.Load()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e = source.Poll(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if calls.Load() != before {
		t.Fatal("cancelled request sent")
	}
	if e = source.Run(context.Background(), time.Second, false, nil); e == nil {
		t.Fatal("POST repeated without explicit consent")
	}
}
func TestHTTPSourceRunCancellationJoinsRequest(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }))
	defer server.Close()
	source, e := NewHTTPSource(nil, httpx.Request{URL: server.URL}, []Mapping{{Name: "AI1", Kind: Float}}, core.NewTagDB(), nil)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- source.Run(ctx, 100*time.Millisecond, false, nil) }()
	<-entered
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request not joined")
	}
}
