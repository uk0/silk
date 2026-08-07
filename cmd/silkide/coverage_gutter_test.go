package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// The coverage gutter had never lit up, on any platform, and a unit test
// feeding the lookup a hand-written key could not see it: `go test
// -coverprofile` names every file by its FULL IMPORT PATH
// ("github.com/uk0/silk/geom/mat3x2.go"), never by anything resembling the
// checkout it came from. So these tests run the real toolchain against a real
// throwaway module, parse the profile it writes with the production parser,
// and then ask the production lookup for a real on-disk path — the same
// sequence runProjectWithCoverage performs, minus the goroutine.

// covDemoSource is the file under test in the throwaway module. One branch is
// exercised, the other is not, so the profile has to disagree with itself
// across two adjacent lines — that is what makes a one-line stripe shift
// visible instead of self-consistent.
const covDemoSource = `package calc

// Classify names the sign of n.
func Classify(n int) string {
	if n > 0 {
		return "positive"
	}
	return "non-positive"
}
`

const covDemoTest = `package calc

import "testing"

func TestClassifyPositive(t *testing.T) {
	if got := Classify(1); got != "positive" {
		t.Fatalf("Classify(1) = %q", got)
	}
}
`

// writeCovDemoModule lays a one-package module down in a temp directory and
// returns its root plus the absolute path of the source file.
func writeCovDemoModule(t *testing.T, modulePath string) (root, srcPath string) {
	t.Helper()
	root = t.TempDir()
	pkg := filepath.Join(root, "calc")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	files := map[string]string{
		filepath.Join(root, "go.mod"):      "module " + modulePath + "\n\ngo 1.21\n",
		filepath.Join(pkg, "calc.go"):      covDemoSource,
		filepath.Join(pkg, "calc_test.go"): covDemoTest,
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return root, filepath.Join(pkg, "calc.go")
}

// runCovDemoProfile runs the real `go test -coverprofile` in root and hands
// back the parsed per-file coverage, exactly as runProjectWithCoverage does.
func runCovDemoProfile(t *testing.T, root string) map[string]*core.FileCoverage {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; cannot produce a real cover profile")
	}
	out := filepath.Join(t.TempDir(), "cover.out")
	cmd := exec.Command("go", "test", "-coverprofile="+out, "-covermode=set", "./...")
	cmd.Dir = root
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test in %s: %v\n%s", root, err, combined)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	t.Logf("profile:\n%s", data)
	_, blocks, err := core.ParseCoverage(string(data))
	if err != nil {
		t.Fatalf("ParseCoverage: %v", err)
	}
	return core.BuildFileCoverage(blocks)
}

// installCovDemo mirrors the two lines runProjectWithCoverage runs on the UI
// thread: resolve the module identity of the tree go test ran in, then adopt
// the profile as the IDE's current coverage. Restored when the test ends.
func installCovDemo(t *testing.T, fc map[string]*core.FileCoverage, root string) {
	t.Helper()
	savedCov, savedMod, savedRoot := globalCoverage, globalCoverageModule, globalCoverageRoot
	t.Cleanup(func() {
		globalCoverage, globalCoverageModule, globalCoverageRoot = savedCov, savedMod, savedRoot
	})
	modPath, modRoot := loadModuleInfo(root)
	globalCoverage, globalCoverageModule, globalCoverageRoot = fc, modPath, modRoot
}

// lineIndexOf returns the 0-based index of the single line whose trimmed text
// is `want` — the editor's own coordinate for that line of code.
func lineIndexOf(t *testing.T, text, want string) int {
	t.Helper()
	found := -1
	for i, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != want {
			continue
		}
		if found >= 0 {
			t.Fatalf("line %q appears more than once; cannot anchor the assertion", want)
		}
		found = i
	}
	if found < 0 {
		t.Fatalf("line %q not found in the editor buffer", want)
	}
	return found
}

// TestCoverageGutterLightsUpForARealProfile is the end-to-end proof. It fails
// on origin/main: the profile's keys are import paths, the editor's key is an
// absolute path under a checkout named nothing like the module, so the lookup
// misses and the editor never gets a coverage map at all.
func TestCoverageGutterLightsUpForARealProfile(t *testing.T) {
	root, srcPath := writeCovDemoModule(t, "example.com/covdemo")
	fc := runCovDemoProfile(t, root)

	// The premise, asserted rather than assumed: the toolchain keys the
	// profile by import path, and that key is NOT a suffix of the file's
	// path on disk — so nothing but the module mapping can resolve it.
	wantKey := "example.com/covdemo/calc/calc.go"
	if _, ok := fc[wantKey]; !ok {
		t.Fatalf("profile keys = %v, want one spelled %q", covKeys(fc), wantKey)
	}
	if strings.HasSuffix(filepath.ToSlash(srcPath), "/"+wantKey) {
		t.Fatalf("editor path %q ends in the profile key; this test is no longer testing anything", srcPath)
	}

	installCovDemo(t, fc, root)

	saved := openEditors
	openEditors = map[string]*gui.CodeEditor{}
	t.Cleanup(func() { openEditors = saved })
	tabs := buildEditorTabs(nil)
	if !openFileInEditor(tabs, srcPath) {
		t.Fatal("openFileInEditor returned false")
	}
	ed := openEditors[srcPath]
	if ed == nil {
		t.Fatal("openEditors did not track the opened file")
	}
	applyCoverageToOpenEditors(fc)

	if !ed.HasCoverage() {
		t.Fatal("editor has no coverage map after a real coverage run; the gutter is dark")
	}
	// The user's property: the stripe sits on the branch that ran, and the
	// other colour sits on the branch that did not. Anchored to the text of
	// the lines so a one-line shift cannot pass — line `return "positive"`
	// is inside the covered block, and the line one above it (the `if`) is
	// covered too, but `return "non-positive"` is NOT, while the `}` above
	// it is.
	ran := lineIndexOf(t, ed.Text(), `return "positive"`)
	if covered, has := ed.LineCovered(ran); !has || !covered {
		t.Errorf(`LineCovered(%d) for 'return "positive"' = (%v,%v), want (true,true)`, ran, covered, has)
	}
	skipped := lineIndexOf(t, ed.Text(), `return "non-positive"`)
	if covered, has := ed.LineCovered(skipped); !has || covered {
		t.Errorf(`LineCovered(%d) for 'return "non-positive"' = (%v,%v), want (false,true)`, skipped, covered, has)
	}
}

// TestCoverageAppliedToAFileOpenedAfterTheRun: the run pushes into the tabs
// that exist at the time, so opening a file afterwards used to show nothing
// even though the profile on hand had measured it. No applyCoverage* call
// here — openFileInEditor has to do it on its own.
func TestCoverageAppliedToAFileOpenedAfterTheRun(t *testing.T) {
	root, srcPath := writeCovDemoModule(t, "example.com/covlate")
	fc := runCovDemoProfile(t, root)
	installCovDemo(t, fc, root)

	saved := openEditors
	openEditors = map[string]*gui.CodeEditor{}
	t.Cleanup(func() { openEditors = saved })
	tabs := buildEditorTabs(nil)
	if !openFileInEditor(tabs, srcPath) {
		t.Fatal("openFileInEditor returned false")
	}
	ed := openEditors[srcPath]
	if ed == nil {
		t.Fatal("openEditors did not track the opened file")
	}
	ran := lineIndexOf(t, ed.Text(), `return "positive"`)
	if covered, has := ed.LineCovered(ran); !has || !covered {
		t.Errorf(`file opened after the run: LineCovered(%d) = (%v,%v), want (true,true)`,
			ran, covered, has)
	}
}

// TestCoverageStripesClearedOnAFileTheNewRunDidNotMeasure is the other half
// of a working gutter. The walk only ever SET coverage, so a tab left over
// from an earlier project kept showing that project's green — a claim about
// the current run that the current run never made. Two real modules, two real
// profiles: after the second run the first module's tab must be blank.
func TestCoverageStripesClearedOnAFileTheNewRunDidNotMeasure(t *testing.T) {
	rootA, srcA := writeCovDemoModule(t, "example.com/covfirst")
	fcA := runCovDemoProfile(t, rootA)
	rootB, _ := writeCovDemoModule(t, "example.com/covsecond")
	fcB := runCovDemoProfile(t, rootB)

	saved := openEditors
	openEditors = map[string]*gui.CodeEditor{}
	t.Cleanup(func() { openEditors = saved })
	tabs := buildEditorTabs(nil)
	if !openFileInEditor(tabs, srcA) {
		t.Fatal("openFileInEditor returned false")
	}
	ed := openEditors[srcA]
	if ed == nil {
		t.Fatal("openEditors did not track the opened file")
	}

	installCovDemo(t, fcA, rootA)
	applyCoverageToOpenEditors(fcA)
	if !ed.HasCoverage() {
		t.Fatal("setup: first run did not light the gutter")
	}

	// Second run, different module, and it says nothing about srcA.
	installCovDemo(t, fcB, rootB)
	applyCoverageToOpenEditors(fcB)
	if ed.HasCoverage() {
		covered, _ := ed.LineCovered(lineIndexOf(t, ed.Text(), `return "positive"`))
		t.Errorf("stale stripes survived a run that did not measure %s (line still reports covered=%v)",
			srcA, covered)
	}
}

// TestCoverageKeyMappingWindowsSeparators drives the shape a Windows host
// hands the matcher: a backslashed editor path and a backslashed module root
// against the forward-slashed import-path key that is the only form `go test
// -coverprofile` ever writes. We cannot run this on the real thing from here,
// so the separator is a parameter and the assertion is on the fold.
func TestCoverageKeyMappingWindowsSeparators(t *testing.T) {
	fc := map[string]*core.FileCoverage{
		"example.com/covdemo/calc/calc.go": {
			File:    "example.com/covdemo/calc/calc.go",
			Covered: map[int]bool{6: true, 8: false},
		},
	}
	got, ok := coverageForPathIn(fc, `C:\Users\alice\src\dc\calc\calc.go`,
		"example.com/covdemo", `C:\Users\alice\src\dc`, '\\')
	if !ok {
		t.Fatalf("windows module mapping: ok = false, want true")
	}
	if got.File != "example.com/covdemo/calc/calc.go" {
		t.Errorf("got.File = %q, want the demo key", got.File)
	}
}

// TestCoverageKeyMappingKeepsRootBoundary: the module root has to be matched
// on a directory boundary like everything else here, or a sibling checkout
// ("dc-old", "dcx") would inherit the neighbour's stripes — and with
// import-path keys there is no suffix fallback left to correct it.
func TestCoverageKeyMappingKeepsRootBoundary(t *testing.T) {
	fc := map[string]*core.FileCoverage{
		"example.com/covdemo/calc/calc.go": {
			File:    "example.com/covdemo/calc/calc.go",
			Covered: map[int]bool{6: true},
		},
	}
	if _, ok := coverageForPathIn(fc, "/src/dcx/calc/calc.go",
		"example.com/covdemo", "/src/dc", '/'); ok {
		t.Errorf("module rooted at /src/dc claimed /src/dcx/calc/calc.go")
	}
	if _, ok := coverageForPathIn(fc, "/src/dc/calc/calc.go",
		"example.com/covdemo", "/src/dc", '/'); !ok {
		t.Errorf("module rooted at /src/dc failed to claim its own /src/dc/calc/calc.go")
	}
}

// TestLoadModuleInfoReturnsTheDirectoryTheModuleNames pins the half of the
// mapping that did not exist before: loadModulePath threw the go.mod's
// directory away, which is exactly the piece needed to turn an import-path
// key back into a file. Asked from a SUBdirectory, since that is how it is
// called (the coverage run's cwd is the project dir, not necessarily the
// module root).
func TestLoadModuleInfoReturnsTheDirectoryTheModuleNames(t *testing.T) {
	root, srcPath := writeCovDemoModule(t, "example.com/covinfo")
	path, dir := loadModuleInfo(filepath.Dir(srcPath))
	if path != "example.com/covinfo" {
		t.Errorf("module path = %q, want %q", path, "example.com/covinfo")
	}
	if dir != root {
		t.Errorf("module root = %q, want %q", dir, root)
	}
}

// covKeys is only for failure messages.
func covKeys(fc map[string]*core.FileCoverage) []string {
	out := make([]string, 0, len(fc))
	for k := range fc {
		out = append(out, k)
	}
	return out
}
