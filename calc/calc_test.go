package calc

import (
	"testing"

	"github.com/uk0/silk/core"
)

func TestAverageCalcTag(t *testing.T) {
	db := core.NewTagDB()
	e := NewEngine(db)

	if err := e.Add(CalcTag{Output: "avg", Expr: "(a + b) / 2", Inputs: []string{"a", "b"}}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	avg, ok := db.Get("avg")
	if !ok {
		t.Fatal("output tag avg was not created")
	}

	db.SetValue("a", 10.0)
	db.SetValue("b", 20.0)
	if got := avg.Value().Float(); got != 15 {
		t.Fatalf("avg = %v, want 15", got)
	}

	db.SetValue("a", 0.0)
	if got := avg.Value().Float(); got != 10 {
		t.Fatalf("avg after a=0 = %v, want 10", got)
	}
}

func TestSingleInputCalcTag(t *testing.T) {
	db := core.NewTagDB()
	e := NewEngine(db)

	if err := e.Add(CalcTag{Output: "scaled", Expr: "level * 2.5", Inputs: []string{"level"}}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	scaled, ok := db.Get("scaled")
	if !ok {
		t.Fatal("output tag scaled was not created")
	}

	db.SetValue("level", 4.0)
	if got := scaled.Value().Float(); got != 10 {
		t.Fatalf("scaled = %v, want 10", got)
	}
}

func TestMalformedExprReturnsError(t *testing.T) {
	db := core.NewTagDB()
	e := NewEngine(db)
	if err := e.Add(CalcTag{Output: "bad", Expr: "(a + ", Inputs: []string{"a"}}); err == nil {
		t.Fatal("Add with malformed Expr: want error, got nil")
	}
}

func TestRemoveAllStopsUpdates(t *testing.T) {
	db := core.NewTagDB()
	e := NewEngine(db)
	if err := e.Add(CalcTag{Output: "avg", Expr: "(a + b) / 2", Inputs: []string{"a", "b"}}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	avg, _ := db.Get("avg")
	db.SetValue("a", 10.0)
	db.SetValue("b", 20.0)

	e.RemoveAll()
	db.SetValue("a", 100.0) // no longer subscribed, avg must stay frozen
	if got := avg.Value().Float(); got != 15 {
		t.Fatalf("avg after RemoveAll = %v, want 15 (frozen)", got)
	}
}
