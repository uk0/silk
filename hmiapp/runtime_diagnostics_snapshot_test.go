package hmiapp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/point"
)

type snapshotRetentionStore struct{}

func (snapshotRetentionStore) Define(context.Context, point.Definition) error    { return nil }
func (snapshotRetentionStore) Publish(context.Context, string, core.Value) error { return nil }
func (snapshotRetentionStore) Prune(context.Context, time.Time, int) (int64, error) {
	return 0, fmt.Errorf("test storage maintenance failure")
}

func TestWorkbenchSnapshotSeparatesMaintenanceAndUsesRunningPoints(t *testing.T) {
	before := i18n.Locale()
	i18n.SetLocale("en")
	t.Cleanup(func() { i18n.SetLocale(before) })
	var healthy atomic.Bool
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "{\"value\": 42.5}")
	}))
	t.Cleanup(httpServer.Close)
	p := hmi.Template()
	p.RetentionDays = 1
	p.Sources = []hmi.Source{{ID: "storage", URL: httpServer.URL, Method: http.MethodGet,
		Enabled: true, IntervalMS: 100, TimeoutMS: 500,
		Mappings: []point.Mapping{{Name: "AI101", Pointer: "/value", Kind: point.Float}}}}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	rt, err := hmi.NewRuntime(ctx, p, snapshotRetentionStore{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer rt.Stop()

	// The editor can hold a newer draft than the active runtime. The
	// workbench must never dereference a missing draft-only runtime tag.
	p.Points = append(p.Points, hmi.Point{Definition: point.Definition{Name: "draft-only", Kind: point.Float}})
	draft, err := p.JSON()
	if err != nil {
		t.Fatal(err)
	}
	f, err := New(Config{ProjectJSON: string(draft)})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Join()
	defer f.Close()
	s := &session{runtime: rt, cancel: cancel, done: make(chan struct{})}
	close(s.done)
	f.session = s

	await := func(condition func(hmi.RuntimeDiagnostics) bool) {
		t.Helper()
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for !condition(rt.Diagnostics()) {
			select {
			case <-ctx.Done():
				t.Fatalf("unexpected runtime diagnostics: %+v", rt.Diagnostics())
			case <-tick.C:
			}
		}
	}
	await(func(d hmi.RuntimeDiagnostics) bool {
		return strings.Contains(d.Sources["storage"], "503") && d.RetentionError != ""
	})
	f.Snapshot()
	if got := f.status.Text(); !strings.HasPrefix(got, "1 source error · storage maintenance error · running") {
		t.Fatalf("storage error incorrectly counted as a source fault: %q", got)
	}
	healthy.Store(true)
	await(func(d hmi.RuntimeDiagnostics) bool { return len(d.Sources) == 0 })
	f.Snapshot()
	if got := f.status.Text(); !strings.HasPrefix(got, "storage maintenance error · running") || !strings.HasSuffix(got, "0 source errors") {
		t.Fatalf("HTTP recovery cleared the maintenance warning: %q", got)
	}
}
