package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/internal/idetools"
)

func TestToolModulePinsMatch(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("../..", defaultToolSource, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{idetools.GoplsModule + " " + idetools.GoplsVersion, "toolchain " + idetools.BuildToolchain} {
		if !strings.Contains(string(b), s) {
			t.Fatal("pin drift", s)
		}
	}
}
func TestBuildEnvDoesNotInheritCrossCompilerOrModEdits(t *testing.T) {
	e := buildEnv([]string{"PATH=/bin", "GOTOOLCHAIN=local", "GOROOT=/old", "GOWORK=/work", "GOFLAGS=-mod=mod", "CGO_ENABLED=1", "GOOS=linux", "GOARCH=arm64", "GOPROXY=https://proxy.golang.org"}, "windows", "amd64")
	joined := strings.Join(e, "\n")
	for _, bad := range []string{"GOTOOLCHAIN=local", "GOROOT=", "-mod=mod", "CGO_ENABLED=1", "GOARCH=arm64", "GOWORK=/work"} {
		if strings.Contains(joined, bad) {
			t.Fatal(joined)
		}
	}
	if !strings.Contains(joined, "GOOS=windows") || !strings.Contains(joined, "GOPROXY=") {
		t.Fatal(joined)
	}
}
func TestPublishPreservesUnmanagedDirectoriesAndReplacesTools(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "tools")
	os.Mkdir(dst, 0700)
	os.WriteFile(filepath.Join(dst, "user.go"), []byte("keep"), 0600)
	stage := filepath.Join(root, "stage")
	os.Mkdir(stage, 0700)
	os.WriteFile(filepath.Join(stage, "gopls"), []byte("new"), 0700)
	if err := publish(stage, dst); err == nil {
		t.Fatal("user directory replaced")
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "user.go")); string(b) != "keep" {
		t.Fatal("user data lost")
	}
	os.Remove(filepath.Join(dst, "user.go"))
	os.WriteFile(filepath.Join(dst, "gopls"), []byte("old"), 0700)
	if err := publish(stage, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "gopls")); string(b) != "new" {
		t.Fatal("not published")
	}
}

func TestPublishMustNotReplaceSourceToolModule(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "tools")
	module := filepath.Join(dst, "gopls", "go.mod")
	if err := os.MkdirAll(filepath.Dir(module), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(module, []byte("module keep"), 0600); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(root, "stage")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	if err := publish(stage, dst); err == nil {
		t.Fatal("source module overwritten")
	}
	if b, err := os.ReadFile(module); err != nil || string(b) != "module keep" {
		t.Fatal("source lost", err)
	}
}

// A source-only build in the checkout root uses executable-relative tools/.
// Keeping source at tools/gopls makes the resolver see a corrupt installed
// binary on Unix and prevents the documented developer PATH fallback.
func TestToolSourceDoesNotShadowRuntimeBinary(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		binary := filepath.Join("tools", idetools.BinaryName(goos))
		if filepath.Clean(defaultToolSource) == binary {
			t.Fatalf("tool source directory %q shadows %s bundled executable", defaultToolSource, goos)
		}
	}
}

func TestBuildEnvPinsPortableCPU(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			values := map[string]string{}
			for _, entry := range buildEnv([]string{"GOAMD64=v4", "goarm64=v9.0,lse,crypto", "GOPROXY=https://proxy.example.test"}, "windows", arch) {
				k, v, _ := strings.Cut(entry, "=")
				k = strings.ToUpper(k)
				if _, exists := values[k]; exists {
					t.Fatalf("duplicate environment key %s", k)
				}
				values[k] = v
			}
			if values["GOAMD64"] != "v1" || values["GOARM64"] != "v8.0" {
				t.Fatalf("host CPU features leaked into distribution: %v", values)
			}
			if values["GOPROXY"] != "https://proxy.example.test" {
				t.Fatal("proxy configuration lost")
			}
		})
	}
}
