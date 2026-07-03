package main

import (
	"testing"

	"github.com/uk0/silk/core"
)

// TestPackagesActivatedToastMessage locks in the user-facing string
// SigPackageActivated toasts. ImportPath wins when set; falls back to
// Dir (the stdlib / GOPATH edge case where go list omits ImportPath);
// final fallback is the "(empty)" placeholder so the toast is never
// blank.
func TestPackagesActivatedToastMessage(t *testing.T) {
	cases := []struct {
		name string
		pkg  core.GoListPackage
		want string
	}{
		{
			name: "import path wins",
			pkg:  core.GoListPackage{ImportPath: "github.com/uk0/silk/ged", Dir: "/repo/ged"},
			want: "github.com/uk0/silk/ged",
		},
		{
			name: "dir fallback when import path empty",
			pkg:  core.GoListPackage{Dir: "/tmp/loose"},
			want: "/tmp/loose",
		},
		{
			name: "empty placeholder when nothing set",
			pkg:  core.GoListPackage{},
			want: "(empty)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := packagesActivatedToastMessage(c.pkg)
			if got != c.want {
				t.Errorf("packagesActivatedToastMessage(%+v) = %q, want %q", c.pkg, got, c.want)
			}
		})
	}
}

// TestRegisterPaletteCommandsContainsPackagesEntries: the two
// PackagesPanel commands ("Show Packages" + "Refresh Packages") have
// to be reachable from the palette — without them the dock-flip
// muscle memory we wired for Outline / Problems / Bookmarks doesn't
// extend to the Packages tab.
func TestRegisterPaletteCommandsContainsPackagesEntries(t *testing.T) {
	saved := paletteCommands
	defer func() { paletteCommands = saved }()
	paletteCommands = nil

	registerPaletteCommands(nil, nil)

	want := map[string]bool{
		"Show Packages":    false,
		"Refresh Packages": false,
	}
	for _, c := range paletteCommands {
		if _, ok := want[c.Name]; ok {
			want[c.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("paletteCommands missing %q entry", name)
		}
	}
}

// TestRefreshPackagesNilPanelNoOp: refreshPackages must not panic when
// globalPackages hasn't been wired (the buildPanels-before-main test
// path leaves the panel nil). Mirrors the silkideToast nil-frame test —
// same defensive shape, same reason: startup wiring fires before the
// shell is fully assembled.
func TestRefreshPackagesNilPanelNoOp(t *testing.T) {
	saved := globalPackages
	defer func() { globalPackages = saved }()
	globalPackages = nil

	// Calling with a nil canvas + nil panel must be a clean no-op.
	refreshPackages(nil)
}
