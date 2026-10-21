package diagnostic

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestErrorPreservesCauseAndLocation(t *testing.T) {
	e := New(FileRead, "read source", os.ErrNotExist).At(Location{File: `C:\work\中文.go`, Line: 2, Column: 5, Offset: 12})
	if !errors.Is(e, os.ErrNotExist) || CodeOf(e) != FileRead {
		t.Fatal(e)
	}
	if !strings.Contains(e.Error(), `C:\work\中文.go:2:5: [SILK-FS001]`) {
		t.Fatal(e)
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Error
	if err = json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Error() != e.Error() || decoded.Hint == "" {
		t.Fatalf("json lost diagnostic: %s", b)
	}
	if e.Location.Line != 2 || e.At(Location{Line: 8}).Location.Line != 8 {
		t.Fatal("At mutated original")
	}
}
func TestCatalogStableCopiesAndFallback(t *testing.T) {
	entries := Catalog()
	seen := map[Code]bool{}
	for _, d := range entries {
		if seen[d.Code] || d.Summary == "" || d.Hint == "" {
			t.Fatal(d)
		}
		seen[d.Code] = true
		if got, ok := Lookup(d.Code); !ok || got != d {
			t.Fatal(d)
		}
	}
	first := entries[0]
	entries[0].Hint = "changed"
	if d, _ := Lookup(first.Code); d != first {
		t.Fatal("catalog exposed mutable data")
	}
	if New("unknown", "", nil).Code != Internal || CodeOf(nil) != "" {
		t.Fatal("fallback")
	}
}
