//go:build cgo && !silk_pure_go

package glrender

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/scene3d"
)

var nativeErr error
var nativeRan bool

func TestMain(m *testing.M) {
	runtime.LockOSThread()
	if os.Getenv("SILK_TEST_SCENE_GPU") == "1" {
		nativeRan = true
		nativeErr = nativeProbe()
	}
	os.Exit(m.Run())
}
func TestNativeGPUReadbackParityAndLifecycle(t *testing.T) {
	if !nativeRan {
		t.Skip("set SILK_TEST_SCENE_GPU=1 for actual driver/context validation")
	}
	if nativeErr != nil {
		t.Fatal(nativeErr)
	}
}
func nativeProbe() error {
	if e := glfw.Init(); e != nil {
		return e
	}
	defer glfw.Terminate()
	glfw.DefaultWindowHints()
	glfw.WindowHint(glfw.Visible, glfw.False)
	glfw.WindowHint(glfw.ContextVersionMajor, 2)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	w, e := glfw.CreateWindow(32, 32, "Silk 3D driver probe", nil, nil)
	if e != nil {
		return e
	}
	defer w.Destroy()
	w.MakeContextCurrent()
	nativeInstancingRan = true
	nativeInstancingErr = nativeInstancingProbe()
	if nativeInstancingErr != nil {
		return nativeInstancingErr
	}
	nativeFrustumRan = true
	nativeFrustumErr = nativeFrustumProbe()
	if nativeFrustumErr != nil {
		return nativeFrustumErr
	}
	nativeBVHRan = true
	nativeBVHErr = nativeBVHProbe()
	if nativeBVHErr != nil {
		return nativeBVHErr
	}
	r, e := NewCurrent()
	if e != nil {
		return e
	}
	defer r.Close()
	fmt.Printf("GPU_INFO %+v\n", r.Info())
	for _, extension := range strings.Fields(r.api.GetString(0x1F03)) {
		if strings.Contains(extension, "instanc") {
			fmt.Printf("GPU_INSTANCE_EXTENSION %s\n", extension)
		}
	}
	s := scene3d.NewScene()
	mesh := scene3d.Box()
	for _, n := range []scene3d.Node{{ID: "front", Mesh: mesh, Color: color.RGBA{230, 40, 60, 255}}, {ID: "back", Mesh: mesh, Transform: scene3d.Translation(scene3d.Vec3{X: .7, Z: -3}), Color: color.RGBA{20, 210, 140, 255}}} {
		if e = s.Add(n); e != nil {
			return e
		}
	}
	snap, e := s.Snapshot()
	if e != nil {
		return e
	}
	c := scene3d.DefaultCamera()
	c.Eye = scene3d.Vec3{Z: 6}
	var cpu scene3d.Renderer
	for _, size := range [][2]int{{160, 120}, {200, 150}, {320, 240}} {
		f, e := r.Render(snap, c, size[0], size[1], color.RGBA{20, 28, 41, 255})
		if e != nil {
			return e
		}
		ref, e := cpu.Render(snap, c, size[0], size[1], color.RGBA{20, 28, 41, 255})
		if e != nil {
			return e
		}
		mismatch, checked := 0, 0
		// Ignore triangle-edge rules; compare stable interiors and backgrounds.
		for y := 2; y < size[1]-2; y++ {
			for x := 2; x < size[0]-2; x++ {
				id := ref.Pick(x, y)
				stable := true
				for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
					if ref.Pick(x+d[0], y+d[1]) != id {
						stable = false
					}
				}
				if !stable {
					continue
				}
				checked++
				if f.Pick(x, y) != id {
					mismatch++
					continue
				}
				a, b := f.Image.RGBAAt(x, y), ref.Image.RGBAAt(x, y)
				if abs(int(a.R)-int(b.R)) > 2 || abs(int(a.G)-int(b.G)) > 2 || abs(int(a.B)-int(b.B)) > 2 || a.A != b.A {
					mismatch++
				}
			}
		}
		if mismatch != 0 {
			return fmt.Errorf("GPU/CPU parity %v mismatches=%d/%d", size, mismatch, checked)
		}
		if f.Pick(size[0]/2, size[1]/2) != "front" {
			return fmt.Errorf("wrong front ID")
		}
		seq := f.Sequence
		f, e = r.Render(snap, c, size[0], size[1], color.RGBA{20, 28, 41, 255})
		if e != nil || !f.Stats.Cached || f.Sequence != seq {
			return fmt.Errorf("cache miss: %v", e)
		}
		fmt.Printf("GPU_PARITY size=%v checked=%d sequence=%d cached=%v\n", size, checked, seq, f.Stats.Cached)
	}
	// Shared vertex/index buffers survive camera/transform/color changes.
	c.Eye.X += .1
	f, e := r.Render(snap, c, 320, 240, color.RGBA{})
	if e != nil {
		return e
	}
	if f.Stats.MeshUploads != 0 || f.Stats.ResidentMeshes != 1 {
		return fmt.Errorf("shared meshes reuploaded: %+v", f.Stats)
	}
	if e = s.SetVisible("front", false); e != nil {
		return e
	}
	snap, _ = s.Snapshot()
	f, e = r.Render(snap, c, 320, 240, color.RGBA{})
	if e != nil {
		return e
	}
	if f.Pick(160, 120) == "front" {
		return fmt.Errorf("hidden GPU node still pickable")
	}
	// Repeated model replacement must evict old GPU resources.
	for i := 0; i < 20; i++ {
		replacement := scene3d.NewScene()
		_ = replacement.Add(scene3d.Node{ID: "replacement", Mesh: scene3d.Box()})
		ss, _ := replacement.Snapshot()
		f, e = r.Render(ss, c, 64, 64, color.RGBA{})
		if e != nil {
			return e
		}
		if f.Stats.ResidentMeshes != 1 {
			return fmt.Errorf("mesh cache leak: %+v", f.Stats)
		}
	}
	if _, e = r.Render(snap, c, scene3d.MaxDimension+1, 1, color.RGBA{}); e == nil {
		return fmt.Errorf("oversize GPU target accepted")
	}
	// A representative larger scene, including readback (not GPU-only timing).
	bench := scene3d.NewScene()
	cylinder, _ := scene3d.Cylinder(1, 2, 64)
	for i := 0; i < 1000; i++ {
		_ = bench.Add(scene3d.Node{ID: fmt.Sprintf("device-%d", i), Mesh: cylinder, Transform: scene3d.Translation(scene3d.Vec3{X: float64(i%40) - 20, Z: float64(i/40) - 12}).Mul(scene3d.Scaling(scene3d.Vec3{X: .3, Y: .7, Z: .3}))})
	}
	bs, _ := bench.Snapshot()
	camera := scene3d.DefaultCamera()
	camera.Eye = scene3d.Vec3{X: 30, Y: 30, Z: 45}
	for _, backend := range []string{"cpu", "gpu"} {
		start := time.Now()
		for j := 0; j < 6; j++ {
			camera.Eye.X = 30 + float64(j+1)*.001
			if backend == "gpu" {
				f, e = r.Render(bs, camera, 640, 480, color.RGBA{})
			} else {
				_, e = cpu.Render(bs, camera, 640, 480, color.RGBA{})
			}
			if e != nil {
				return e
			}
		}
		fmt.Printf("GPU_COMPARISON backend=%s triangles=256000 frames=6 total=%s\n", backend, time.Since(start))
	}
	fmt.Printf("GPU_STATS %+v\n", f.Stats)
	r.Close()
	r.Close()
	if _, e = r.Render(snap, c, 16, 16, color.RGBA{}); e != ErrClosed {
		return fmt.Errorf("closed renderer accepted work")
	}
	return nativeContextProbe(w)
}
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Native objects can have the same numeric GL names in different contexts.
// Wrong-context cleanup must not delete a sibling's objects, and concurrent
// context initialization/rendering must not replace another renderer's table.
func nativeContextProbe(first *glfw.Window) error {
	second, e := glfw.CreateWindow(32, 32, "Silk isolated context probe", nil, nil)
	if e != nil {
		return e
	}
	defer second.Destroy()
	first.MakeContextCurrent()
	a, e := NewCurrent()
	if e != nil {
		return e
	}
	defer func() { first.MakeContextCurrent(); a.Close() }()
	s := scene3d.NewScene()
	_ = s.Add(scene3d.Node{ID: "one", Mesh: scene3d.Box(), Color: color.RGBA{220, 20, 30, 255}})
	snapshot, _ := s.Snapshot()
	camera := scene3d.DefaultCamera()
	camera.Eye = scene3d.Vec3{Z: 6}
	f, e := a.Render(snapshot, camera, 96, 72, color.RGBA{})
	if e != nil {
		return e
	}
	seq := f.Sequence
	second.MakeContextCurrent()
	b, e := NewCurrent()
	if e != nil {
		return e
	}
	defer func() { second.MakeContextCurrent(); b.Close() }()
	if _, e = a.Render(snapshot, camera, 96, 72, color.RGBA{}); !errors.Is(e, ErrContext) {
		return fmt.Errorf("wrong context accepted cached frame: %v", e)
	}
	if e = a.CloseCurrent(); !errors.Is(e, ErrContext) {
		return fmt.Errorf("wrong context cleanup not rejected: %v", e)
	}
	if f, e = b.Render(snapshot, camera, 96, 72, color.RGBA{}); e != nil || f.Pick(48, 36) != "one" {
		return fmt.Errorf("sibling damaged by rejected close: %v", e)
	}
	first.MakeContextCurrent()
	if f, e = a.Render(snapshot, camera, 96, 72, color.RGBA{}); e != nil || f.Sequence != seq || !f.Stats.Cached {
		return fmt.Errorf("owner/cache lost after context switch: %v", e)
	}

	// Workers must reject even concurrent/post-close calls without reading the
	// owner's mutable fields or freed C table. The race build verifies this path.
	started, stop, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	workerCamera := camera
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		close(started)
		check := func() error {
			if _, e := a.Render(snapshot, workerCamera, 96, 72, color.RGBA{}); !errors.Is(e, ErrContext) {
				return fmt.Errorf("worker render: %v", e)
			}
			if e := a.CloseCurrent(); !errors.Is(e, ErrContext) {
				return fmt.Errorf("worker cleanup: %v", e)
			}
			return nil
		}
		for {
			if e := check(); e != nil {
				done <- e
				return
			}
			select {
			case <-stop:
				for i := 0; i < 100; i++ {
					if e := check(); e != nil {
						done <- e
						return
					}
				}
				done <- nil
				return
			default:
			}
		}
	}()
	<-started
	for i := 0; i < 8; i++ {
		camera.Eye.X += .001
		if _, e = a.Render(snapshot, camera, 96, 72, color.RGBA{}); e != nil {
			break
		}
	}
	closeErr := a.CloseCurrent()
	close(stop)
	if workerErr := <-done; workerErr != nil {
		return workerErr
	}
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	second.MakeContextCurrent()
	if e = b.CloseCurrent(); e != nil {
		return e
	}
	glfw.DetachCurrentContext()
	var ready, finished sync.WaitGroup
	finished.Add(2)
	ready.Add(2)
	start := make(chan struct{})
	results := make(chan error, 2)
	for index, window := range []*glfw.Window{first, second} {
		go func(index int, window *glfw.Window) {
			defer finished.Done()
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			window.MakeContextCurrent()
			defer glfw.DetachCurrentContext()
			r, e := NewCurrent()
			ready.Done()
			<-start
			if e != nil {
				results <- e
				return
			}
			defer r.Close()
			scene := scene3d.NewScene()
			id := fmt.Sprintf("context-%d", index)
			c := color.RGBA{230, 10, 20, 255}
			if index == 1 {
				c = color.RGBA{10, 20, 230, 255}
			}
			_ = scene.Add(scene3d.Node{ID: id, Mesh: scene3d.Box(), Color: c})
			ss, _ := scene.Snapshot()
			camera := scene3d.DefaultCamera()
			camera.Eye = scene3d.Vec3{Z: 6}
			for frame := 0; frame < 64; frame++ {
				camera.Eye.X = float64(frame) * .001
				f, e := r.Render(ss, camera, 96, 72, color.RGBA{})
				if e != nil {
					results <- e
					return
				}
				if f.Pick(48, 36) != id || f.Sequence != uint64(frame+1) || (frame > 0 && f.Stats.MeshUploads != 0) {
					results <- fmt.Errorf("cross-context frame/cache corruption context=%d", index)
					return
				}
				p := f.Image.RGBAAt(48, 36)
				if (index == 0 && p.R <= p.B) || (index == 1 && p.B <= p.R) {
					results <- fmt.Errorf("cross-context color corruption: %v", p)
					return
				}
			}
			results <- nil
		}(index, window)
	}
	ready.Wait()
	close(start)
	// A send occurs before worker defers; wait for context detachment before the
	// main thread can bind/destroy these native windows.
	var result error
	for range 2 {
		if e := <-results; e != nil {
			result = e
		}
	}
	finished.Wait()
	first.MakeContextCurrent()
	if result != nil {
		return result
	}
	fmt.Println("GPU_CONTEXT_PASS wrong_context wrong_thread concurrent_close two_parallel_contexts frames=128")
	return nil
}
