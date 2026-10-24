// silktools builds and verifies the pinned, relocatable IDE language server.
package main

import (
	"context"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/uk0/silk/internal/idetools"
)

// Tool module sources must not occupy tools/gopls: that path is the Unix
// executable in a runtime bundle beside a source-only IDE build.
const defaultToolSource = "tools/gopls-module"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "silktools:", err)
		os.Exit(1)
	}
}
func run() error {
	out := flag.String("out", "bin/tools", "tool directory")
	source := flag.String("source", defaultToolSource, "pinned tool module directory")
	goos := flag.String("goos", runtime.GOOS, "target OS")
	arch := flag.String("goarch", runtime.GOARCH, "target architecture")
	verify := flag.Bool("verify", false, "verify only; never download/build")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	if !supported(*goos, *arch) {
		return fmt.Errorf("unsupported target %s/%s", *goos, *arch)
	}
	dir, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	if *verify {
		_, err := idetools.Verify(dir, *goos, *arch)
		return err
	}
	// Valid artifacts are reusable offline. Corruption/old versions are rebuilt
	// in a new sibling directory, and only a verified tree replaces the old one.
	if _, err := idetools.Verify(dir, *goos, *arch); err == nil {
		fmt.Println("verified cached", dir)
		return nil
	}
	src, err := filepath.Abs(*source)
	if err != nil {
		return err
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dir), ".silk-tools-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	env := buildEnv(os.Environ(), *goos, *arch)
	call := func(args ...string) ([]byte, error) {
		c := exec.CommandContext(ctx, goBin, args...)
		c.Dir = src
		c.Env = env
		return c.CombinedOutput()
	}
	if output, err := call("build", "-mod=readonly", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(stage, idetools.BinaryName(*goos)), idetools.GoplsModule); err != nil {
		return fmt.Errorf("build: %w\n%s", err, output)
	}
	output, err := call("list", "-mod=readonly", "-m", "-json", "all")
	if err != nil {
		return fmt.Errorf("module licenses: %w\n%s", err, output)
	}
	info, err := buildinfo.ReadFile(filepath.Join(stage, idetools.BinaryName(*goos)))
	if err != nil {
		return err
	}
	used := map[string]bool{info.Main.Path: true}
	for _, m := range info.Deps {
		used[m.Path] = true
	}
	if err = copyLicenses(output, stage, used); err != nil {
		return err
	}
	m, err := idetools.Describe(stage, *goos, *arch)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(stage, "gopls.json"), append(data, '\n'), 0644); err != nil {
		return err
	}
	if _, err = idetools.Verify(stage, *goos, *arch); err != nil {
		return err
	}
	if err = publish(stage, dir); err != nil {
		return err
	}
	fmt.Printf("bundled %s@%s for %s/%s: %s\n", m.Module, m.Version, m.GOOS, m.GOARCH, dir)
	return nil
}
func supported(goos, arch string) bool {
	return (goos == "windows" || goos == "linux" || goos == "darwin") && (arch == "amd64" || arch == "arm64")
}
func buildEnv(original []string, goos, arch string) []string {
	blocked := map[string]bool{"GOOS": true, "GOARCH": true, "CGO_ENABLED": true, "GOTOOLCHAIN": true, "GOROOT": true, "GOWORK": true, "GOFLAGS": true, "GOBIN": true, "GOAMD64": true, "GOARM64": true}
	var result []string
	for _, s := range original {
		k, _, _ := strings.Cut(s, "=")
		if !blocked[strings.ToUpper(k)] {
			result = append(result, s)
		}
	}
	return append(result, "GOOS="+goos, "GOARCH="+arch, "CGO_ENABLED=0", "GOTOOLCHAIN="+idetools.BuildToolchain, "GOWORK=off", "GOFLAGS=", "GOAMD64="+idetools.AMD64Baseline, "GOARM64="+idetools.ARM64Baseline)
}
func copyLicenses(data []byte, stage string, used map[string]bool) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	for {
		var m struct {
			Path, Version, Dir string
			Main               bool
		}
		if err := decoder.Decode(&m); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if m.Main || !used[m.Path] {
			continue
		}
		if m.Dir == "" {
			return fmt.Errorf("module directory missing: %s", m.Path)
		}
		entries, err := os.ReadDir(m.Dir)
		if err != nil {
			return err
		}
		name := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(m.Path) + "@" + m.Version
		if m.Path == idetools.GoplsModule {
			name = "gopls"
		}
		delete(used, m.Path)
		found := false
		for _, e := range entries {
			upper := strings.ToUpper(e.Name())
			if !(strings.HasPrefix(upper, "LICENSE") || strings.HasPrefix(upper, "COPYING") || strings.HasPrefix(upper, "NOTICE")) || !e.Type().IsRegular() {
				continue
			}
			b, err := os.ReadFile(filepath.Join(m.Dir, e.Name()))
			if err != nil {
				return err
			}
			dst := filepath.Join(stage, "licenses", name)
			if err = os.MkdirAll(dst, 0755); err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(dst, e.Name()), b, 0644); err != nil {
				return err
			}
			found = true
		}
		if !found {
			return fmt.Errorf("license unavailable for %s@%s; refusing incomplete distribution", m.Path, m.Version)
		}
	}
	if len(used) != 0 {
		return fmt.Errorf("linked module licenses not found: %v", used)
	}
	return nil
}
func publish(stage, dst string) error {
	// Rename complete directories on the same volume. Preserve the old tree on
	// Windows sharing violations and never remove it before the new tree exists.
	if info, err := os.Lstat(dst); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("tool destination is not a real directory")
		}
		entries, err := os.ReadDir(dst)
		if err != nil {
			return err
		}
		for _, e := range entries {
			switch e.Name() {
			case "gopls", "gopls.exe", "gopls.json":
				if !e.Type().IsRegular() {
					return fmt.Errorf("refusing to replace non-file tool entry: %s", e.Name())
				}
			case "licenses":
				if !e.IsDir() {
					return errors.New("tool licenses entry is not a directory")
				}
			default:
				return fmt.Errorf("refusing to replace unmanaged tool directory entry: %s", e.Name())
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	backup, err := os.MkdirTemp(filepath.Dir(dst), ".silk-tools-backup-*")
	if err != nil {
		return err
	}
	if err = os.Remove(backup); err != nil {
		return err
	}
	existed := false
	if _, err = os.Lstat(dst); err == nil {
		if err = os.Rename(dst, backup); err != nil {
			return err
		}
		existed = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err = os.Rename(stage, dst); err != nil {
		if existed {
			if restore := os.Rename(backup, dst); restore != nil {
				return errors.Join(err, fmt.Errorf("restore failed; previous tools retained at %s: %w", backup, restore))
			}
		}
		return err
	}
	if existed {
		if err = os.RemoveAll(backup); err != nil {
			return fmt.Errorf("new tools installed; old tools retained at %s: %w", backup, err)
		}
	}
	return nil
}
