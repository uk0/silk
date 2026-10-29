// scene3dplant assembles a reusable device model into a portable industrial scene.
// Run from the repository root; it does not start a UI, driver or subscription.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"

	"github.com/uk0/silk/scene3d"
)

func assemble(ctx context.Context, source *scene3d.Document, count, columns int, spacing float64) (*scene3d.Document, error) {
	if count < 1 || count > scene3d.MaxNodes || columns < 1 || columns > count || spacing <= 0 || math.IsNaN(spacing) || math.IsInf(spacing, 0) {
		return nil, fmt.Errorf("count, columns and spacing must describe a finite positive grid")
	}
	template, err := scene3d.NewTemplate(ctx, source)
	if err != nil {
		return nil, err
	}
	if count > (scene3d.MaxNodes-1)/(template.NodeCount()+1) {
		return nil, scene3d.ErrLimit
	}
	scene := scene3d.NewScene()
	if err = scene.Add(scene3d.Node{ID: "plant", Name: "Plant assembly"}); err != nil {
		return nil, err
	}
	placements := make([]scene3d.InstanceOptions, count)
	for i := range placements {
		id := fmt.Sprintf("device-%04d", i+1)
		placements[i] = scene3d.InstanceOptions{
			ID: id, Parent: "plant", Name: fmt.Sprintf("Device %d", i+1), TagPrefix: id + ".",
			Transform: scene3d.Translation(scene3d.Vec3{X: float64(i%columns) * spacing, Z: float64(i/columns) * spacing}),
		}
	}
	instances, err := template.InstantiateBatch(ctx, scene, placements)
	if err != nil {
		return nil, err
	}
	var bindings []scene3d.Binding
	for _, instance := range instances {
		bindings = append(bindings, instance.Bindings()...)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	document, err := scene3d.CaptureDocument(scene, bindings)
	if err != nil {
		return nil, err
	}
	document.Warnings = template.Warnings()
	return document, ctx.Err()
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("scene3dplant", flag.ContinueOnError)
	flags.SetOutput(stdout)
	model := flags.String("model", "examples/scene3d/assembly.silkscene", "authored OBJ, glTF, GLB or .silkscene device model")
	out := flags.String("out", "", "required destination .silkscene path (replaces an existing regular file)")
	count := flags.Int("count", 25, "number of independent devices")
	columns := flags.Int("columns", 5, "devices per grid row")
	spacing := flags.Float64("spacing", 6, "distance between device origins in model units")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *out == "" || flags.NArg() != 0 {
		return fmt.Errorf("specify -out and use named flags only")
	}
	loaded, err := scene3d.LoadModelFile(ctx, *model, scene3d.LoadLimits{})
	if err != nil {
		return err
	}
	source, err := scene3d.CaptureDocument(loaded.Scene, loaded.Bindings)
	if err != nil {
		return err
	}
	source.Warnings = loaded.Warnings
	document, err := assemble(ctx, source, *count, *columns, *spacing)
	if err != nil {
		return err
	}
	if err = scene3d.SaveDocumentFile(ctx, *out, document); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(struct {
		Output       string   `json:"output"`
		Devices      int      `json:"devices"`
		Nodes        int      `json:"nodes"`
		UniqueMeshes int      `json:"uniqueMeshes"`
		Bindings     int      `json:"bindings"`
		Warnings     []string `json:"warnings,omitempty"`
	}{*out, *count, len(document.Nodes), len(document.Meshes), len(document.Bindings), document.Warnings})
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
