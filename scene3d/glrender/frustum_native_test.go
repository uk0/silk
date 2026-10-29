//go:build cgo && !silk_pure_go

package glrender

import (
	"bytes"
	"fmt"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/uk0/silk/scene3d"
)

var nativeFrustumRan bool
var nativeFrustumErr error

func TestNativeFrustumParityUploadsAndStableIDs(t *testing.T) {
	if !nativeRan {
		t.Skip("set SILK_TEST_SCENE_GPU=1 for actual context validation")
	}
	if !nativeFrustumRan {
		t.Fatalf("context setup failed before frustum probe: %v", nativeErr)
	}
	if nativeFrustumErr != nil {
		t.Fatal(nativeFrustumErr)
	}
}

// Unreferenced vertices enlarge bounds only: every indexed triangle, material,
// transform and draw order remains identical. The reference must submit every
// instance, providing an actual driver comparison with culling bypassed without
// a production debug switch or access to scene3d's private mesh fields.
func uncullableNativeSnapshot(s *scene3d.Snapshot) (*scene3d.Snapshot, error) {
	target := scene3d.NewScene()
	meshes := map[*scene3d.Mesh]*scene3d.Mesh{}
	for i := 0; i < s.InstanceCount(); i++ {
		in, _ := s.Instance(i)
		m := meshes[in.Mesh]
		if m == nil {
			vertices := make([]scene3d.Vec3, 0, in.Mesh.VertexCount()+8)
			for j := 0; j < in.Mesh.VertexCount(); j++ {
				v, _ := in.Mesh.Vertex(j)
				vertices = append(vertices, v)
			}
			for j := 0; j < 8; j++ {
				v := scene3d.Vec3{X: -1e7, Y: -1e7, Z: -1e7}
				if j&1 != 0 {
					v.X = 1e7
				}
				if j&2 != 0 {
					v.Y = 1e7
				}
				if j&4 != 0 {
					v.Z = 1e7
				}
				vertices = append(vertices, v)
			}
			var err error
			m, err = scene3d.NewMesh(vertices, in.Mesh.CopyIndices(nil))
			if err != nil {
				return nil, err
			}
			meshes[in.Mesh] = m
		}
		if err := target.Add(scene3d.Node{ID: in.ID, Mesh: m, Transform: in.Transform, Color: color.RGBA{R: in.Color[0], G: in.Color[1], B: in.Color[2], A: 255}}); err != nil {
			return nil, err
		}
	}
	return target.Snapshot()
}

func compareNativeFrustum(r *Renderer, s *scene3d.Snapshot, c scene3d.Camera) error {
	const width, height = 96, 72
	frame, err := r.Render(s, c, width, height, color.RGBA{R: 9, G: 18, B: 27, A: 255})
	if err != nil {
		return err
	}
	pixels := append([]byte(nil), frame.Image.Pix...)
	ids := make([]string, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			ids[y*width+x] = frame.Pick(x, y)
		}
	}
	ref, err := uncullableNativeSnapshot(s)
	if err != nil {
		return err
	}
	reference, err := r.Render(ref, c, width, height, color.RGBA{R: 9, G: 18, B: 27, A: 255})
	if err != nil {
		return err
	}
	if reference.Stats.CulledInstances != 0 || reference.Stats.DrawCalls != s.InstanceCount() {
		return fmt.Errorf("reference did not submit every instance: %+v", reference.Stats)
	}
	if !bytes.Equal(pixels, reference.Image.Pix) {
		return fmt.Errorf("GPU culling changed exact color bytes")
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if ids[y*width+x] != reference.Pick(x, y) {
				return fmt.Errorf("GPU culling changed pick ID at %d,%d", x, y)
			}
		}
	}
	return nil
}

func nativeFrustumProbe() (result error) {
	r, err := NewCurrentWithOptions(Options{Instancing: InstancingDisabled})
	if err != nil {
		return err
	}
	defer func() {
		if err := r.CloseCurrent(); result == nil {
			result = err
		}
	}()
	scene := scene3d.NewScene()
	box := scene3d.Box()
	cylinder, err := scene3d.Cylinder(.5, 1, 64)
	if err != nil {
		return err
	}
	for _, n := range []scene3d.Node{
		{ID: "offscreen-first", Mesh: box, Transform: scene3d.Translation(scene3d.Vec3{X: -1e4})},
		{ID: "visible-second", Mesh: box, Color: color.RGBA{R: 180, G: 30, B: 210, A: 255}},
		{ID: "behind", Mesh: box, Transform: scene3d.Translation(scene3d.Vec3{Z: 100})},
		{ID: "cold-mesh", Mesh: cylinder, Transform: scene3d.Translation(scene3d.Vec3{Y: 1e4})},
	} {
		if err = scene.Add(n); err != nil {
			return err
		}
	}
	snap, err := scene.Snapshot()
	if err != nil {
		return err
	}
	camera := scene3d.DefaultCamera()
	camera.Eye = scene3d.Vec3{Z: 6}
	frame, err := r.Render(snap, camera, 96, 72, color.RGBA{})
	if err != nil {
		return err
	}
	if frame.Stats.DrawCalls != 1 || frame.Stats.CulledInstances != 3 || frame.Stats.MeshUploads != 1 || frame.Stats.ResidentMeshes != 1 || frame.Pick(48, 36) != "visible-second" {
		return fmt.Errorf("cold culling/upload/stable ID: %+v pick=%s", frame.Stats, frame.Pick(48, 36))
	}
	seq := frame.Sequence
	frame, err = r.Render(snap, camera, 96, 72, color.RGBA{})
	if err != nil {
		return err
	}
	if !frame.Stats.Cached || frame.Sequence != seq || frame.Stats.CulledInstances != 3 {
		return fmt.Errorf("culling cache changed")
	}
	if err = compareNativeFrustum(r, snap, camera); err != nil {
		return err
	}
	// A formerly skipped unique mesh must be uploaded when it enters the view.
	if err = scene.SetVisible("visible-second", false); err != nil {
		return err
	}
	if err = scene.SetTransform("cold-mesh", scene3d.Identity()); err != nil {
		return err
	}
	snap, err = scene.Snapshot()
	if err != nil {
		return err
	}
	frame, err = r.Render(snap, camera, 96, 72, color.RGBA{})
	if err != nil {
		return err
	}
	if frame.Stats.DrawCalls != 1 || frame.Stats.CulledInstances != 2 || frame.Stats.MeshUploads != 1 || frame.Pick(48, 36) != "cold-mesh" {
		return fmt.Errorf("entering mesh not restored: %+v", frame.Stats)
	}
	if err = compareNativeFrustum(r, snap, camera); err != nil {
		return err
	}
	// Keep the exact native clipping/edge behavior for retained or borderline
	// boxes, including tiny offsets rounded by actual float32 uniforms.
	cases := []scene3d.Mat4{
		scene3d.Identity(), scene3d.Scaling(scene3d.Vec3{X: -2, Y: 1, Z: 1}),
		scene3d.Translation(scene3d.Vec3{Z: 5.95}), scene3d.Translation(scene3d.Vec3{Z: -1100}),
		scene3d.Scaling(scene3d.Vec3{X: 20, Y: 20, Z: 20}),
	}
	shear := scene3d.Identity()
	shear[4] = 3
	cases = append(cases, shear)
	lo, _ := box.Bounds()
	rightEdge := (camera.Eye.Z-lo.Z)*math.Tan(camera.FOVY/2)*(96.0/72) - lo.X
	for i := -6; i <= 6; i++ {
		cases = append(cases, scene3d.Translation(scene3d.Vec3{X: rightEdge + float64(i)*1e-7}))
	}
	for i, pose := range cases {
		candidate := scene3d.NewScene()
		if err = candidate.Add(scene3d.Node{ID: "boundary", Mesh: box, Transform: pose, Color: color.RGBA{R: 20, G: 210, B: 80, A: 255}}); err != nil {
			return err
		}
		ss, e := candidate.Snapshot()
		if e != nil {
			return e
		}
		if i >= 6 {
			f, e := r.Render(ss, camera, 96, 72, color.RGBA{})
			if e != nil || f.Stats.CulledInstances != 0 {
				return fmt.Errorf("float32-boundary %d was culled: %v", i, e)
			}
		}
		if err = compareNativeFrustum(r, ss, camera); err != nil {
			return fmt.Errorf("boundary %d: %w", i, err)
		}
	}
	// Offscreen invalid float32 transforms still fail before changing the frame.
	last := scene3d.NewScene()
	_ = last.Add(scene3d.Node{ID: "good", Mesh: box})
	ss, _ := last.Snapshot()
	frame, err = r.Render(ss, camera, 96, 72, color.RGBA{})
	if err != nil {
		return err
	}
	seq = frame.Sequence
	invalid := scene3d.NewScene()
	_ = invalid.Add(scene3d.Node{ID: "invalid", Mesh: box, Transform: scene3d.Translation(scene3d.Vec3{X: 1e100})})
	bad, _ := invalid.Snapshot()
	if _, err = r.Render(bad, camera, 96, 72, color.RGBA{}); err == nil {
		return fmt.Errorf("culling concealed invalid float32 transform")
	}
	frame, err = r.Render(ss, camera, 96, 72, color.RGBA{})
	if err != nil || !frame.Stats.Cached || frame.Sequence != seq {
		return fmt.Errorf("invalid offscreen input damaged cached frame: %v", err)
	}
	// Removing all nodes must retire offscreen as well as visible resident meshes.
	empty, _ := scene3d.NewScene().Snapshot()
	frame, err = r.Render(empty, camera, 96, 72, color.RGBA{})
	if err != nil {
		return err
	}
	if frame.Stats.ResidentMeshes != 0 || frame.Stats.DrawCalls != 0 || frame.Pick(48, 36) != "" {
		return fmt.Errorf("offscreen resource/pick leak: %+v", frame.Stats)
	}
	// Include readback in both timings; do not label this hardware instancing.
	many := scene3d.NewScene()
	for i := 0; i < 1000; i++ {
		position := scene3d.Vec3{X: 1000 + float64(i%40)*3, Y: float64(i/40) * 3}
		if i == 500 {
			position = scene3d.Vec3{}
		}
		if err = many.Add(scene3d.Node{ID: fmt.Sprint(i), Mesh: cylinder, Transform: scene3d.Translation(position)}); err != nil {
			return err
		}
	}
	manySnap, err := many.Snapshot()
	if err != nil {
		return err
	}
	reference, err := uncullableNativeSnapshot(manySnap)
	if err != nil {
		return err
	}
	for _, sample := range []struct {
		name     string
		snapshot *scene3d.Snapshot
		draws    int
	}{{"full-submit", reference, 1000}, {"frustum", manySnap, 1}} {
		camera.Eye.X = -.1
		if _, err = r.Render(sample.snapshot, camera, 160, 120, color.RGBA{}); err != nil {
			return err
		}
		start := time.Now()
		for i := 0; i < 20; i++ {
			camera.Eye.X = float64(i%2) * .001
			frame, err = r.Render(sample.snapshot, camera, 160, 120, color.RGBA{})
			if err != nil {
				return err
			}
			if frame.Stats.DrawCalls != sample.draws || frame.Stats.Cached {
				return fmt.Errorf("unexpected measured work: %+v", frame.Stats)
			}
		}
		fmt.Printf("GPU_FRUSTUM_BENCH mode=%s instances=1000 frames=20 total=%s stats=%+v\n", sample.name, time.Since(start), frame.Stats)
	}
	fmt.Printf("GPU_FRUSTUM_PASS exact_rgba_pick scenarios=%d cold_upload cache entering_mesh empty_scene error_preservation\n", len(cases)+2)
	return nil
}
