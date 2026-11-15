package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/hmi"
)

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
