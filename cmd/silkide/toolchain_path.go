package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Explorer often has a different PATH from a developer shell. Extend only
// this IDE process, after explicit PATH entries, with known absolute install
// directories. No registry changes, recursive disk scan or subprocess probes.
func configureIDEProcessToolPath() {
	candidates := []string{}
	for _, root := range []string{os.Getenv("GOROOT"), runtime.GOROOT()} {
		if root != "" {
			candidates = append(candidates, filepath.Join(root, "bin"))
		}
	}
	if runtime.GOOS == "windows" {
		for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
			if root != "" {
				candidates = append(candidates, filepath.Join(root, "Go", "bin"))
			}
		}
		if drive := os.Getenv("SystemDrive"); drive != "" {
			candidates = append(candidates, filepath.Join(drive+string(filepath.Separator), "Go", "bin"))
		}
	}
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			gopath = filepath.Join(home, "go")
		}
	}
	for _, root := range filepath.SplitList(gopath) {
		if root != "" {
			candidates = append(candidates, filepath.Join(root, "bin"))
		}
	}
	path := os.Getenv("PATH")
	if extended := appendExistingToolDirs(path, candidates, runtime.GOOS == "windows"); extended != path {
		_ = os.Setenv("PATH", extended)
	}
}

func appendExistingToolDirs(path string, dirs []string, caseInsensitive bool) string {
	key := func(s string) string {
		s = filepath.Clean(s)
		if caseInsensitive {
			return strings.ToLower(s)
		}
		return s
	}
	seen := map[string]bool{}
	for _, dir := range filepath.SplitList(path) {
		seen[key(dir)] = true
	}
	for _, dir := range dirs {
		if !filepath.IsAbs(dir) || seen[key(dir)] {
			continue
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		if path != "" {
			path += string(os.PathListSeparator)
		}
		path += dir
		seen[key(dir)] = true
	}
	return path
}
