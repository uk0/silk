package hmiapp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/point"
	pointsql "github.com/uk0/silk/point/sqlite"
)

func TestRuntimeViewShowsLiveHTTPDiagnosticsWithoutSnapshot(t *testing.T) {
	if !pointsql.StoreAvailable() {
		t.Skip("SQLite backend unavailable")
	}
	before := i18n.Locale()
	i18n.SetLocale("en")
	t.Cleanup(func() { i18n.SetLocale(before) })
	var healthy atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "{\"value\":42.5}")
	}))
	t.Cleanup(server.Close)
	project := runtimeViewProject()
	project.Sources = []hmi.Source{{ID: "storage", URL: server.URL, Method: http.MethodGet,
		Enabled: true, IntervalMS: 100, TimeoutMS: 500,
		Mappings: []point.Mapping{{Name: "AI11", Pointer: "/value", Kind: point.Float}}}}
	v, posts := runtimeViewTest(t, RuntimeViewOptions{})
	if err := v.SetSnapshot(project, filepath.Join(t.TempDir(), "live.sqlite")); err != nil {
		t.Fatal(err)
	}
	v.SetSourceLabel("live-demo.silkhmi")
	v.Start()
	pumpRuntimeView(t, posts, func() bool { return v.Runtime() != nil })
	base := "live-demo.silkhmi · Running frozen source snapshot"
	pumpRuntimeView(t, posts, func() bool { return strings.Contains(v.Runtime().Diagnostics().Sources["storage"], "503") })
	pumpRuntimeView(t, posts, func() bool { return v.status.Text() == "1 source error · "+base })
	healthy.Store(true)
	pumpRuntimeView(t, posts, func() bool {
		x, e := v.Runtime().PointState("AI11")
		return e == nil && x.Value.Raw == 42.5 && v.status.Text() == base
	})
	healthy.Store(false)
	// Wait for the second error without delivering its pending view update.
	// Stop must retire it so an old diagnostic cannot overwrite the stopping or
	// stopped caption after its owning runtime has been revoked.
	pumpRuntimeView(t, posts, func() bool { return len(v.Runtime().Diagnostics().Sources) == 1 })
	v.Stop()
	pumpRuntimeView(t, posts, func() bool { return !v.Busy() })
	if got := v.status.Text(); strings.Contains(got, "source error") || got != "live-demo.silkhmi · Stopped; no acquisition" {
		t.Fatalf("retired diagnostic rewrote the status: %q", got)
	}
}
