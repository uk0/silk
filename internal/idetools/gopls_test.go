package idetools

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

func put(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
}
func TestResolveBundleOverrideAndDeveloperFallback(t *testing.T) {
	for _, goos := range []string{"windows", "darwin", "linux"} {
		t.Run(goos, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "Silk IDE spaces 中文")
			exe := filepath.Join(root, "silkide")
			if goos == "darwin" {
				exe = filepath.Join(root, "SilkIDE.app", "Contents", "MacOS", "silkide")
			}
			binary := filepath.Join(filepath.Dir(exe), "tools", BinaryName(goos))
			put(t, binary)
			calls := 0
			look := func(string) (string, error) { calls++; return "", exec.ErrNotFound }
			check := func(dir, os, arch string) (Manifest, error) {
				if filepath.Join(dir, BinaryName(os)) != binary || arch != "amd64" {
					t.Fatal(dir, os, arch)
				}
				return Manifest{Version: GoplsVersion}, nil
			}
			got, err := resolve(exe, "", goos, "amd64", look, check)
			if err != nil || got.Path != binary || got.Source != "bundled" || calls != 0 {
				t.Fatal(got, err, calls)
			}
			bad := func(string, string, string) (Manifest, error) { return Manifest{}, errors.New("corrupt") }
			if _, err = resolve(exe, "", goos, "amd64", look, bad); err == nil || calls != 0 {
				t.Fatal("corrupt bundle fell back", err, calls)
			}
			override := filepath.Join(root, "custom-gopls")
			put(t, override)
			got, err = resolve(exe, override, goos, "amd64", look, bad)
			if err != nil || got.Source != "override" || got.Path != override {
				t.Fatal(got, err)
			}
			if _, err = resolve(exe, "./gopls", goos, "amd64", look, check); err == nil {
				t.Fatal("relative override accepted")
			}
			if _, err = resolve(exe, root, goos, "amd64", look, check); err == nil {
				t.Fatal("directory override accepted")
			}
			if err = os.Remove(binary); err != nil {
				t.Fatal(err)
			}
			if _, err = resolve(exe, "", goos, "amd64", look, check); err == nil || calls != 1 {
				t.Fatal(err, calls)
			}
			got, err = resolve(exe, "", goos, "amd64", func(string) (string, error) { return override, nil }, check)
			if err != nil || got.Source != "PATH" {
				t.Fatal(got, err)
			}
			if _, err = resolve(exe, "", goos, "amd64", func(string) (string, error) { return "./gopls", nil }, check); err == nil {
				t.Fatal("workspace PATH program accepted")
			}
			put(t, filepath.Join(filepath.Dir(binary), "gopls.json"))
			if _, err = resolve(exe, "", goos, "amd64", look, bad); err == nil || !strings.Contains(err.Error(), "bundled") {
				t.Fatal("missing binary hidden", err)
			}
		})
	}
}
func TestPinnedBuildMetadata(t *testing.T) {
	fresh := func() *debug.BuildInfo {
		return &debug.BuildInfo{GoVersion: BuildToolchain, Path: GoplsModule, Main: debug.Module{Path: GoplsModule, Version: GoplsVersion}, Settings: []debug.BuildSetting{{Key: "GOOS", Value: "windows"}, {Key: "GOARCH", Value: "amd64"}, {Key: "CGO_ENABLED", Value: "0"}, {Key: "GOAMD64", Value: AMD64Baseline}}}
	}
	if err := checkBuild(fresh(), "windows", "amd64"); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*debug.BuildInfo){func(i *debug.BuildInfo) { i.Main.Version = "v0.0.0" }, func(i *debug.BuildInfo) { i.Main.Replace = &debug.Module{} }, func(i *debug.BuildInfo) { i.GoVersion = "go1.1" }, func(i *debug.BuildInfo) { i.Settings[0].Value = "linux" }, func(i *debug.BuildInfo) { i.Settings[1].Value = "arm64" }, func(i *debug.BuildInfo) { i.Settings[2].Value = "1" }} {
		i := fresh()
		mutate(i)
		if err := checkBuild(i, "windows", "amd64"); err == nil {
			t.Fatal("wrong metadata accepted", i)
		}
	}
}
func TestDigestAndMalformedBundle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gopls")
	put(t, path)
	first, err := Digest(path)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("changed"), 0700)
	next, _ := Digest(path)
	if first == next {
		t.Fatal("digest ignored mutation")
	}
	for _, data := range []string{`{`, `{"unexpected":true}`, `{}` + strings.Repeat(" ", 16384) + `{}`} {
		os.WriteFile(filepath.Join(dir, "gopls.json"), []byte(data), 0600)
		if _, err := Verify(dir, "linux", "amd64"); err == nil {
			t.Fatal("broken bundle accepted")
		}
	}
}

func TestCompleteBuildRejectsEntireMissingBundle(t *testing.T) {
	old := RequireBundle
	RequireBundle = "1"
	defer func() { RequireBundle = old }()
	called := false
	_, err := resolve(filepath.Join(t.TempDir(), "silkide"), "", "linux", "amd64", func(string) (string, error) { called = true; return "/external/gopls", nil }, Verify)
	if err == nil || called || !strings.Contains(err.Error(), "tools directory") {
		t.Fatal(err, called)
	}
}

func TestExecutablePermissionsMatchHostFilesystem(t *testing.T) {
	p := filepath.Join(t.TempDir(), "gopls")
	if err := os.WriteFile(p, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	err := executableFile(p, "linux")
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatal("Windows cannot attest POSIX execute bits", err)
		}
	} else if err == nil {
		t.Fatal("non-executable Unix binary accepted")
	}
	if err := executableFile(p, "windows"); err != nil {
		t.Fatal(err)
	}
}

func TestMissingDeveloperGoplsRecoveryNamesCompleteBuild(t *testing.T) {
	_, err := resolve(filepath.Join(t.TempDir(), "silkide"), "", "linux", "amd64", func(string) (string, error) { return "", exec.ErrNotFound }, Verify)
	if !errors.Is(err, exec.ErrNotFound) || !strings.Contains(err.Error(), "make ide and launch bin/silkide") {
		t.Fatalf("missing actionable source build recovery: %v", err)
	}
	if strings.Contains(err.Error(), "make ide-tools") {
		t.Fatal("tool-only repair cannot repair a standalone root executable")
	}
}

func TestPortableBuildRejectsHostCPUFeatures(t *testing.T) {
	for _, tc := range []struct {
		arch, key, baseline string
		bad                 []string
	}{
		{"amd64", "GOAMD64", "v1", []string{"", "v2", "v3", "v4"}},
		{"arm64", "GOARM64", "v8.0", []string{"", "v8.0,lse", "v8.0,crypto", "v9.0"}},
	} {
		t.Run(tc.arch, func(t *testing.T) {
			info := &debug.BuildInfo{GoVersion: BuildToolchain, Path: GoplsModule, Main: debug.Module{Path: GoplsModule, Version: GoplsVersion}, Settings: []debug.BuildSetting{{Key: "GOOS", Value: "windows"}, {Key: "GOARCH", Value: tc.arch}, {Key: "CGO_ENABLED", Value: "0"}, {Key: tc.key, Value: tc.baseline}}}
			if err := checkBuild(info, "windows", tc.arch); err != nil {
				t.Fatal(err)
			}
			for _, value := range tc.bad {
				info.Settings[3].Value = value
				if err := checkBuild(info, "windows", tc.arch); err == nil {
					t.Errorf("accepted nonportable %s=%q", tc.key, value)
				}
			}
		})
	}
}
