package point_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/httpx"
	"github.com/uk0/silk/point"
)

func TestHTTPSourceOversizedPointKeepsLastGoodPayloadWithBadQuality(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 2 {
			fmt.Fprintf(w, "{\"label\":%q,\"DI\":true}", strings.Repeat("x", (1<<20)+1))
			return
		}
		fmt.Fprint(w, "{\"label\":\"ready\",\"DI\":false}")
	}))
	defer server.Close()
	// Force transport allowance above the scalar budget; route validation
	// must not be delegated to a response byte cap or an optional store.
	client, err := httpx.NewClient(httpx.Options{MaxResponseBytes: 2 << 20})
	if err != nil {
		t.Fatal(err)
	}
	tags := core.NewTagDB()
	src, err := point.NewHTTPSource(client, httpx.Request{URL: server.URL}, []point.Mapping{
		{Name: "TEXT", Kind: point.String, Pointer: "/label"},
		{Name: "DI11", Kind: point.Bool, Pointer: "/DI"},
	}, tags, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := src.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	err = src.Poll(ctx)
	tag, _ := tags.Get("TEXT")
	v := tag.Value()
	if err == nil || v.Raw != "ready" || v.Quality != core.QualityBad {
		t.Errorf("oversized field accepted or cleared: rejected=%v oldPayload=%v bad=%v", err != nil, v.Raw == "ready", v.Quality == core.QualityBad)
	}
	di, _ := tags.Get("DI11")
	v = di.Value()
	if v.Raw != true || v.Quality != core.QualityGood || calls.Load() != 2 {
		t.Fatal("valid sibling did not update from same response")
	}
}
