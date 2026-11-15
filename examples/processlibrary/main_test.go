package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/hmi"
)

func TestDeviceLibraryCLIClosesFrameworkOnSuccessAndFailure(t *testing.T) {
	if os.Getenv("SILK_PROCESS_LIBRARY_CHILD") == "1" {
		os.Args = []string{"processlibrary", "-out", os.Getenv("SILK_PROCESS_LIBRARY_OUT")}
		if mode := os.Getenv("SILK_PROCESS_LIBRARY_MODE"); mode != "" {
			os.Args = []string{"processlibrary", mode}
		}
		main()
		os.Exit(0)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, out := t.TempDir(), t.TempDir()
	for i := 0; i < 5; i++ {
		if i == 4 {
			out = t.TempDir()
		}
		cmd := exec.Command(exe, "-test.run=^TestDeviceLibraryCLIClosesFrameworkOnSuccessAndFailure$")
		mode := ""
		if i == 2 {
			mode = "-invalid-flag"
		}
		if i == 3 {
			mode = "-h"
		}
		cmd.Env = append(os.Environ(), "SILK_PROCESS_LIBRARY_CHILD=1", "SILK_PROCESS_LIBRARY_OUT="+out, "SILK_DATA_DIR="+data, "SILK_PROCESS_LIBRARY_MODE="+mode)
		text, err := cmd.CombinedOutput()
		want := 0
		if i == 1 {
			want = 1
		}
		if i == 2 {
			want = 2
		}
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		if code != want {
			t.Fatalf("CLI %d exit %d want %d: %s", i, code, want, text)
		}
		if strings.Contains(string(text), "runlock") || strings.Contains(string(text), "core.Close()") {
			t.Fatalf("unclean CLI lifecycle: %s", text)
		}
		if err := filepath.WalkDir(data, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if strings.HasSuffix(path, ".runlock") {
				t.Errorf("CLI %d left run marker %s", i, path)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDeviceLibraryExampleRoundTripAndNoClobber(t *testing.T) {
	dir := t.TempDir()
	if err := run(dir); err != nil {
		t.Fatal(err)
	}
	p, err := hmi.Load(filepath.Join(dir, "plant.silkhmi"))
	if err != nil || len(p.Nodes) != 13 || len(p.Links) != 5 || len(p.Points) != 8 || len(p.Sources) != 0 {
		t.Fatal("project readback", p, err)
	}
	l, err := hmi.LoadDeviceLibrary(context.Background(), filepath.Join(dir, "devices.silkhmilibrary"))
	if err != nil || len(l.Devices) != 4 {
		t.Fatal("library readback", err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "plant.silkhmi"))
	if err != nil {
		t.Fatal(err)
	}
	if err := run(dir); err == nil {
		t.Fatal("existing library was overwritten")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "plant.silkhmi"))
	if string(after) != string(before) {
		t.Fatal("existing project changed")
	}
}
