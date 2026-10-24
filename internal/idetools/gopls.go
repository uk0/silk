// Package idetools defines the IDE's packaged language-server contract.
// It does not install programs or modify the machine's PATH at runtime.
package idetools

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
)

const (
	GoplsModule    = "golang.org/x/tools/gopls"
	GoplsVersion   = "v0.24.0"
	BuildToolchain = "go1.27.3"
	AMD64Baseline  = "v1"
	ARM64Baseline  = "v8.0"
)

// RequireBundle is set to "1" by complete IDE builds (-ldflags -X).
// A source-only go build can still use a developer PATH installation.
var RequireBundle string

type Manifest struct {
	Schema    int    `json:"schema"`
	Module    string `json:"module"`
	Version   string `json:"version"`
	Toolchain string `json:"toolchain"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	SHA256    string `json:"sha256"`
}

type Tool struct{ Path, Source, Version string }

// RecoveryHint names a complete build and its output, not just a tool repair.
// make ide-tools writes bin/tools and cannot repair a standalone root binary.
const RecoveryHint = "Reinstall the complete SilkIDE package. Source checkout: run make ide and launch bin/silkide (Windows: bin/silkide.exe)."

func BinaryName(goos string) string {
	if goos == "windows" {
		return "gopls.exe"
	}
	return "gopls"
}

func Digest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func checkBuild(info *debug.BuildInfo, goos, goarch string) error {
	if info.Path != GoplsModule || info.Main.Path != GoplsModule || info.Main.Version != GoplsVersion || info.Main.Replace != nil {
		return fmt.Errorf("expected %s@%s, found %s@%s", GoplsModule, GoplsVersion, info.Main.Path, info.Main.Version)
	}
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	if info.GoVersion != BuildToolchain || settings["GOOS"] != goos || settings["GOARCH"] != goarch || settings["CGO_ENABLED"] != "0" {
		return fmt.Errorf("incompatible gopls build: compiler=%s target=%s/%s CGO=%s; expected %s %s/%s CGO=0", info.GoVersion, settings["GOOS"], settings["GOARCH"], settings["CGO_ENABLED"], BuildToolchain, goos, goarch)
	}
	// A matching GOARCH alone is insufficient: developer GOAMD64/GOARM64
	// settings can require newer instructions and crash on supported machines.
	key, baseline := "", ""
	switch goarch {
	case "amd64":
		key, baseline = "GOAMD64", AMD64Baseline
	case "arm64":
		key, baseline = "GOARM64", ARM64Baseline
	default:
		return fmt.Errorf("unsupported gopls architecture: %s", goarch)
	}
	if settings[key] != baseline {
		return fmt.Errorf("nonportable gopls build: %s=%q; expected %s", key, settings[key], baseline)
	}
	return nil
}

func Describe(dir, goos, goarch string) (Manifest, error) {
	path := filepath.Join(dir, BinaryName(goos))
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	if err = checkBuild(info, goos, goarch); err != nil {
		return Manifest{}, err
	}
	sum, err := Digest(path)
	if err != nil {
		return Manifest{}, err
	}
	return Manifest{1, GoplsModule, GoplsVersion, BuildToolchain, goos, goarch, sum}, nil
}

// Verify checks integrity/target/version, not publisher authenticity or signing.
// Relocating a complete bundle does not change any of these checks.
func Verify(dir, goos, goarch string) (Manifest, error) {
	path := filepath.Join(dir, BinaryName(goos))
	if err := executableFile(path, goos); err != nil {
		return Manifest{}, err
	}
	f, err := os.Open(filepath.Join(dir, "gopls.json"))
	if err != nil {
		return Manifest{}, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return Manifest{}, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > 16<<10 {
		return Manifest{}, errors.New("invalid gopls manifest file/size")
	}
	var want Manifest
	decoder := json.NewDecoder(io.LimitReader(f, 16<<10))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&want); err != nil {
		return Manifest{}, err
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return Manifest{}, errors.New("invalid gopls manifest trailer")
	}
	got, err := Describe(dir, goos, goarch)
	if err != nil {
		return Manifest{}, err
	}
	if want != got {
		return Manifest{}, errors.New("gopls manifest/version/platform/SHA-256 mismatch; reinstall the complete IDE bundle")
	}
	info, err := os.Stat(filepath.Join(dir, "licenses", "gopls", "LICENSE"))
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return Manifest{}, errors.New("gopls license missing from bundle")
	}
	return got, nil
}

func executableFile(path, goos string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	// Windows filesystems do not expose POSIX executable bits, even when
	// inspecting a Unix cross-build. Enforce them on Unix hosts/targets only.
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && goos != "windows" && info.Mode().Perm()&0111 == 0) {
		return fmt.Errorf("not an executable file: %s", path)
	}
	return nil
}

// Resolve prefers an explicit absolute override, then executable-relative tools,
// then a developer PATH installation. A damaged bundle is never silently hidden
// by a PATH fallback. It never searches the workspace or changes global PATH.
func Resolve() (Tool, error) {
	exe, err := os.Executable()
	if err != nil {
		return Tool{}, err
	}
	if real, e := filepath.EvalSymlinks(exe); e == nil {
		exe = real
	}
	return resolve(exe, os.Getenv("SILKIDE_GOPLS"), runtime.GOOS, runtime.GOARCH, exec.LookPath, Verify)
}
func resolve(exe, override, goos, goarch string, look func(string) (string, error), verify func(string, string, string) (Manifest, error)) (Tool, error) {
	if override != "" {
		if !filepath.IsAbs(override) {
			return Tool{}, errors.New("SILKIDE_GOPLS must be an absolute executable path (no shell arguments)")
		}
		if err := executableFile(override, goos); err != nil {
			return Tool{}, fmt.Errorf("SILKIDE_GOPLS: %w", err)
		}
		return Tool{filepath.Clean(override), "override", "external"}, nil
	}
	dir := filepath.Join(filepath.Dir(exe), "tools")
	// Either file establishes an installed-bundle contract. Missing companions
	// are packaging errors, not an excuse to execute an unrelated PATH program.
	for _, name := range []string{BinaryName(goos), "gopls.json"} {
		_, err := os.Lstat(filepath.Join(dir, name))
		if err == nil || !errors.Is(err, os.ErrNotExist) {
			m, err := verify(dir, goos, goarch)
			if err != nil {
				return Tool{}, fmt.Errorf("bundled gopls: %w", err)
			}
			return Tool{filepath.Join(dir, BinaryName(goos)), "bundled", m.Version}, nil
		}
	}
	if RequireBundle == "1" {
		return Tool{}, errors.New("bundled gopls is missing; reinstall the complete SilkIDE package (tools directory required)")
	}
	path, err := look("gopls")
	if err != nil {
		return Tool{}, fmt.Errorf("gopls unavailable: %s Explicit override: SILKIDE_GOPLS: %w", RecoveryHint, err)
	}
	// Reject relative PATH entries and Windows' implicit current-directory lookup.
	if !filepath.IsAbs(path) {
		return Tool{}, errors.New("refusing workspace-relative gopls from PATH")
	}
	if strings.TrimSpace(path) == "" {
		return Tool{}, errors.New("empty gopls path")
	}
	if err = executableFile(path, goos); err != nil {
		return Tool{}, err
	}
	return Tool{filepath.Clean(path), "PATH", "external"}, nil
}
