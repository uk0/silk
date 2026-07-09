package script

import (
	"testing"

	"github.com/uk0/silk/core"
)

// TestEngineReadWriteTags runs a script that reads one tag and writes a derived
// one — the core "logic without recompile" use case.
func TestEngineReadWriteTags(t *testing.T) {
	db := core.NewTagDB()
	db.GetOrCreate("a", core.Meta{}).SetValue(10.0)

	e := NewEngine(db)
	if err := e.Run(`silk.SetTag("b", silk.GetTag("a")*2)`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := db.GetOrCreate("b", core.Meta{}).Value().Float(); got != 20 {
		t.Errorf("b = %v, want 20", got)
	}
}

// TestEngineBoolTags covers the boolean tag helpers.
func TestEngineBoolTags(t *testing.T) {
	db := core.NewTagDB()
	db.GetOrCreate("run", core.Meta{}).SetValue(true)

	e := NewEngine(db)
	if err := e.Run(`silk.SetTagBool("stopped", !silk.GetTagBool("run"))`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if db.GetOrCreate("stopped", core.Meta{}).Value().Bool() {
		t.Error("stopped should be false when run is true")
	}
}

// TestEngineRunOnTagChange verifies a script re-runs on every tag change,
// driving a derived tag reactively.
func TestEngineRunOnTagChange(t *testing.T) {
	db := core.NewTagDB()
	e := NewEngine(db)

	cancel := e.RunOnTagChange("temp", `silk.SetTag("tempF", silk.GetTag("temp")*9/5+32)`)
	defer cancel()

	db.GetOrCreate("temp", core.Meta{}).SetValue(100.0)
	if got := db.GetOrCreate("tempF", core.Meta{}).Value().Float(); got != 212 {
		t.Errorf("tempF = %v, want 212", got)
	}

	db.GetOrCreate("temp", core.Meta{}).SetValue(0.0)
	if got := db.GetOrCreate("tempF", core.Meta{}).Value().Float(); got != 32 {
		t.Errorf("tempF after change = %v, want 32", got)
	}

	cancel()
	db.GetOrCreate("temp", core.Meta{}).SetValue(50.0)
	if got := db.GetOrCreate("tempF", core.Meta{}).Value().Float(); got != 32 {
		t.Errorf("tempF after cancel = %v, want unchanged 32", got)
	}
}

// TestEngineScriptErrorReturned confirms a bad script surfaces an error rather
// than panicking.
func TestEngineScriptErrorReturned(t *testing.T) {
	e := NewEngine(core.NewTagDB())
	if err := e.Run(`silk.NoSuchFunc()`); err == nil {
		t.Error("expected error for unknown symbol")
	}
}
