package ged

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
)

// TestGoModSummaryString_Populated covers the exact one-line format the
// panel header and tests rely on. The format is load-bearing.
func TestGoModSummaryString_Populated(t *testing.T) {
	m := &core.GoMod{
		Module:    "silk/example",
		GoVersion: "1.22",
		Requires: []core.GoModRequire{
			{Path: "github.com/a/b", Version: "v1.0.0"},
			{Path: "github.com/c/d", Version: "v2.3.4"},
		},
	}
	got := goModSummaryString(m)
	want := "Module: silk/example • Go 1.22 • 2 requires"
	if got != want {
		t.Fatalf("goModSummaryString = %q\nwant %q", got, want)
	}
}

// TestGoModSummaryString_SingleRequire verifies the singular noun branch.
func TestGoModSummaryString_SingleRequire(t *testing.T) {
	m := &core.GoMod{
		Module:    "silk/example",
		GoVersion: "1.21",
		Requires:  []core.GoModRequire{{Path: "foo", Version: "v1.0.0"}},
	}
	got := goModSummaryString(m)
	want := "Module: silk/example • Go 1.21 • 1 require"
	if got != want {
		t.Fatalf("goModSummaryString = %q\nwant %q", got, want)
	}
}

// TestGoModSummaryString_Nil verifies the nil sentinel.
func TestGoModSummaryString_Nil(t *testing.T) {
	got := goModSummaryString(nil)
	want := "(no go.mod found)"
	if got != want {
		t.Fatalf("goModSummaryString(nil) = %q\nwant %q", got, want)
	}
}

// TestProjectSettingsRefreshGoMod verifies RefreshGoMod populates the panel
// from a real go.mod file written to a temp directory.
func TestProjectSettingsRefreshGoMod(t *testing.T) {
	dir := t.TempDir()
	content := "module silk/example\n\ngo 1.21\n\nrequire foo v1.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	p := &ProjectSettingsPanel{}
	p.RefreshGoMod(dir)

	if p.goMod == nil {
		t.Fatalf("goMod is nil after RefreshGoMod")
	}
	if p.goModule != "silk/example" {
		t.Errorf("goModule = %q, want %q", p.goModule, "silk/example")
	}
	if p.goVersion != "1.21" {
		t.Errorf("goVersion = %q, want %q", p.goVersion, "1.21")
	}
	if got, want := len(p.goMod.Requires), 1; got != want {
		t.Errorf("len(Requires) = %d, want %d", got, want)
	}
	want := "Module: silk/example • Go 1.21 • 1 require"
	if p.goModSummary != want {
		t.Errorf("goModSummary = %q, want %q", p.goModSummary, want)
	}
	if p.goModPath == "" || !strings.HasSuffix(p.goModPath, "go.mod") {
		t.Errorf("goModPath = %q, want absolute path ending in go.mod", p.goModPath)
	}
}

// TestProjectSettingsRefreshGoMod_NotFound verifies the no-go.mod fallback.
// FindGoMod walks parent directories upward; we use a temp dir whose
// ancestors (/tmp or /var/folders/...) have no go.mod, so the search
// exhausts and the panel must fall back to the sentinel summary.
func TestProjectSettingsRefreshGoMod_NotFound(t *testing.T) {
	dir := t.TempDir()
	// Sanity-check: if some ancestor unexpectedly contains a go.mod the
	// no-found branch can't be exercised; skip rather than report a false
	// failure. This protects unusual CI layouts without hiding real bugs.
	if path, ok := core.FindGoMod(dir); ok {
		t.Skipf("ancestor go.mod found at %s; cannot exercise not-found branch", path)
	}

	p := &ProjectSettingsPanel{}
	p.RefreshGoMod(dir)

	if p.goMod != nil {
		t.Fatalf("goMod = %+v, want nil", p.goMod)
	}
	if p.goModSummary != "(no go.mod found)" {
		t.Errorf("goModSummary = %q, want %q", p.goModSummary, "(no go.mod found)")
	}
	if p.goModPath != "" {
		t.Errorf("goModPath = %q, want empty", p.goModPath)
	}
	if p.goModule != "(未找到 go.mod)" {
		t.Errorf("goModule = %q, want %q", p.goModule, "(未找到 go.mod)")
	}
}
