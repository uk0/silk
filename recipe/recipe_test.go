package recipe

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/uk0/silk/core"
)

func TestBookSaveLoad(t *testing.T) {
	b := &Book{}
	b.Add(Recipe{Name: "Cola", Values: map[string]float64{"sugar": 12.5, "water": 87.5}})
	b.Add(Recipe{Name: "Diet", Values: map[string]float64{"sugar": 0, "water": 100}})

	path := filepath.Join(t.TempDir(), "book.json")
	if err := b.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, b) {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, b)
	}
}

func TestBookOps(t *testing.T) {
	b := &Book{}
	b.Add(Recipe{Name: "A", Values: map[string]float64{"x": 1}})
	b.Add(Recipe{Name: "B", Values: map[string]float64{"x": 2}})

	if got := b.List(); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Fatalf("List = %v", got)
	}

	// Add replaces a same-name recipe in place instead of appending.
	b.Add(Recipe{Name: "A", Values: map[string]float64{"x": 9}})
	if r, ok := b.Get("A"); !ok || r.Values["x"] != 9 {
		t.Fatalf("Add did not replace: %+v ok=%v", r, ok)
	}
	if len(b.Recipes) != 2 {
		t.Fatalf("replace grew book to %d recipes", len(b.Recipes))
	}
	if _, ok := b.Get("missing"); ok {
		t.Fatalf("Get(missing) reported found")
	}

	if !b.Delete("A") {
		t.Fatalf("Delete(A) = false")
	}
	if b.Delete("A") {
		t.Fatalf("Delete(A) twice = true")
	}
	if got := b.List(); !reflect.DeepEqual(got, []string{"B"}) {
		t.Fatalf("after delete List = %v", got)
	}
}

func TestApply(t *testing.T) {
	db := core.NewTagDB()
	db.GetOrCreate("temp", core.Meta{}) // pre-existing tag
	r := Recipe{Name: "Warm", Values: map[string]float64{"temp": 75, "flow": 3.2}}

	Apply(r, db)

	for name, want := range r.Values {
		tg, ok := db.Get(name)
		if !ok {
			t.Fatalf("tag %q not present after Apply", name)
		}
		if got := tg.Value().Float(); got != want {
			t.Fatalf("tag %q = %v, want %v", name, got, want)
		}
	}
}

func TestCapture(t *testing.T) {
	db := core.NewTagDB()
	db.SetValue("temp", 42.0)
	db.SetValue("flow", 5.5)

	r := Capture("Snap", db, []string{"temp", "flow"})

	if r.Name != "Snap" {
		t.Fatalf("Name = %q, want Snap", r.Name)
	}
	want := map[string]float64{"temp": 42, "flow": 5.5}
	if !reflect.DeepEqual(r.Values, want) {
		t.Fatalf("Values = %v, want %v", r.Values, want)
	}
}
