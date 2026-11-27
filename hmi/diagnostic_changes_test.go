package hmi

import (
	"context"
	"errors"
	"testing"
)

func TestDiagnosticObserversOnlyNotifyOnTransitionsAndRetire(t *testing.T) {
	r, err := NewRuntime(context.Background(), Template(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	var got []RuntimeDiagnostics
	cancel := r.SubscribeDiagnosticChanges(func() { got = append(got, r.Diagnostics()) })
	fault := errors.New("HTTP unavailable")
	r.setSourceDiagnostic("storage", nil)
	r.setSourceDiagnostic("storage", fault)
	r.setSourceDiagnostic("storage", fault)
	r.setRetentionDiagnostic(errors.New("prune failed"))
	r.setRetentionDiagnostic(errors.New("prune failed"))
	r.setSourceDiagnostic("storage", nil)
	r.setSourceDiagnostic("storage", nil)
	r.setRetentionDiagnostic(nil)
	r.setRetentionDiagnostic(nil)
	if len(got) != 4 {
		t.Fatalf("changes = %v; repeated attempts must stay quiet", got)
	}
	if got[0].Sources["storage"] != "HTTP unavailable" || got[0].RetentionError != "" ||
		got[1].Sources["storage"] != "HTTP unavailable" || got[1].RetentionError != "prune failed" ||
		len(got[2].Sources) != 0 || got[2].RetentionError != "prune failed" ||
		len(got[3].Sources) != 0 || got[3].RetentionError != "" {
		t.Fatalf("wrong fault transitions: %v", got)
	}
	cancel()
	cancel()
	r.setSourceDiagnostic("storage", fault)
	if len(got) != 4 || r.Diagnostics().Sources["storage"] == "" {
		t.Fatalf("cancellation suppressed state or retained callback: %v", got)
	}
	r.SubscribeDiagnosticChanges(func() { t.Error("closed runtime delivered a diagnostic") })
	r.Stop()
	r.setRetentionDiagnostic(errors.New("late"))
	r.SubscribeDiagnosticChanges(func() { t.Error("accepted subscription after stop") })
	r.setSourceDiagnostic("storage", nil)
	if r.Diagnostics().RetentionError != "" {
		t.Fatal("late diagnostics mutated stopped runtime")
	}
}

func TestDiagnosticCancellationRetiresAlreadyQueuedFanout(t *testing.T) {
	r, err := NewRuntime(context.Background(), Template(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	cancel := r.SubscribeDiagnosticChanges(func() { t.Error("canceled fanout entered subscriber") })
	r.mu.Lock()
	pending := r.diagnosticFanoutLocked()
	r.mu.Unlock()
	cancel()
	notifyDiagnosticChanges(pending)
}
