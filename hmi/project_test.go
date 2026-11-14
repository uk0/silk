package hmi

import (
	"context"
	"encoding/json"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/point"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestProjectRoundtripReferencesAndLimits(t *testing.T) {
	p := Template()
	b, e := p.JSON()
	if e != nil {
		t.Fatal(e)
	}
	q, e := Parse(b)
	if e != nil || len(q.Nodes) != 4 || q.Points[0].Kind != point.Bool {
		t.Fatal(q, e)
	}
	path := filepath.Join(t.TempDir(), "project.silkhmi")
	if e = Save(path, p); e != nil {
		t.Fatal(e)
	}
	if e = Save(path, p); e != nil {
		t.Fatal("overwrite", e)
	}
	q, e = Load(path)
	if e != nil || q.Title != p.Title {
		t.Fatal(e)
	}
	for _, mut := range []func(*Project){func(p *Project) { p.Nodes[0].Tag = "missing" }, func(p *Project) { p.Links[0].To.Port = "wrong" }, func(p *Project) { p.Nodes[0].Width = 1e9 }, func(p *Project) { p.Points = append(p.Points, p.Points[0]) }, func(p *Project) { p.Nodes[0].Colors = []ColorRule{{When: "x>0", Color: "#zzzzzz"}} }} {
		q, _ = Parse(b)
		mut(&q)
		if q.Validate() == nil {
			t.Fatal("invalid project accepted")
		}
	}
	if _, e = Parse(append(b, []byte(" {}")...)); e == nil {
		t.Fatal("trailing data accepted")
	}
	a := Port(p.Nodes[0], "east")
	z := Port(p.Nodes[1], "west")
	route := Route(a, z, nil)
	for i := 1; i < len(route); i++ {
		if route[i].X != route[i-1].X && route[i].Y != route[i-1].Y {
			t.Fatal("non orthogonal route")
		}
	}
}
func TestRuntimeHTTPQualityRecoveryStalenessAndStop(t *testing.T) {
	var mode atomic.Int32
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if mode.Load() == 1 {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": "92.8"})
	}))
	defer server.Close()
	p := Template()
	p.Points[1].StaleMS = 100
	p.Sources = []Source{{ID: "http", URL: server.URL, Method: "GET", IntervalMS: 100, TimeoutMS: 500, Enabled: true, Mappings: []point.Mapping{{Name: "AI101", Pointer: "/value", Kind: point.Float}}}}
	r, e := NewRuntime(context.Background(), p, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	tag, _ := r.Tags.Get("AI101")
	if tag.Value().Quality != core.QualityUncertain || calls.Load() != 0 {
		t.Fatal("constructor performed I/O or looked live")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if e = r.Start(ctx); e != nil {
		t.Fatal(e)
	}
	defer r.Stop()
	await := func(test func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !test() {
			if time.Now().After(deadline) {
				t.Fatal("runtime observation timed out", r.Errors(), tag.Value())
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	await(func() bool { return tag.Value().Quality == core.QualityGood })
	mode.Store(1)
	await(func() bool { return tag.Value().Quality == core.QualityBad && len(r.Errors()) > 0 })
	if tag.Value().Float() != 92.8 {
		t.Fatal("failure made zero")
	}
	mode.Store(0)
	await(func() bool { return tag.Value().Quality == core.QualityGood && len(r.Errors()) == 0 })
	r.Stop()
	n := calls.Load()
	time.Sleep(120 * time.Millisecond)
	if calls.Load() != n {
		t.Fatal("source survived stop")
	}
	r.Tick(time.Now().Add(time.Second))
	if tag.Value().Quality != core.QualityUncertain {
		t.Fatal("staleness missing")
	}
	if r.Start(ctx) == nil {
		t.Fatal("closed runtime restarted")
	}
}

func TestProjectRejectsCredentialsMalformedHeadersAndDuplicateMappings(t *testing.T) {
	for _, headers := range []map[string]string{{"Authorization": "secret"}, {"cookie": "secret"}, {"bad name": "v"}, {"X-Test": "v\r\nx:y"}} {
		p := Template()
		p.Sources = []Source{{ID: "s", URL: "http://127.0.0.1:1", Method: "GET", IntervalMS: 1000, TimeoutMS: 1000, Headers: headers, Mappings: []point.Mapping{{Name: "AI101", Kind: point.Float}}}}
		if p.Validate() == nil {
			t.Fatal("invalid header accepted")
		}
	}
	p := Template()
	p.Sources = []Source{{ID: "s", URL: "http://127.0.0.1:1", Method: "GET", IntervalMS: 1000, TimeoutMS: 1000, Mappings: []point.Mapping{{Name: "AI101", Kind: point.Float}, {Name: "AI101", Kind: point.Float}}}}
	if p.Validate() == nil {
		t.Fatal("duplicate disabled source mapping accepted")
	}
}
