//go:build cgo && !silk_pure_go

package glrender

import (
	"bytes"
	"fmt"
	"image/color"
	"testing"

	"github.com/uk0/silk/scene3d"
)

var nativeBVHRan bool
var nativeBVHErr error

func TestNativeBVHOrderingRefitAndValidationCache(t *testing.T) {
	if !nativeRan {
		t.Skip("set SILK_TEST_SCENE_GPU=1 for actual context validation")
	}
	if !nativeBVHRan {
		t.Fatalf("context failed before BVH probe: %v", nativeErr)
	}
	if nativeBVHErr != nil {
		t.Fatal(nativeBVHErr)
	}
}

func bvhNativeParity(fast, reference *Renderer, s *scene3d.Snapshot, c scene3d.Camera) error {
	a, err := fast.Render(s, c, 96, 72, color.RGBA{})
	if err != nil {
		return err
	}
	unculled, err := uncullableNativeSnapshot(s)
	if err != nil {
		return err
	}
	b, err := reference.Render(unculled, c, 96, 72, color.RGBA{})
	if err != nil {
		return err
	}
	if b.Stats.CulledInstances != 0 || b.Stats.DrawCalls != s.InstanceCount() {
		return fmt.Errorf("BVH reference did not draw all instances: %+v", b.Stats)
	}
	if !bytes.Equal(a.Image.Pix, b.Image.Pix) || !bytes.Equal(a.pickPixels, b.pickPixels) {
		return fmt.Errorf("BVH changed exact RGBA or picking bytes")
	}
	return nil
}

func nativeBVHProbe() (result error) {
	r, err := NewCurrent()
	if err != nil {
		return err
	}
	defer r.Close()
	ref, err := NewCurrentWithOptions(Options{Instancing: InstancingDisabled})
	if err != nil {
		return err
	}
	defer ref.Close()
	scene := scene3d.NewScene()
	mesh := scene3d.Box()
	for i := 0; i < 1000; i++ {
		pose := scene3d.Translation(scene3d.Vec3{X: 1000 + float64(i%40)*3, Y: float64(i/40) * 3})
		if i == 500 {
			pose = scene3d.Identity()
		}
		if err = scene.Add(scene3d.Node{ID: fmt.Sprint(i), Mesh: mesh, Transform: pose}); err != nil {
			return err
		}
	}
	snap, err := scene.Snapshot()
	if err != nil {
		return err
	}
	original := snap
	camera := scene3d.DefaultCamera()
	camera.Eye = scene3d.Vec3{Z: 6}
	frame, err := r.Render(snap, camera, 96, 72, color.RGBA{})
	if err != nil {
		return err
	}
	if frame.Stats.ValidatedInstances != 1000 || frame.Stats.Culling.RefittedNodes == 0 || frame.Stats.Culling.InstanceTests > 16 || frame.Stats.CulledInstances != 999 || frame.Pick(48, 36) != "500" {
		return fmt.Errorf("BVH initial draw: %+v", frame.Stats)
	}
	if err = bvhNativeParity(r, ref, snap, camera); err != nil {
		return err
	}
	cases := 1
	for i := 0; i < 16; i++ {
		camera.Eye.X += .01
		frame, err = r.Render(snap, camera, 96, 72, color.RGBA{})
		if err != nil {
			return err
		}
		if frame.Stats.Cached || frame.Stats.ValidatedInstances != 0 || frame.Stats.Culling.RefittedNodes != 0 || frame.Stats.MeshUploads != 0 {
			return fmt.Errorf("camera repeated immutable snapshot preparation: %+v", frame.Stats)
		}
		if err = bvhNativeParity(r, ref, snap, camera); err != nil {
			return err
		}
		cases++
	}
	if err = scene.SetColor("500", color.RGBA{R: 250, G: 30, B: 40, A: 255}); err != nil {
		return err
	}
	snap, err = scene.Snapshot()
	if err != nil {
		return err
	}
	frame, err = r.Render(snap, camera, 96, 72, color.RGBA{})
	if err != nil || frame.Stats.ValidatedInstances != 1000 || frame.Stats.Culling.RefittedNodes != 0 || frame.Stats.MeshUploads != 0 {
		return fmt.Errorf("color-only preparation/refit: %v", err)
	}
	if err = bvhNativeParity(r, ref, snap, camera); err != nil {
		return err
	}
	cases++
	// Two identical visible boxes: the earlier original ID must still win.
	if err = scene.SetTransform("900", scene3d.Identity()); err != nil {
		return err
	}
	snap, err = scene.Snapshot()
	if err != nil {
		return err
	}
	frame, err = r.Render(snap, camera, 96, 72, color.RGBA{})
	if err != nil || frame.Stats.Culling.RefittedNodes <= 0 || frame.Stats.Culling.RefittedNodes > 12 || frame.Pick(48, 36) != "500" {
		return fmt.Errorf("refit or equal-depth order changed: %v", err)
	}
	if err = bvhNativeParity(r, ref, snap, camera); err != nil {
		return err
	}
	cases++
	if err = scene.SetVisible("500", false); err != nil {
		return err
	}
	snap, err = scene.Snapshot()
	if err != nil {
		return err
	}
	frame, err = r.Render(snap, camera, 96, 72, color.RGBA{})
	if err != nil || frame.Pick(48, 36) != "900" || frame.Stats.ValidatedInstances != 999 || frame.Stats.Culling.RefittedNodes == 0 {
		return fmt.Errorf("visibility change retained old ID/index: %v", err)
	}
	if err = bvhNativeParity(r, ref, snap, camera); err != nil {
		return err
	}
	cases++
	// An invalid, offscreen transform must still be checked before culling.
	if err = scene.SetTransform("999", scene3d.Translation(scene3d.Vec3{X: 1e100})); err != nil {
		return err
	}
	bad, err := scene.Snapshot()
	if err != nil {
		return err
	}
	if _, err = r.Render(bad, camera, 96, 72, color.RGBA{}); err == nil {
		return fmt.Errorf("cached preparation concealed invalid new snapshot")
	}
	if err = bvhNativeParity(r, ref, original, camera); err != nil {
		return err
	}
	cases++
	if err = r.CloseCurrent(); err != nil {
		return err
	}
	if r.prepared != nil || r.key != nil || r.visible != nil || len(r.meshes) != 0 {
		return fmt.Errorf("close retained preparation/visibility resources")
	}
	fmt.Printf("BVH_NATIVE_PASS exact_rgba_pick cases=%d camera_only_validations=0 camera_only_refits=0\n", cases)
	return nil
}
