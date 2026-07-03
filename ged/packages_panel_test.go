package ged

import (
	"reflect"
	"testing"

	"github.com/uk0/silk/core"
)

// fixturePackages returns two GoListPackages covering the two shapes
// the panel cares about: one with both GoFiles and TestGoFiles, one
// with GoFiles only. Used by every test that drives the public API.
func fixturePackages() []core.GoListPackage {
	mod := &core.GoListModule{Path: "silk", Main: true, Dir: "/repo"}
	return []core.GoListPackage{
		{
			Dir:         "/repo/core",
			ImportPath:  "github.com/uk0/silk/core",
			Name:        "core",
			GoFiles:     []string{"a.go", "b.go"},
			TestGoFiles: []string{"a_test.go"},
			Module:      mod,
		},
		{
			Dir:        "/repo/geom",
			ImportPath: "github.com/uk0/silk/geom",
			Name:       "geom",
			GoFiles:    []string{"vec.go"},
			Module:     mod,
		},
	}
}

// TestSetPackagesAndAccessor verifies SetPackages stores the slice and
// Packages() returns the same set.
func TestSetPackagesAndAccessor(t *testing.T) {
	p := NewPackagesPanel()
	fx := fixturePackages()
	p.SetPackages(fx)

	got := p.Packages()
	if !reflect.DeepEqual(got, fx) {
		t.Fatalf("Packages() = %+v\nwant %+v", got, fx)
	}
}

// TestBuildPackagesRowsCollapsed verifies the pure row helper emits
// header-only output when nothing is expanded — one row per package,
// in input order.
func TestBuildPackagesRowsCollapsed(t *testing.T) {
	fx := fixturePackages()
	got := buildPackagesRows(fx, map[string]bool{})

	want := []packageRow{
		{Kind: packageRowHeader, PkgIdx: 0},
		{Kind: packageRowHeader, PkgIdx: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildPackagesRows(collapsed) = %+v\nwant %+v", got, want)
	}
}

// TestBuildPackagesRowsExpanded verifies an expanded package contributes
// its GoFiles (in order) then its TestGoFiles (in order), interleaved
// with the next package's collapsed header.
func TestBuildPackagesRowsExpanded(t *testing.T) {
	fx := fixturePackages()
	exp := map[string]bool{"github.com/uk0/silk/core": true}
	got := buildPackagesRows(fx, exp)

	want := []packageRow{
		{Kind: packageRowHeader, PkgIdx: 0},
		{Kind: packageRowFile, PkgIdx: 0, File: "a.go", IsTest: false},
		{Kind: packageRowFile, PkgIdx: 0, File: "b.go", IsTest: false},
		{Kind: packageRowFile, PkgIdx: 0, File: "a_test.go", IsTest: true},
		{Kind: packageRowHeader, PkgIdx: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildPackagesRows(expanded) = %+v\nwant %+v", got, want)
	}
}

// TestPackagesTotalFileCount verifies the header tally sums GoFiles +
// TestGoFiles across all packages.
func TestPackagesTotalFileCount(t *testing.T) {
	if got, want := totalFileCount(fixturePackages()), 4; got != want {
		t.Fatalf("totalFileCount = %d, want %d", got, want)
	}
	if got := totalFileCount(nil); got != 0 {
		t.Fatalf("totalFileCount(nil) = %d, want 0", got)
	}
}

// TestPackagesToggleExpanded verifies the internal toggle flips
// expansion state for an ImportPath and is observable via IsExpanded.
func TestPackagesToggleExpanded(t *testing.T) {
	p := NewPackagesPanel()
	p.SetPackages(fixturePackages())

	if p.IsExpanded("github.com/uk0/silk/core") {
		t.Fatal("new panel: silk/core should default to collapsed")
	}
	p.toggleExpanded("github.com/uk0/silk/core")
	if !p.IsExpanded("github.com/uk0/silk/core") {
		t.Fatal("after toggle: silk/core should be expanded")
	}
	p.toggleExpanded("github.com/uk0/silk/core")
	if p.IsExpanded("github.com/uk0/silk/core") {
		t.Fatal("after second toggle: silk/core should be collapsed")
	}
}

// TestSetPackagesResetsExpansion verifies SetPackages wipes per-package
// expansion state so a re-load doesn't carry stale entries forward.
func TestSetPackagesResetsExpansion(t *testing.T) {
	p := NewPackagesPanel()
	p.SetPackages(fixturePackages())
	p.toggleExpanded("github.com/uk0/silk/core")
	if !p.IsExpanded("github.com/uk0/silk/core") {
		t.Fatal("toggle did not take")
	}
	p.SetPackages(fixturePackages())
	if p.IsExpanded("github.com/uk0/silk/core") {
		t.Fatal("SetPackages did not reset expansion state")
	}
}

// TestPackagesSigPackageActivated verifies clicking the first row (the
// silk/core header, collapsed by default) fires SigPackageActivated
// with that package and toggles its expansion.
func TestPackagesSigPackageActivated(t *testing.T) {
	p := NewPackagesPanel()
	p.SetPackages(fixturePackages())

	var (
		gotPkg core.GoListPackage
		fired  bool
	)
	p.SigPackageActivated(func(pkg core.GoListPackage) {
		gotPkg = pkg
		fired = true
	})

	// Row 0 sits at y in [22, 22+rowHeight). Click the middle.
	y := packagesHeaderH + p.rowHeight/2
	p.OnLeftDown(5, y)

	if !fired {
		t.Fatal("OnLeftDown on header did not fire SigPackageActivated")
	}
	if gotPkg.ImportPath != "github.com/uk0/silk/core" {
		t.Errorf("activated ImportPath = %q, want %q", gotPkg.ImportPath, "github.com/uk0/silk/core")
	}
	if !p.IsExpanded("github.com/uk0/silk/core") {
		t.Error("header click did not toggle expansion")
	}
}

// TestPackagesSigFileActivated verifies clicking a file row inside an
// expanded package fires SigFileActivated with (pkg, file). silk/core
// gets pre-expanded; after that, row 0 is its header, row 1 is "a.go".
func TestPackagesSigFileActivated(t *testing.T) {
	p := NewPackagesPanel()
	p.SetPackages(fixturePackages())
	p.toggleExpanded("github.com/uk0/silk/core") // expand so file rows appear

	var (
		gotPkg  core.GoListPackage
		gotFile string
		fired   bool
	)
	p.SigFileActivated(func(pkg core.GoListPackage, file string) {
		gotPkg = pkg
		gotFile = file
		fired = true
	})

	// Row 1 (a.go) sits at y in [22+rowHeight, 22+2*rowHeight).
	y := packagesHeaderH + p.rowHeight + p.rowHeight/2
	p.OnLeftDown(5, y)

	if !fired {
		t.Fatal("OnLeftDown on file row did not fire SigFileActivated")
	}
	if gotPkg.ImportPath != "github.com/uk0/silk/core" {
		t.Errorf("activated pkg = %q, want %q", gotPkg.ImportPath, "github.com/uk0/silk/core")
	}
	if gotFile != "a.go" {
		t.Errorf("activated file = %q, want %q", gotFile, "a.go")
	}
}

// TestPackagesHeaderClickNoFileCallback verifies a header click does
// not fire the file-activation callback, even when the package is
// expanded.
func TestPackagesHeaderClickNoFileCallback(t *testing.T) {
	p := NewPackagesPanel()
	p.SetPackages(fixturePackages())
	p.toggleExpanded("github.com/uk0/silk/core")
	// After expand, row 0 is still the silk/core header.

	fileFired := false
	p.SigFileActivated(func(core.GoListPackage, string) { fileFired = true })
	p.OnLeftDown(5, packagesHeaderH+p.rowHeight/2)

	if fileFired {
		t.Error("clicking header fired SigFileActivated")
	}
}

// TestPackagesHeaderBandClickNoop verifies clicks inside the 22px
// header band do not activate or toggle anything.
func TestPackagesHeaderBandClickNoop(t *testing.T) {
	p := NewPackagesPanel()
	p.SetPackages(fixturePackages())

	pkgFired := false
	p.SigPackageActivated(func(core.GoListPackage) { pkgFired = true })
	p.OnLeftDown(5, 5) // inside the 22px header band

	if pkgFired {
		t.Error("OnLeftDown in header band fired SigPackageActivated")
	}
	if p.IsExpanded("github.com/uk0/silk/core") {
		t.Error("OnLeftDown in header band toggled expansion")
	}
}
