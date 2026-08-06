package ged

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/graph"
)

// writeTestGoMod drops a minimal go.mod so the directory becomes a project
// root — the marker ScanProject looks for.
func writeTestGoMod(t *testing.T, dir, module string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	src := "module " + module + "\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
}

// writeTestDesign saves a real one-button design at path so the generator has
// something to read back. Directories are created as needed.
func writeTestDesign(t *testing.T, path, title string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	scene := NewGedScene()
	scene.SetFormTitle(title)
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

	if err := scene.SaveDesign().SaveFile(path); err != nil {
		t.Fatalf("save design %s: %v", path, err)
	}
}

// TestIsDesignPath: the rule that decides what the project owns and what the
// file tree opens on the canvas. Generated Go sitting next to a design must
// not be mistaken for one.
func TestIsDesignPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/a/b/foo.silkui", true},
		{"foo.silkui", true},
		{"/a/b/FOO.SILKUI", true},
		{"/a/b/foo.silk.go", false},
		{"/a/b/foo.go", false},
		{"/a/b/foo.cml", false},
		{"/a/b/silkui", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsDesignPath(c.path); got != c.want {
			t.Errorf("IsDesignPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// TestOutputPathsForDesign: the design->output mapping. Output lands beside
// the design, never in a temp dir, and the two halves never share a name.
func TestOutputPathsForDesign(t *testing.T) {
	cases := []struct {
		design        string
		wantGenerated string
		wantCompanion string
	}{
		{
			design:        filepath.Join("/proj", "foo.silkui"),
			wantGenerated: filepath.Join("/proj", "foo.silk.go"),
			wantCompanion: filepath.Join("/proj", "foo.go"),
		},
		{
			design:        filepath.Join("/proj", "ui", "deep", "Main Window.silkui"),
			wantGenerated: filepath.Join("/proj", "ui", "deep", "Main Window.silk.go"),
			wantCompanion: filepath.Join("/proj", "ui", "deep", "Main Window.go"),
		},
		{
			design:        "foo.silkui",
			wantGenerated: "foo.silk.go",
			wantCompanion: "foo.go",
		},
	}
	for _, c := range cases {
		got := OutputPathsFor(c.design)
		if got.Generated != c.wantGenerated {
			t.Errorf("OutputPathsFor(%q).Generated = %q, want %q", c.design, got.Generated, c.wantGenerated)
		}
		if got.Companion != c.wantCompanion {
			t.Errorf("OutputPathsFor(%q).Companion = %q, want %q", c.design, got.Companion, c.wantCompanion)
		}
	}

	if got := OutputPathsFor(""); got.Generated != "" || got.Companion != "" {
		t.Errorf("OutputPathsFor(\"\") = %+v, want zero value", got)
	}
}

// TestFindProjectRootWalksUp: a design nested several directories deep still
// resolves to the module that contains it.
func TestFindProjectRootWalksUp(t *testing.T) {
	tmp := t.TempDir()
	writeTestGoMod(t, tmp, "example.com/app")
	deep := filepath.Join(tmp, "ui", "screens")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}

	root, err := FindProjectRoot(deep)
	if err != nil {
		t.Fatalf("FindProjectRoot: %v", err)
	}
	if !sameDir(root, tmp) {
		t.Errorf("root = %q, want %q", root, tmp)
	}
}

// TestFindProjectRootWithoutGoMod: a directory outside any module has no
// project. t.TempDir() lives under the system temp dir, which has no go.mod
// above it; if one somehow exists the walk is still correct as long as it did
// not invent a root inside tmp.
func TestFindProjectRootWithoutGoMod(t *testing.T) {
	tmp := t.TempDir()
	root, err := FindProjectRoot(tmp)
	if err == nil && strings.HasPrefix(root, tmp) {
		t.Errorf("FindProjectRoot(%q) = %q with no go.mod under it", tmp, root)
	}
}

// TestScanProjectCollectsNestedDesigns: the project is the go.mod directory
// plus every .silkui under it, sorted, with generated Go and other files
// ignored.
func TestScanProjectCollectsNestedDesigns(t *testing.T) {
	tmp := t.TempDir()
	writeTestGoMod(t, tmp, "example.com/app")

	writeTestDesign(t, filepath.Join(tmp, "b.silkui"), "B")
	writeTestDesign(t, filepath.Join(tmp, "a.silkui"), "A")
	writeTestDesign(t, filepath.Join(tmp, "ui", "screens", "c.silkui"), "C")
	if err := os.WriteFile(filepath.Join(tmp, "a.silk.go"), []byte("package app\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "notes.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	proj, err := ScanProject(filepath.Join(tmp, "ui"))
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}
	if !sameDir(proj.Root, tmp) {
		t.Errorf("Root = %q, want %q", proj.Root, tmp)
	}
	if proj.Module != "example.com/app" {
		t.Errorf("Module = %q, want example.com/app", proj.Module)
	}

	want := []string{
		filepath.Join(tmp, "a.silkui"),
		filepath.Join(tmp, "b.silkui"),
		filepath.Join(tmp, "ui", "screens", "c.silkui"),
	}
	if len(proj.Designs) != len(want) {
		t.Fatalf("Designs = %v, want %v", proj.Designs, want)
	}
	for i := range want {
		if !sameDir(proj.Designs[i], want[i]) {
			t.Errorf("Designs[%d] = %q, want %q", i, proj.Designs[i], want[i])
		}
	}
}

// TestScanProjectStopsAtNestedModule: a subdirectory with its own go.mod is a
// different project. silk itself ships mod/map that way, so a scan that walked
// into it would hand one project the other's designs.
func TestScanProjectStopsAtNestedModule(t *testing.T) {
	tmp := t.TempDir()
	writeTestGoMod(t, tmp, "example.com/app")
	sub := filepath.Join(tmp, "mod", "map")
	writeTestGoMod(t, sub, "example.com/app/mod/map")

	writeTestDesign(t, filepath.Join(tmp, "own.silkui"), "Own")
	writeTestDesign(t, filepath.Join(sub, "other.silkui"), "Other")

	proj, err := ScanProject(tmp)
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}
	if len(proj.Designs) != 1 {
		t.Fatalf("Designs = %v, want only the outer design", proj.Designs)
	}
	if filepath.Base(proj.Designs[0]) != "own.silkui" {
		t.Errorf("Designs[0] = %q, want own.silkui", proj.Designs[0])
	}
}

// TestScanProjectSkipsNoiseDirs: the project list and the file tree must agree
// on what the project contains, so the scan hides the same directories the
// tree does.
func TestScanProjectSkipsNoiseDirs(t *testing.T) {
	tmp := t.TempDir()
	writeTestGoMod(t, tmp, "example.com/app")
	writeTestDesign(t, filepath.Join(tmp, "keep.silkui"), "Keep")
	writeTestDesign(t, filepath.Join(tmp, "vendor", "dep.silkui"), "Dep")
	writeTestDesign(t, filepath.Join(tmp, ".cache", "hidden.silkui"), "Hidden")

	proj, err := ScanProject(tmp)
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}
	if len(proj.Designs) != 1 || filepath.Base(proj.Designs[0]) != "keep.silkui" {
		t.Errorf("Designs = %v, want only keep.silkui", proj.Designs)
	}
}

// TestScanProjectExcludesDesignOutsideModule: a design that is a sibling of
// the module root, not under it, belongs to no project of ours.
func TestScanProjectExcludesDesignOutsideModule(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "app")
	writeTestGoMod(t, root, "example.com/app")
	writeTestDesign(t, filepath.Join(root, "inside.silkui"), "Inside")
	writeTestDesign(t, filepath.Join(tmp, "outside.silkui"), "Outside")

	proj, err := ScanProject(root)
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}
	for _, d := range proj.Designs {
		if filepath.Base(d) == "outside.silkui" {
			t.Fatalf("Designs = %v, must not reach outside the module root", proj.Designs)
		}
	}
	if len(proj.Designs) != 1 {
		t.Errorf("Designs = %v, want only inside.silkui", proj.Designs)
	}
}

// TestScanProjectWithoutGoMod: no module marker, no project.
func TestScanProjectWithoutGoMod(t *testing.T) {
	tmp := t.TempDir()
	writeTestDesign(t, filepath.Join(tmp, "lonely.silkui"), "Lonely")

	proj, err := ScanProject(tmp)
	if err == nil && proj != nil && strings.HasPrefix(proj.Root, tmp) {
		t.Errorf("ScanProject(%q) = root %q with no go.mod under it", tmp, proj.Root)
	}
}

// TestPackageNameForDir: generated code has to carry the package clause of the
// directory it lands in, and Go's convention is package == directory name.
func TestPackageNameForDir(t *testing.T) {
	cases := []struct{ dir, want string }{
		{filepath.Join("/proj", "screens"), "screens"},
		{filepath.Join("/proj", "My-App"), "myapp"},
		{filepath.Join("/proj", "mod", "map"), "ui"}, // keyword
		{filepath.Join("/proj", "123"), "ui"},        // not an identifier
		{filepath.Join("/proj", "界面"), "ui"},         // nothing usable left
	}
	for _, c := range cases {
		if got := packageNameForDir(c.dir); got != c.want {
			t.Errorf("packageNameForDir(%q) = %q, want %q", c.dir, got, c.want)
		}
	}
}

// TestGenerateAllWritesBesideDesign: 全部生成 writes each design's Go next to
// the design itself and never touches the hand-written companion half.
func TestGenerateAllWritesBesideDesign(t *testing.T) {
	tmp := t.TempDir()
	writeTestGoMod(t, tmp, "example.com/app")
	writeTestDesign(t, filepath.Join(tmp, "screens", "editor.silkui"), "Editor")

	companion := filepath.Join(tmp, "screens", "editor.go")
	if err := os.WriteFile(companion, []byte("package screens\n\n// hand written\n"), 0644); err != nil {
		t.Fatal(err)
	}

	proj, err := ScanProject(tmp)
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}
	results := proj.GenerateAll()
	if len(results) != 1 {
		t.Fatalf("results = %+v, want one per design", results)
	}
	if results[0].Err != nil {
		t.Fatalf("generate failed: %v", results[0].Err)
	}

	generated := filepath.Join(tmp, "screens", "editor.silk.go")
	if !sameDir(results[0].Output, generated) {
		t.Errorf("Output = %q, want %q", results[0].Output, generated)
	}
	data, err := os.ReadFile(generated)
	if err != nil {
		t.Fatalf("generated file not written beside the design: %v", err)
	}
	code := string(data)
	if !strings.Contains(code, "package screens") {
		t.Errorf("generated code must join the directory's package\n----\n%s", code)
	}
	if !strings.Contains(code, "// Module: example.com/app") {
		t.Errorf("generated code missing the project's module comment\n----\n%s", code)
	}
	if !strings.Contains(code, "type EditorUI struct") {
		t.Errorf("type name should come from the design file stem\n----\n%s", code)
	}
	// The generated half is a library half: main() belongs to the companion.
	if strings.Contains(code, "func main()") {
		t.Errorf("generated half must not declare main()\n----\n%s", code)
	}

	kept, err := os.ReadFile(companion)
	if err != nil {
		t.Fatalf("companion read: %v", err)
	}
	if !strings.Contains(string(kept), "hand written") {
		t.Errorf("companion was overwritten: %q", string(kept))
	}
}

// TestGenerateAllReportsPerDesignFailure: one design that cannot be written
// must not abort the batch — every design gets a result and the caller reports
// them together. The blocked output is a directory sitting on the generated
// file's path, which fails the write on every platform.
func TestGenerateAllReportsPerDesignFailure(t *testing.T) {
	tmp := t.TempDir()
	writeTestGoMod(t, tmp, "example.com/app")
	writeTestDesign(t, filepath.Join(tmp, "good.silkui"), "Good")
	writeTestDesign(t, filepath.Join(tmp, "broken.silkui"), "Broken")
	if err := os.MkdirAll(filepath.Join(tmp, "broken.silk.go"), 0755); err != nil {
		t.Fatal(err)
	}

	proj, err := ScanProject(tmp)
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}
	results := proj.GenerateAll()
	if len(results) != 2 {
		t.Fatalf("results = %+v, want one per design", results)
	}

	var okCount, failCount int
	for _, r := range results {
		if r.Err != nil {
			failCount++
			if filepath.Base(r.Design) != "broken.silkui" {
				t.Errorf("unexpected failure on %q: %v", r.Design, r.Err)
			}
			continue
		}
		okCount++
	}
	if okCount != 1 || failCount != 1 {
		t.Errorf("ok=%d fail=%d, want 1/1 (%+v)", okCount, failCount, results)
	}
}

// TestFormatGenerateReport: one dialog body for the whole batch, paths shown
// relative to the project root, counts up front.
func TestFormatGenerateReport(t *testing.T) {
	root := filepath.Join("/proj")
	results := []GenerateResult{
		{
			Design: filepath.Join(root, "a.silkui"),
			Output: filepath.Join(root, "a.silk.go"),
		},
		{
			Design: filepath.Join(root, "ui", "b.silkui"),
			Err:    errors.New("boom"),
		},
	}
	report := FormatGenerateReport(root, results)

	if !strings.Contains(report, "成功 1") || !strings.Contains(report, "失败 1") {
		t.Errorf("report missing counts:\n%s", report)
	}
	if !strings.Contains(report, "a.silkui") || !strings.Contains(report, "a.silk.go") {
		t.Errorf("report missing the successful pair:\n%s", report)
	}
	if !strings.Contains(report, filepath.Join("ui", "b.silkui")) || !strings.Contains(report, "boom") {
		t.Errorf("report missing the failure:\n%s", report)
	}
	if strings.Contains(report, root+string(filepath.Separator)+"a.silkui") {
		t.Errorf("paths should be relative to the project root:\n%s", report)
	}

	if empty := FormatGenerateReport(root, nil); !strings.Contains(empty, ".silkui") {
		t.Errorf("empty report should say the project has no designs, got %q", empty)
	}
}

// TestGeneratedProjectCodeCompiles type-checks what 全部生成 actually wrote,
// through the same go vet machinery the other codegen tests use. String
// matching proves the package clause is there; vet proves the file builds.
func TestGeneratedProjectCodeCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	tmp := t.TempDir()
	writeTestGoMod(t, tmp, "example.com/app")
	writeTestDesign(t, filepath.Join(tmp, "screens", "editor.silkui"), "Editor")

	proj, err := ScanProject(tmp)
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}
	results := proj.GenerateAll()
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("generate failed: %+v", results)
	}

	data, err := os.ReadFile(results[0].Output)
	if err != nil {
		t.Fatal(err)
	}
	vetGeneratedCode(t, string(data))
}

// sameDir compares two paths after resolving symlinks, so a macOS /var vs
// /private/var temp path does not fail an otherwise-correct comparison.
func sameDir(a, b string) bool {
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		ra = a
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		rb = b
	}
	return ra == rb
}
