package hmi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/point"
)

type retentionFaultStore struct {
	err error
}

func (*retentionFaultStore) Define(context.Context, point.Definition) error    { return nil }
func (*retentionFaultStore) Publish(context.Context, string, core.Value) error { return nil }
func (s *retentionFaultStore) Prune(context.Context, time.Time, int) (int64, error) {
	return 0, s.err
}

func TestStorageNamedSourceAndRetentionKeepIndependentDiagnostics(t *testing.T) {
	fault := errors.New("retention backend unavailable")
	releaseRequest := make(chan struct{})
	var recoverSource atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/healthy" {
			_, _ = io.WriteString(w, "{\"value\":true}")
			return
		}
		select {
		case <-req.Context().Done():
			return
		case <-releaseRequest:
		}
		if !recoverSource.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "{\"value\":42.5}")
	}))
	t.Cleanup(srv.Close)
	p := Template()
	p.RetentionDays = 1
	p.Sources = []Source{
		{ID: "storage", URL: srv.URL, Method: http.MethodGet, IntervalMS: 100, TimeoutMS: 4000, Enabled: true,
			Mappings: []point.Mapping{{Name: "AI101", Pointer: "/value", Kind: point.Float}}},
		{ID: "storage (retention)", URL: srv.URL + "/healthy", Method: http.MethodGet, IntervalMS: 100, TimeoutMS: 4000, Enabled: true,
			Mappings: []point.Mapping{{Name: "DI11", Pointer: "/value", Kind: point.Bool}}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	r, err := NewRuntime(ctx, p, &retentionFaultStore{fault}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	await := func(check func(map[string]string) bool) {
		t.Helper()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for !check(r.Errors()) {
			select {
			case <-ctx.Done():
				t.Fatalf("diagnostics did not reach expected state: %v", r.Errors())
			case <-ticker.C:
			}
		}
	}
	await(func(state map[string]string) bool {
		for _, value := range state {
			if value == fault.Error() {
				return true
			}
		}
		return false
	})
	close(releaseRequest)
	await(func(state map[string]string) bool { return strings.Contains(state["storage"], "503") })
	snapshot := r.Errors()
	if len(snapshot) != 2 || snapshot["storage (retention 2)"] != fault.Error() {
		t.Fatalf("retention and HTTP error must coexist without overwriting a configured source ID: %v", snapshot)
	}
	two := r.Diagnostics()
	if two.RetentionError != fault.Error() || two.Sources["storage"] != snapshot["storage"] || len(two.Sources) != 1 {
		t.Fatalf("diagnostics did not separate fault categories: %+v", two)
	}
	flat := two.ErrorMap()
	delete(flat, "storage")
	if flat["storage (retention 2)"] != fault.Error() || two.Sources["storage"] != snapshot["storage"] || len(two.Sources) != 1 {
		t.Fatalf("flattening a snapshot changed its categorized fields: %+v", two)
	}
	delete(two.Sources, "storage")
	if !strings.Contains(r.Diagnostics().Sources["storage"], "503") {
		t.Fatal("mutating a returned snapshot affected the runtime")
	}
	recoverSource.Store(true)
	await(func(state map[string]string) bool { return state["storage"] == "" })
	snapshot = r.Errors()
	if len(snapshot) != 1 || snapshot["storage (retention 2)"] != fault.Error() {
		t.Fatalf("polling recovery removed unrelated retention error: %v", snapshot)
	}
	sample, err := r.PointState("AI101")
	if err != nil || sample.Value.Quality != core.QualityGood || sample.Value.Float() != 42.5 {
		t.Fatalf("expected Good recovery sample, got %+v, %v", sample, err)
	}
}

func TestRetentionRecoveryLeavesSourceFault(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := Template()
		p.RetentionDays = 1
		p.Sources = []Source{{ID: "storage", URL: "http://127.0.0.1:1", Method: http.MethodGet,
			IntervalMS: 100, TimeoutMS: 500, Enabled: true, Mappings: []point.Mapping{{Name: "AI101", Kind: point.Float}}}}
		store := new(retentionRecorder)
		r, err := NewRuntime(context.Background(), p, store, nil)
		if err != nil {
			t.Fatal(err)
		}
		const sourceFault = "source unavailable"
		r.sourceErrors["storage"] = sourceFault
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			r.runRetention(ctx, store)
		}()
		defer func() { cancel(); <-done }()
		synctest.Wait()
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()
		first := r.Diagnostics()
		if first.Sources["storage"] != sourceFault || first.RetentionError != context.DeadlineExceeded.Error() {
			t.Fatalf("retention failure overwrote the source fault: %+v", first)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		second := r.Diagnostics()
		if second.Sources["storage"] != sourceFault || second.RetentionError != "" || len(r.Errors()) != 1 {
			t.Fatalf("retention recovery removed the source fault: %+v", second)
		}
	})
}

func TestInactiveSourceKeepsLegacyStorageDiagnosticKey(t *testing.T) {
	sources := []Source{{ID: "storage"}, {ID: "storage (retention)"}}
	if key := retentionDiagnosticKey(sources); key != "storage" {
		t.Fatalf("disabled source changed legacy retention key: %q", key)
	}
	sources[0].Enabled = true
	if key := retentionDiagnosticKey(sources); key != "storage (retention)" {
		t.Fatalf("disabled fallback source incorrectly took the key: %q", key)
	}
}
