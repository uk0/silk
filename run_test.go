package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestOnRunDoesNotBuildOnTheUIThread guards the fix for a freeze that looked
// like a crash: clicking the green Run button called go build synchronously
// from the click handler, so the message pump stopped for the whole
// compile — tens of seconds, minutes for a cold cgo/cairo build on Windows —
// and the title bar gained "(Not Responding)".
//
// The compile must stay on a goroutine, and everything that touches a widget
// afterwards must come back through gui.Post. There is no way to assert that
// from a headless test process (onRun needs a live frame and a scene), so this
// reads the source: crude, but it fails the moment someone moves the build
// back onto the event loop.
func TestOnRunDoesNotBuildOnTheUIThread(t *testing.T) {
	body := funcBody(t, "design.go", "func onRun() {")

	build := strings.Index(body, "CombinedOutput()")
	if build < 0 {
		t.Fatal("onRun no longer runs a build; update this test to match")
	}
	spawn := strings.Index(body, "go func() {")
	if spawn < 0 || spawn > build {
		t.Error("the compile in onRun is not inside a goroutine; it will block the event loop for the whole build")
	}
	if post := strings.Index(body, "gui.Post("); post < 0 || post < spawn {
		t.Error("the build result is not handed back through gui.Post; widgets would be mutated off the event loop")
	}
}

// TestOnRunNamesAWindowsExecutable pins the other half: CreateProcess will not
// launch a path without an extension, and "go build -o app" writes exactly the
// name it is given. Without the suffix the build succeeds and the spawn fails.
func TestOnRunNamesAWindowsExecutable(t *testing.T) {
	body := funcBody(t, "design.go", "func onRun() {")
	if !strings.Contains(body, `runtime.GOOS == "windows"`) || !strings.Contains(body, `appPath += ".exe"`) {
		t.Error("onRun does not add the .exe suffix on Windows; the built program cannot be started there")
	}
	if strings.Contains(body, `exec.Command(tmpDir + "/app")`) {
		t.Error("onRun still spawns the extension-less path")
	}
}

// funcBody returns the text between the braces of the named function.
func funcBody(t *testing.T, file, header string) string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("cannot read %s: %v", file, err)
	}
	start := strings.Index(string(src), header)
	if start < 0 {
		t.Fatalf("%s does not contain %q", file, header)
	}
	rest := string(src)[start+len(header):]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatalf("%q in %s is not terminated", header, file)
	}
	return rest[:end]
}

// TestPrependPathPutsToolDirsFirst checks the resolved directories win over
// whatever the inherited PATH already had.
func TestPrependPathPutsToolDirsFirst(t *testing.T) {
	sep := string(os.PathListSeparator)
	got := prependPath("/inherited"+sep+"/more", []string{"/tools", "/cc"})
	want := "/tools" + sep + "/cc" + sep + "/inherited" + sep + "/more"
	if got != want {
		t.Errorf("prependPath = %q, want %q", got, want)
	}
	if got := prependPath("/inherited", nil); got != "/inherited" {
		t.Errorf("nothing to prepend should leave PATH alone, got %q", got)
	}
	if got := prependPath("", []string{"/tools"}); got != "/tools" {
		t.Errorf("empty PATH should not gain a separator, got %q", got)
	}
}

// TestFindToolDirLocatesExecutable covers the lookup that replaces the missing
// PATH entry, including the .exe suffix Windows needs.
func TestFindToolDirLocatesExecutable(t *testing.T) {
	dir := t.TempDir()
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findToolDir([]string{"", filepath.Join(dir, "nope"), dir}, "go"); got != dir {
		t.Errorf("findToolDir = %q, want %q", got, dir)
	}
	if got := findToolDir([]string{dir}, "definitely-not-here"); got != "" {
		t.Errorf("missing tool should yield \"\", got %q", got)
	}
	// A directory named like the tool is not the tool.
	sub := t.TempDir()
	if err := os.Mkdir(filepath.Join(sub, name), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findToolDir([]string{sub}, "go"); got != "" {
		t.Errorf("a directory was mistaken for an executable: %q", got)
	}
}

// TestBuildFailureDetailKeepsTheReason is the regression guard for the report
// the user actually saw: a bare "编译失败" with an empty output panel, because
// a toolchain that never started produces no output and err was discarded.
func TestBuildFailureDetailKeepsTheReason(t *testing.T) {
	detail := buildFailureDetail(nil, errors.New(`exec: "go": executable file not found in %PATH%`))
	if !strings.Contains(detail, "executable file not found") {
		t.Errorf("the only explanation there was got dropped: %q", detail)
	}

	// A compiler that did run explains itself; don't bury that under ours.
	compiler := []byte("./main.go:7:2: undefined: Foo\n")
	if got := buildFailureDetail(compiler, errors.New("exit status 1")); got != string(compiler) {
		t.Errorf("compiler output was rewritten: %q", got)
	}
}

// TestRunBuildNamesTheGoBinaryOutright guards the trap that made the Run
// button fail instantly with an empty output panel: exec.Command resolves a
// bare "go" against the parent's PATH — the one an Explorer-launched GUI
// inherits — and cmd.Env cannot undo that decision afterwards.
func TestRunBuildNamesTheGoBinaryOutright(t *testing.T) {
	body := funcBody(t, "design.go", "func onRun() {")
	if strings.Contains(body, `exec.Command("go"`) {
		t.Error("onRun still spawns a bare \"go\"; it will not be found when the designer's own PATH lacks the toolchain")
	}
	if !strings.Contains(body, "exec.Command(goExecutable()") {
		t.Error("onRun does not resolve the Go binary through goExecutable()")
	}
	if !strings.Contains(body, `"PATH="+buildToolPath()`) {
		t.Error("the child never gets a repaired PATH, so the go tool cannot find gcc/pkg-config")
	}
}

// TestGoExecutableIsAbsoluteWhenFound pins the property that matters: whatever
// comes back must be launchable without a PATH search.
func TestGoExecutableIsAbsoluteWhenFound(t *testing.T) {
	got := goExecutable()
	if got == "go" {
		t.Skip("no Go toolchain on PATH or in the known install dirs")
	}
	if !filepath.IsAbs(got) {
		t.Errorf("goExecutable() = %q, want an absolute path", got)
	}
	if st, err := os.Stat(got); err != nil || st.IsDir() {
		t.Errorf("goExecutable() = %q, which is not an executable file", got)
	}
}

// TestFindModuleRootWalksUp covers the search itself, including the case that
// used to be silently papered over with ".".
func TestFindModuleRootWalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findModuleRoot(deep); got != root {
		t.Errorf("findModuleRoot(%q) = %q, want %q", deep, got, root)
	}
	if got := findModuleRoot(root); got != root {
		t.Errorf("findModuleRoot on the root itself = %q, want %q", got, root)
	}

	// Nothing above a bare temp dir carries a go.mod, and reporting that
	// honestly is the point: "." sent the build to a directory outside any
	// module, where it failed with "no required module provides package".
	bare := t.TempDir()
	if got := findModuleRoot(bare); got != "" {
		t.Errorf("findModuleRoot(%q) = %q, want \"\" — unless a go.mod really sits above the temp dir", bare, got)
	}
}

// TestSilkModuleRootPrefersTheExecutable is the regression guard for a Run
// failure that only appeared when the designer was launched from a shortcut:
// a GUI process inherits the launcher's working directory, so resolving the
// module from the cwd found nothing and the preview build ran outside the
// module entirely.
func TestSilkModuleRootPrefersTheExecutable(t *testing.T) {
	body := funcBody(t, "design.go", "func silkModuleRoot() string {")
	exe := strings.Index(body, "os.Executable()")
	cwd := strings.Index(body, "os.Getwd()")
	if exe < 0 {
		t.Fatal("silkModuleRoot no longer looks at the executable; a shortcut-launched designer cannot build")
	}
	if cwd >= 0 && cwd < exe {
		t.Error("silkModuleRoot consults the cwd before the executable; the launcher's directory would win")
	}
}

// TestPreviewAppGetsARunnableEnvironment guards the last step of Run: the
// program is built into a temp directory, so nothing there can resolve the
// cairo libraries silk links, and it died with STATUS_DLL_NOT_FOUND the moment
// the designer's own working directory was outside the source tree.
func TestPreviewAppGetsARunnableEnvironment(t *testing.T) {
	body := funcBody(t, "design.go", "func onRun() {")
	if !strings.Contains(body, "runCmd.Env = runAppEnv()") {
		t.Error("the preview program inherits the designer's PATH unchanged; it cannot find its runtime libraries")
	}
	if !strings.Contains(body, "runCmd.Dir = moduleRoot") {
		t.Error("the preview program inherits the designer's working directory, which may be anywhere")
	}

	env := runAppEnv()
	var path string
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			path = kv[len("PATH="):]
		}
	}
	if path == "" {
		t.Fatal("runAppEnv did not set PATH")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Skip("cannot locate the test binary")
	}
	if dirs := filepath.SplitList(path); len(dirs) == 0 || dirs[0] != filepath.Dir(exe) {
		t.Errorf("PATH does not lead with the program's own directory: %q", path)
	}
}
