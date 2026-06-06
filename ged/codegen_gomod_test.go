package ged

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"silk/graph"
)

// newSceneWithButton builds a scene containing a single named button
// at a fixed location. Used by the go.mod-aware codegen tests to keep
// the fixture identical across cases — what varies between cases is
// the projectDir / opts argument, not the scene.
func newSceneWithButton(t *testing.T) *GedScene {
	t.Helper()
	scene := NewGedScene()
	scene.SetFormTitle("ModTest")
	scene.SetSize(120, 80)

	btn, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create button: %v", err)
	}
	btn.SetWidgetName("btn")
	btn.SetBounds(5, 5, 25, 7)
	cmd := graph.NewAddCommand()
	cmd.AddItem(btn, scene)
	scene.PushCommand(cmd)
	return scene
}

// TestCodegenGomodWrapperFindsModuleAtProjectRoot: pointing
// GenerateCodeWithMod at a directory that contains a go.mod surfaces
// the module path as a "// Module: <path>" comment in the generated
// header.
func TestCodegenGomodWrapperFindsModuleAtProjectRoot(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"),
		[]byte("module silk/example\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	scene := newSceneWithButton(t)
	code := scene.GenerateCodeWithMod(tmp, CodeGenOptions{})

	if !strings.Contains(code, "// Module: silk/example") {
		t.Errorf("expected `// Module: silk/example` comment\n----\n%s", code)
	}
}

// TestCodegenGomodWrapperWalksUpFromSubdir: core.LoadGoMod walks
// upward to find go.mod; the wrapper must inherit that behaviour so a
// caller in a nested package still sees the project's module path.
func TestCodegenGomodWrapperWalksUpFromSubdir(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"),
		[]byte("module silk/example\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(tmp, "pkg", "deep")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	scene := newSceneWithButton(t)
	code := scene.GenerateCodeWithMod(sub, CodeGenOptions{})

	if !strings.Contains(code, "// Module: silk/example") {
		t.Errorf("upward walk from sub-sub-dir should still find go.mod\n----\n%s", code)
	}
}

// TestCodegenGomodWrapperNoGoModFallsBack: an absent go.mod is
// non-fatal — the wrapper should produce a working output that simply
// omits the module comment.
func TestCodegenGomodWrapperNoGoModFallsBack(t *testing.T) {
	tmp := t.TempDir()
	// Deliberately do NOT write a go.mod here. core.LoadGoMod will
	// walk upward and may find the repo's real go.mod above tmp, so
	// we cross-check against a path under tmp that doesn't exist on
	// disk above it: t.TempDir() returns a path under /tmp on most
	// systems, which has no parent go.mod — but to be safe we accept
	// either "no Module comment" OR a Module comment with a path that
	// is NOT what we'd have written.
	scene := newSceneWithButton(t)
	code := scene.GenerateCodeWithMod(tmp, CodeGenOptions{})

	// The crucial property: it didn't crash and produced compilable
	// header content (package + at least one import line).
	if !strings.Contains(code, "package main") {
		t.Errorf("missing package declaration in fallback path\n----\n%s", code)
	}
	// It must not have invented a module path we never set. If a
	// "// Module:" line appears, it can only come from a real
	// ancestor go.mod (which is fine — that's correct upward-walk
	// behaviour), but it must not equal an arbitrary string like
	// the tmp dir name.
	if strings.Contains(code, "// Module: "+filepath.Base(tmp)) {
		t.Errorf("module path should not be derived from temp dir name\n----\n%s", code)
	}
}

// TestCodegenGomodExplicitModulePathHonored: when the caller pre-sets
// opts.ModulePath, the wrapper must not overwrite it from go.mod.
// Pair this with a tmp dir that has no go.mod so the LoadGoMod call
// would otherwise return nothing — proves the explicit value reaches
// GenerateCode unchanged.
func TestCodegenGomodExplicitModulePathHonored(t *testing.T) {
	tmp := t.TempDir()
	scene := newSceneWithButton(t)

	code := scene.GenerateCodeWithMod(tmp, CodeGenOptions{ModulePath: "github.com/foo/bar"})

	if !strings.Contains(code, "// Module: github.com/foo/bar") {
		t.Errorf("explicit ModulePath should be emitted verbatim\n----\n%s", code)
	}
}

// TestGenerateCodeWithoutModulePathEmitsNoComment: existing callers
// that don't touch ModulePath must see byte-identical output to
// pre-change behaviour. The single observable proof here is that no
// "// Module:" line appears anywhere in the generated source.
func TestGenerateCodeWithoutModulePathEmitsNoComment(t *testing.T) {
	scene := newSceneWithButton(t)
	code := scene.GenerateCode(CodeGenOptions{})

	if strings.Contains(code, "// Module:") {
		t.Errorf("empty ModulePath must not emit Module comment\n----\n%s", code)
	}
}
