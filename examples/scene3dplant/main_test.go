package main

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/scene3d"
)

func TestPlantCommandLoadComposeSaveAndReadback(t *testing.T) {
	ctx := context.Background()
	out := filepath.Join(t.TempDir(), "plant.silkscene")
	var summary bytes.Buffer
	if err := run(ctx, []string{"-model", "../scene3d/assembly.silkscene", "-out", out, "-count", "1000", "-columns", "40"}, &summary); err != nil {
		t.Fatal(err)
	}
	var result struct{ Devices, Nodes, UniqueMeshes, Bindings int }
	if err := json.Unmarshal(summary.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Devices != 1000 || result.Nodes != 9001 || result.UniqueMeshes != 2 || result.Bindings != 3000 {
		t.Fatal(result)
	}
	loaded, err := scene3d.LoadModelFile(ctx, out, scene3d.LoadLimits{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := scene3d.NewBindingSet(loaded.Scene, loaded.Bindings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = plan.Apply("device-1000.tank.level", 1500.0); err != nil {
		t.Fatal(err)
	}
	last, ok := loaded.Scene.Node("device-1000")
	if !ok || last.Transform[12] != 39*6 || last.Transform[14] != 24*6 {
		t.Fatal(last, ok)
	}
	first, _ := loaded.Scene.Node("device-0001/model/node/1/primitive/0")
	other, _ := loaded.Scene.Node("device-1000/model/node/1/primitive/0")
	if first.Mesh == nil || other.Mesh != first.Mesh {
		t.Fatal("geometry was not shared after file roundtrip")
	}
	before, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// Invalid/cancelled builds must never truncate a previously saved plant.
	for _, args := range [][]string{
		{"-count", "0"}, {"-count", "2000", "-columns", "40"}, {"-spacing", "NaN"}, {"-model", "missing.obj"}, {"unexpected"},
	} {
		base := []string{"-model", "../scene3d/assembly.silkscene", "-out", out}
		if err = run(ctx, append(base, args...), &summary); err == nil {
			t.Fatal("accepted invalid arguments", args)
		}
		after, _ := os.ReadFile(out)
		if !bytes.Equal(before, after) {
			t.Fatal("failure replaced destination")
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = run(cancelled, []string{"-model", "../scene3d/assembly.silkscene", "-out", out}, &summary); err == nil {
		t.Fatal("ignored cancellation")
	}
	after, _ := os.ReadFile(out)
	if !bytes.Equal(before, after) {
		t.Fatal("cancel truncated destination")
	}
	if err = run(ctx, nil, &summary); err == nil {
		t.Fatal("implicit output")
	}
}

func TestPlantGridRejectsInvalidDimensions(t *testing.T) {
	d := &scene3d.Document{Version: scene3d.DocumentVersion}
	for _, tt := range []struct {
		count, columns int
		spacing        float64
	}{
		{0, 1, 1}, {1, 0, 1}, {1, 2, 1}, {1, 1, 0}, {1, 1, -1}, {1, 1, math.NaN()}, {1, 1, math.Inf(1)}, {scene3d.MaxNodes + 1, 1, 1},
	} {
		if _, err := assemble(context.Background(), d, tt.count, tt.columns, tt.spacing); err == nil {
			t.Fatal(tt)
		}
	}
}
