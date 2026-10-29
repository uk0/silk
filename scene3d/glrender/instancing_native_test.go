//go:build cgo && !silk_pure_go

package glrender

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/scene3d"
)

var nativeInstancingErr error
var nativeInstancingRan, nativeInstancingSupported bool

func TestNativeInstancingExactParityOrderIDsAndLifetime(t *testing.T) {
	if !nativeRan {
		if os.Getenv("SILK_TEST_REQUIRE_INSTANCING") == "1" {
			t.Fatal("strict instancing requires SILK_TEST_SCENE_GPU=1")
		}
		t.Skip("set SILK_TEST_SCENE_GPU=1 for actual driver validation")
	}
	if !nativeInstancingRan {
		t.Fatalf("context failed before instancing probe: %v", nativeErr)
	}
	if nativeInstancingErr != nil {
		t.Fatal(nativeInstancingErr)
	}
	if !nativeInstancingSupported {
		if os.Getenv("SILK_TEST_REQUIRE_INSTANCING") == "1" {
			t.Fatal("current driver lacks required instancing")
		}
		t.Skip("current driver lacks instancing; ordinary GPU fallback verified, not hardware instancing")
	}
}

func instancingParity(fast, ref *Renderer, s *scene3d.Snapshot, c scene3d.Camera) (*Frame, error) {
	a, err := fast.Render(s, c, 128, 96, color.RGBA{R: 9, G: 18, B: 27, A: 255})
	if err != nil {
		return nil, err
	}
	b, err := ref.Render(s, c, 128, 96, color.RGBA{R: 9, G: 18, B: 27, A: 255})
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(a.Image.Pix, b.Image.Pix) || !bytes.Equal(a.pickPixels, b.pickPixels) {
		rgba, picks, maxDelta := 0, 0, 0
		for i, v := range a.Image.Pix {
			d := int(v) - int(b.Image.Pix[i])
			if d < 0 {
				d = -d
			}
			if d != 0 {
				rgba++
			}
			if d > maxDelta {
				maxDelta = d
			}
		}
		for i, v := range a.pickPixels {
			if v != b.pickPixels[i] {
				picks++
			}
		}
		return nil, fmt.Errorf("instancing parity rgba_bytes=%d max_delta=%d pick_bytes=%d stats=%+v", rgba, maxDelta, picks, a.Stats)
	}
	if a.Stats.Instances != b.Stats.Instances || a.Stats.Triangles != b.Stats.Triangles || a.Stats.CulledInstances != b.Stats.CulledInstances || a.Stats.CulledTriangles != b.Stats.CulledTriangles {
		return nil, fmt.Errorf("instancing changed visible work: %+v vs %+v", a.Stats, b.Stats)
	}
	return a, nil
}

func nativeInstancingProbe() (result error) {
	fast, err := NewCurrent()
	if err != nil {
		return err
	}
	fmt.Printf("GPU_INSTANCING_INFO %+v shader4=%v\n", fast.Info(), strings.Contains(" "+fast.api.GetString(0x1F03)+" ", " GL_EXT_gpu_shader4 "))
	defer func() {
		if e := fast.CloseCurrent(); result == nil {
			result = e
		}
	}()
	ref, err := NewCurrentWithOptions(Options{Instancing: InstancingDisabled})
	if err != nil {
		return err
	}
	defer ref.Close()
	if ref.Capabilities().InstancedDrawing {
		return fmt.Errorf("disabled instancing advertised as enabled")
	}
	required, requiredErr := NewCurrentWithOptions(Options{Instancing: InstancingRequired})
	if !fast.Capabilities().InstancedDrawing {
		if required != nil || !errors.Is(requiredErr, ErrUnavailable) {
			if required != nil {
				required.Close()
			}
			return fmt.Errorf("required instancing accepted unsupported context: %v", requiredErr)
		}
		return nil
	}
	nativeInstancingSupported = true
	if requiredErr != nil {
		return requiredErr
	}
	required.Close()
	var core *Renderer
	var major, minor int
	if _, err = fmt.Sscanf(fast.Info().Version, "%d.%d", &major, &minor); err != nil {
		return err
	}
	if major >= 3 {
		core, err = NewCurrentWithOptions(Options{Instancing: InstancingRequired})
		if err != nil {
			return err
		}
		defer core.Close()
		vertex, fragment := instancingShaderSources(false)
		program, e := compileProgram(core.api, vertex, fragment, []string{"position", "world0", "world1", "world2", "world3", "instanceMaterial", "instanceID"})
		if e != nil {
			return e
		}
		core.api.DeleteProgram(core.instanceProgram)
		core.instanceProgram = program
		core.instanceVP = core.api.GetUniformLocation(program, "viewProjection")
		if core.instanceVP < 0 {
			return fmt.Errorf("core instancing uniform missing")
		}
	}
	parity := func(s *scene3d.Snapshot, c scene3d.Camera) (*Frame, error) {
		f, e := instancingParity(fast, ref, s, c)
		if e != nil {
			return nil, e
		}
		if core != nil {
			if _, e = instancingParity(core, ref, s, c); e != nil {
				return nil, fmt.Errorf("GLSL130 variant: %w", e)
			}
		}
		return f, nil
	}
	camera := scene3d.DefaultCamera()
	camera.Eye = scene3d.Vec3{Z: 10}
	mesh := scene3d.Box()
	cases := 0
	for _, count := range []int{0, 1, 2, 1023, 1024, 1025, 2051} {
		scene := scene3d.NewScene()
		for i := 0; i < count; i++ {
			pose := scene3d.Translation(scene3d.Vec3{X: (float64(i%64) - 31.5) * .1, Y: (float64(i/64) - 16) * .1}).Mul(scene3d.Scaling(scene3d.Vec3{X: .07, Y: .08, Z: .09}))
			if err = scene.Add(scene3d.Node{ID: fmt.Sprint(i), Mesh: mesh, Transform: pose, Color: color.RGBA{R: byte(i * 37), G: byte(i * 71), B: byte(i * 109), A: 255}}); err != nil {
				return err
			}
		}
		snapshot, e := scene.Snapshot()
		if e != nil {
			return e
		}
		frame, e := parity(snapshot, camera)
		if e != nil {
			return fmt.Errorf("count %d: %w", count, e)
		}
		wantCalls := (count + maxInstanceBatch - 1) / maxInstanceBatch
		instanced := count
		if count%maxInstanceBatch == 1 {
			instanced--
		}
		if frame.Stats.DrawCalls != wantCalls || frame.Stats.Instances != count || frame.Stats.InstancedInstances != instanced || frame.Stats.InstanceUploadBytes != int64(instanced*instanceStride) || frame.Stats.InstanceBufferBytes > maxInstanceBatch*instanceStride {
			return fmt.Errorf("count %d batching/budget mismatch: %+v", count, frame.Stats)
		}
		if ref.frame.Stats.DrawCalls != count || ref.frame.Stats.InstancedDrawCalls != 0 || ref.frame.Stats.InstanceUploadBytes != 0 {
			return fmt.Errorf("reference is not ordinary draw path: %+v", ref.frame.Stats)
		}
		sequence := frame.Sequence
		frame, e = fast.Render(snapshot, camera, 128, 96, color.RGBA{R: 9, G: 18, B: 27, A: 255})
		if e != nil {
			return e
		}
		if !frame.Stats.Cached || frame.Sequence != sequence {
			return fmt.Errorf("batch cache rerendered")
		}
		cases++
	}
	// Grouping by mesh would move later A's before B and change equal-depth winner.
	ordered := scene3d.NewScene()
	other := scene3d.Box()
	for i, n := range []scene3d.Node{
		{ID: "A-back-1", Mesh: mesh, Transform: scene3d.Translation(scene3d.Vec3{Z: -2})},
		{ID: "A-back-2", Mesh: mesh, Transform: scene3d.Translation(scene3d.Vec3{Z: -3})},
		{ID: "B-winner", Mesh: other, Color: color.RGBA{R: 239, A: 255}},
		{ID: "A-tie-1", Mesh: mesh, Color: color.RGBA{G: 239, A: 255}},
		{ID: "hole", Mesh: other, Transform: scene3d.Translation(scene3d.Vec3{X: 1e4})},
		{ID: "A-tie-2", Mesh: mesh, Color: color.RGBA{B: 239, A: 255}},
	} {
		if err = ordered.Add(n); err != nil {
			return fmt.Errorf("ordered %d: %w", i, err)
		}
	}
	orderedSnapshot, _ := ordered.Snapshot()
	frame, err := parity(orderedSnapshot, camera)
	if err != nil {
		return err
	}
	if frame.Pick(64, 48) != "B-winner" || frame.Stats.DrawCalls != 3 || frame.Stats.InstancedInstances != 4 || frame.Stats.CulledInstances != 1 {
		return fmt.Errorf("order or IDs changed: %s %+v", frame.Pick(64, 48), frame.Stats)
	}
	if err = ordered.SetTransform("hole", scene3d.Translation(scene3d.Vec3{Z: 1})); err != nil {
		return err
	}
	reentry, _ := ordered.Snapshot()
	frame, err = parity(reentry, camera)
	if err != nil {
		return err
	}
	if frame.Pick(64, 48) != "hole" {
		return fmt.Errorf("culled different-mesh node failed to reenter")
	}
	cases += 2
	// Invalid input must preserve the previous frame even when other nodes cull.
	previous := append([]byte(nil), frame.Image.Pix...)
	previousIDs := append([]byte(nil), frame.pickPixels...)
	sequence := frame.Sequence
	invalid := scene3d.NewScene()
	pose := scene3d.Identity()
	pose[0] = math.MaxFloat64
	pose[12] = 1e8
	if err = invalid.Add(scene3d.Node{ID: "invalid", Mesh: mesh, Transform: pose}); err != nil {
		return err
	}
	invalidSnapshot, _ := invalid.Snapshot()
	if _, err = fast.Render(invalidSnapshot, camera, 128, 96, color.RGBA{}); !errors.Is(err, scene3d.ErrGeometry) {
		return fmt.Errorf("invalid batched transform accepted: %v", err)
	}
	if frame.Sequence != sequence || !bytes.Equal(previous, frame.Image.Pix) || !bytes.Equal(previousIDs, frame.pickPixels) {
		return fmt.Errorf("invalid input changed previous frame")
	}
	random := rand.New(rand.NewSource(53189))
	for run := 0; run < 80; run++ {
		scene := scene3d.NewScene()
		for i := 0; i < 48; i++ {
			m := mesh
			if i%13 == 0 {
				m = other
			}
			pose := scene3d.Translation(scene3d.Vec3{X: random.Float64()*20 - 10, Y: random.Float64()*12 - 6, Z: random.Float64()*14 - 7}).Mul(scene3d.RotationXYZ(scene3d.Vec3{X: random.Float64() * 6, Y: random.Float64() * 6})).Mul(scene3d.Scaling(scene3d.Vec3{X: random.Float64()*3 - 1.5, Y: random.Float64() * 2, Z: random.Float64() * 2}))
			pose[4] += .2
			if err = scene.Add(scene3d.Node{ID: fmt.Sprint(i), Mesh: m, Transform: pose, Color: color.RGBA{R: byte(random.Intn(256)), G: byte(random.Intn(256)), B: byte(random.Intn(256)), A: 255}}); err != nil {
				return err
			}
		}
		snap, e := scene.Snapshot()
		if e != nil {
			return e
		}
		if _, e = parity(snap, camera); e != nil {
			return fmt.Errorf("random %d: %w", run, e)
		}
		cases++
	}
	cylinder, err := scene3d.Cylinder(.5, 1, 64)
	if err != nil {
		return err
	}
	many := scene3d.NewScene()
	for i := 0; i < 1000; i++ {
		if err = many.Add(scene3d.Node{ID: fmt.Sprint(i), Mesh: cylinder, Transform: scene3d.Translation(scene3d.Vec3{X: (float64(i%40) - 20) * .15, Y: (float64(i/40) - 12) * .15}).Mul(scene3d.Scaling(scene3d.Vec3{X: .1, Y: .1, Z: .1}))}); err != nil {
			return err
		}
	}
	manySnapshot, _ := many.Snapshot()
	if _, err = parity(manySnapshot, camera); err != nil {
		return err
	}
	for _, sample := range []struct {
		name  string
		r     *Renderer
		draws int
	}{{"ordinary", ref, 1000}, {"instanced", fast, 1}} {
		c := camera
		c.Eye.X = -.01
		if _, err = sample.r.Render(manySnapshot, c, 160, 120, color.RGBA{}); err != nil {
			return err
		}
		start := time.Now()
		for i := 0; i < 20; i++ {
			c.Eye.X = float64(i%2) * .001
			frame, err = sample.r.Render(manySnapshot, c, 160, 120, color.RGBA{})
			if err != nil {
				return err
			}
			if frame.Stats.Cached || frame.Stats.DrawCalls != sample.draws {
				return fmt.Errorf("benchmark unexpected work: %+v", frame.Stats)
			}
		}
		fmt.Printf("GPU_INSTANCING_BENCH mode=%s frames=20 total=%s stats=%+v\n", sample.name, time.Since(start), frame.Stats)
	}
	if cap(fast.instanceData) > maxInstanceBatch*instanceFloats {
		return fmt.Errorf("CPU staging unbounded: %d", cap(fast.instanceData))
	}
	if err = fast.CloseCurrent(); err != nil {
		return err
	}
	if fast.instanceBuffer != 0 || fast.instanceProgram != 0 || fast.instanceBufferBytes != 0 || fast.instanceData != nil || len(fast.meshes) != 0 {
		return fmt.Errorf("instance resources survived close")
	}
	if _, err = fast.Render(manySnapshot, camera, 64, 64, color.RGBA{}); !errors.Is(err, ErrClosed) {
		return fmt.Errorf("closed batch renderer accepted render: %v", err)
	}
	fmt.Printf("GPU_INSTANCING_PASS exact_rgba_pick cases=%d core_glsl130=%v ordered_runs original_ids cull_reentry chunks bounded_staging close\n", cases+1, core != nil)
	return nil
}
