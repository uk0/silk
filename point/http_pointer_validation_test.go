package point_test

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/httpx"
	"github.com/uk0/silk/point"
)

func TestHTTPSourceRejectsMalformedPointerBeforePolling(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte("{}"))
	}))
	defer server.Close()
	for _, p := range []string{"/data/bad~2", "/data/missing~", "data/value", "#/data", "/\xff", "/" + strings.Repeat("a", 4096)} {
		m := []point.Mapping{{Name: "AI11", Kind: point.Float, Pointer: p}}
		_, err := point.NewHTTPSource(nil, httpx.Request{Method: http.MethodPost, URL: server.URL, Body: []byte("{\"id\":\"AI11\"}")}, m, core.NewTagDB(), nil)
		if err == nil {
			t.Errorf("malformed HTTP mapping accepted: %q", p[:min(len(p), 40)])
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("config validation sent %d requests", hits.Load())
	}
}

func TestHTTPSourceEscapedResponseSelectionFeedsTypedPointFromSinglePost(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != "{\"line\":1}" || r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Error("configured POST method, body or content type lost")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Write([]byte("{\"line/1\":{\"~sensors\":[{\"raw\":\"928\",\"ready\":true}]}}"))
	}))
	defer server.Close()
	db := core.NewTagDB()
	root := "/line~11/~0sensors/0"
	m := []point.Mapping{
		{Name: "AI11", Kind: point.Float, Pointer: root + "/raw", Steps: []point.Step{{Type: point.Float}, {Expr: "x * 0.1"}}},
		{Name: "DI11", Kind: point.Bool, Pointer: root + "/ready"},
	}
	req := httpx.Request{Method: http.MethodPost, URL: server.URL, Body: []byte("{\"line\":1}"), Header: http.Header{"Content-Type": {"application/json"}}}
	src, err := point.NewHTTPSource(nil, req, m, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := src.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	value := db.GetOrCreate("AI11", core.Meta{}).Value()
	num, ok := value.Raw.(float64)
	if !ok || math.Abs(num-92.8) > 1e-9 || value.Quality != core.QualityGood {
		t.Fatalf("float point: %+v", value)
	}
	if got := db.GetOrCreate("DI11", core.Meta{}).Value(); got.Raw != true || got.Quality != core.QualityGood {
		t.Fatalf("digital point: %+v", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("poll sent %d requests for two mappings", hits.Load())
	}
}
