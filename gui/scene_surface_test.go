package gui

import (
	"context"
	"errors"
	"image/color"
	"math"
	"testing"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/scene3d"
)

func surfaceIndex(t *testing.T, v *SceneView, old *scene3d.RayIndex) *scene3d.RayIndex {
	t.Helper()
	snap, err := v.Scene().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	// Build on a worker, never read mutable view/scene state there.
	type result struct {
		index *scene3d.RayIndex
		err   error
	}
	done := make(chan result, 1)
	go func() { idx, e := old.WithSnapshot(t.Context(), snap); done <- result{idx, e} }()
	r := <-done
	if r.err != nil {
		t.Fatal(r.err)
	}
	return r.index
}
func TestSceneSurfacePreparedHiDPIAndNoRender(t *testing.T) {
	v := NewSceneView()
	defer v.Close()
	v.SetRenderBackend("cpu")
	v.SetSize(240, 180)
	v.SetYaw(0)
	v.SetPitch(0)
	idx := surfaceIndex(t, v, nil)
	for _, scale := range []float64{1, 1.25, 2, 3} {
		v.renderScaleX = scale
		v.renderScaleY = scale
		h, ok, stats, err := v.SurfaceAt(t.Context(), idx, 120, 90, scene3d.RayQuery{})
		if err != nil || !ok || h.NodeID != "model" || h.Position != (scene3d.Vec3{Z: 1}) || h.Normal != (scene3d.Vec3{Z: 1}) || stats.TriangleTests == 0 {
			t.Fatal(scale, h, ok, stats, err)
		}
		if v.RenderCount() != 0 || v.gpuAttempted || v.SelectedNode() != "" {
			t.Fatal("surface query caused rendering, GPU init or selection")
		}
	}
	v.SetYaw(.4)
	v.SetPitch(.2)
	h, ok, _, err := v.SurfaceAt(t.Context(), idx, 120, 90, scene3d.RayQuery{})
	if err != nil || !ok || h.NodeID != "model" {
		t.Fatal(h, ok, err)
	}
	// Pixel picking is still its own path and actually rasterizes.
	if v.PickNode(120, 90) != "model" || v.RenderCount() != 1 {
		t.Fatal("pixel pick contract changed")
	}
}
func TestSceneSurfaceStaleTransformColorSceneAndVisibility(t *testing.T) {
	v := NewSceneView()
	defer v.Close()
	v.SetSize(240, 180)
	v.SetYaw(0)
	v.SetPitch(0)
	idx := surfaceIndex(t, v, nil)
	for _, edit := range []func() error{
		func() error { return v.SetNodeColor("model", color.RGBA{R: 255, A: 255}) },
		func() error { return v.SetNodeTransform("model", scene3d.Translation(scene3d.Vec3{Z: 2})) },
		func() error { return v.SetNodeVisible("model", false) },
	} {
		if err := edit(); err != nil {
			t.Fatal(err)
		}
		_, ok, _, err := v.SurfaceAt(t.Context(), idx, 120, 90, scene3d.RayQuery{})
		if ok || !errors.Is(err, ErrSceneSurfaceStale) {
			t.Fatal(ok, err)
		}
		next := surfaceIndex(t, v, idx)
		if next == idx {
			t.Fatal("stale index reused")
		}
		idx = next
	}
	_, ok, _, err := v.SurfaceAt(t.Context(), idx, 120, 90, scene3d.RayQuery{})
	if ok || err != nil {
		t.Fatal("hidden hit", ok, err)
	}
	other := scene3d.NewScene()
	_ = other.Add(scene3d.Node{ID: "other", Mesh: scene3d.Box()})
	_ = v.SetScene(other)
	_, _, _, err = v.SurfaceAt(t.Context(), idx, 120, 90, scene3d.RayQuery{})
	if !errors.Is(err, ErrSceneSurfaceStale) {
		t.Fatal(err)
	}
	idx = surfaceIndex(t, v, idx)
	replacement := scene3d.NewScene()
	_ = replacement.Add(scene3d.Node{ID: "different", Mesh: scene3d.Box()})
	if replacement.Revision() != other.Revision() {
		t.Fatal("fixture revisions differ")
	}
	_ = v.SetScene(replacement)
	_, _, _, err = v.SurfaceAt(t.Context(), idx, 120, 90, scene3d.RayQuery{})
	if !errors.Is(err, ErrSceneSurfaceStale) {
		t.Fatal("equal revision foreign scene accepted", err)
	}
}
func TestSceneSurfaceCameraClippingAndValidation(t *testing.T) {
	v := NewSceneView()
	defer v.Close()
	v.SetSize(240, 180)
	v.SetYaw(0)
	v.SetPitch(0)
	mesh, err := scene3d.NewMesh([]scene3d.Vec3{{X: -10000, Y: -10000}, {X: 10000, Y: -10000}, {Y: 10000}}, []uint32{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	s := scene3d.NewScene()
	_ = s.Add(scene3d.Node{ID: "plane", Mesh: mesh})
	_ = v.SetScene(s)
	c := v.camera()
	for _, tc := range []struct {
		z    float64
		want bool
	}{{c.Eye.Z - c.Near*.5, false}, {0, true}, {c.Eye.Z - c.Far*.99, true}, {c.Eye.Z - c.Far*1.01, false}} {
		_ = v.SetNodeTransform("plane", scene3d.Translation(scene3d.Vec3{Z: tc.z}))
		idx := surfaceIndex(t, v, nil)
		for _, x := range []float64{120, 150} {
			_, ok, _, err := v.SurfaceAt(t.Context(), idx, x, 90, scene3d.RayQuery{})
			if err != nil || ok != tc.want {
				t.Fatal(tc, x, ok, err)
			}
		}
	}
	_ = v.SetNodeTransform("plane", scene3d.Identity())
	idx := surfaceIndex(t, v, nil)
	for _, q := range []scene3d.RayQuery{{MaxDistance: 1}, {MinDistance: 7000}, {SkipNode: "plane"}} {
		_, ok, _, err := v.SurfaceAt(t.Context(), idx, 120, 90, q)
		if err != nil || ok {
			t.Fatal(q, ok, err)
		}
	}
	for _, q := range []scene3d.RayQuery{{MinDistance: -1}, {MaxDistance: -1}, {MinDistance: 5, MaxDistance: 4}, {MinDistance: math.NaN()}, {MaxDistance: math.Inf(1)}} {
		_, _, _, err := v.SurfaceAt(t.Context(), idx, 120, 90, q)
		if !errors.Is(err, ErrUIInvalid) {
			t.Fatal(q, err)
		}
	}
	for _, p := range [][2]float64{{-1, 0}, {240, 0}, {0, 180}} {
		_, ok, _, err := v.SurfaceAt(t.Context(), idx, p[0], p[1], scene3d.RayQuery{})
		if err != nil || ok {
			t.Fatal(p, ok, err)
		}
	}
	_, _, _, err = v.SurfaceAt(t.Context(), nil, 120, 90, scene3d.RayQuery{})
	if !errors.Is(err, ErrSceneSurfaceStale) {
		t.Fatal(err)
	}
	_, _, _, err = v.SurfaceAt(nil, idx, 120, 90, scene3d.RayQuery{})
	if !errors.Is(err, ErrUIInvalid) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, _, err = v.SurfaceAt(ctx, idx, 120, 90, scene3d.RayQuery{})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	_, _, _, err = v.SurfaceAt(t.Context(), idx, math.NaN(), 0, scene3d.RayQuery{})
	if !errors.Is(err, ErrUIInvalid) {
		t.Fatal(err)
	}
	v.SetSize(0, 0)
	_, _, _, err = v.SurfaceAt(t.Context(), idx, 0, 0, scene3d.RayQuery{})
	if !errors.Is(err, ErrUIInvalid) {
		t.Fatal(err)
	}
	v.Close()
	_, _, _, err = v.SurfaceAt(t.Context(), idx, 0, 0, scene3d.RayQuery{})
	if !errors.Is(err, async.ErrClosed) {
		t.Fatal(err)
	}
}
