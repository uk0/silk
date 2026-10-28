package wmap_test

import (
	"testing"

	wmap "github.com/uk0/silk/mod/map"
)

// Use an external consumer package: this must resolve the actual public module
// identity, not merely type-check the two source files as command-line-arguments.
func TestPublicModuleSceneAssembly(t *testing.T) {
	v := wmap.NewMapView()
	s, ok := v.Scene().(*wmap.MapScene)
	if !ok || s.View() != v {
		t.Fatal("map view did not own its map scene")
	}
	if s.Width() != 200 || s.Height() != 200 || s.PropertyConfigName() != "map" || s.Title() != "平面图" {
		t.Fatal("map scene defaults changed")
	}
	if s.Self() != s || v.Self() != v {
		t.Fatal("embedded SDK identity was not preserved")
	}
}
