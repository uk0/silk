//go:build cgo && !silk_pure_go && !windows

package gui

import (
	"fmt"
	"image/color"
	"os"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/scene3d"
	"github.com/uk0/silk/scene3d/glrender"
)

type recordedSceneNativeGPU struct {
	sceneGPUBackend
	stats        glrender.Stats
	capabilities glrender.Capabilities
}

func (b *recordedSceneNativeGPU) Capabilities() glrender.Capabilities { return b.capabilities }
func (b *recordedSceneNativeGPU) Render(s *scene3d.Snapshot, c scene3d.Camera, w, h int, bg color.RGBA) (scene3d.FrameView, error) {
	f, e := b.sceneGPUBackend.Render(s, c, w, h, bg)
	if e == nil {
		b.stats = f.(*glrender.Frame).Stats
	}
	return f, e
}

var sceneNativeErr error

func TestSceneNativeGPUCompositionAndTeardown(t *testing.T) {
	if os.Getenv("SILK_TEST_SCENE_GPU") != "1" {
		t.Skip("set SILK_TEST_SCENE_GPU=1 for native widget GPU validation")
	}
	if sceneNativeErr != nil {
		t.Fatal(sceneNativeErr)
	}
}
func runSceneNativeProbe() error {
	if !glfwReady {
		return fmt.Errorf("native GLFW desktop unavailable")
	}
	f := NewForm()
	if e := f.TryAttachWindow(WtForm); e != nil {
		return e
	}
	defer f.Window().destroy()
	v := NewSceneView()
	v.SetParent(f)
	defer v.Close()
	v.SetSize(160, 120)
	v.SetYaw(0)
	v.SetPitch(0)
	pm := paint.NewPixmap(200, 160)
	defer paint.CloseSurface(pm)
	p := pm.NewPainter()
	defer paint.ClosePainter(p)
	previous := glfw.GetCurrentContext()
	v.Draw(p)
	if v.LastError() != nil || v.ActiveRenderBackend() != "opengl" {
		return fmt.Errorf("GPU not used: %v / %v", v.LastError(), v.GPUError())
	}
	initialImage, e := pm.Image()
	if e != nil {
		return e
	}
	initialR, initialG, initialB, initialA := initialImage.At(80, 60).RGBA()
	if glfw.GetCurrentContext() != previous {
		return fmt.Errorf("GPU draw changed host current context")
	}
	before := v.RenderCount()
	if v.PickNode(80, 60) != "model" || v.RenderCount() != before {
		return fmt.Errorf("GPU pick/cache mismatch")
	}
	// A normal widget painted later must remain on top of the GPU result.
	p.SetBrush1(paint.Color{R: 231, G: 12, B: 170, A: 255})
	p.Rectangle(70, 50, 20, 20)
	p.Fill()
	im, e := pm.Image()
	if e != nil {
		return e
	}
	r, g, b, _ := im.At(80, 60).RGBA()
	if r>>8 != 231 || g>>8 != 12 || b>>8 != 170 {
		return fmt.Errorf("GPU escaped ordinary painter order")
	}
	if e := v.SetNodeTransform("model", scene3d.Translation(scene3d.Vec3{X: 1e4})); e != nil {
		return e
	}
	v.Draw(p)
	if v.PickNode(80, 60) != "" || glfw.GetCurrentContext() != previous {
		return fmt.Errorf("offscreen GPU instance remained pickable or changed host context")
	}
	if e := v.SetNodeTransform("model", scene3d.Identity()); e != nil {
		return e
	}
	if v.PickNode(80, 60) != "model" {
		return fmt.Errorf("GPU instance failed to reenter after culling")
	}
	v.Draw(p)
	im, e = pm.Image()
	if e != nil {
		return e
	}
	r, g, b, a := im.At(80, 60).RGBA()
	if r != initialR || g != initialG || b != initialB || a != initialA {
		return fmt.Errorf("GPU reentry pick did not refresh composed pixels")
	}
	// Exercise the actual composed widget path with a shared-mesh instance run,
	// not only a direct SDK renderer with a detached FBO.
	recorded := &recordedSceneNativeGPU{sceneGPUBackend: v.gpu, capabilities: v.GPUCapabilities()}
	v.gpu = recorded
	assembly := scene3d.NewScene()
	mesh := scene3d.Box()
	for i := 0; i < 1000; i++ {
		if e := assembly.Add(scene3d.Node{ID: fmt.Sprint(i), Mesh: mesh, Transform: scene3d.Translation(scene3d.Vec3{X: (float64(i%40) - 20) * .1, Y: (float64(i/40) - 12) * .1}).Mul(scene3d.Scaling(scene3d.Vec3{X: .08, Y: .08, Z: .08}))}); e != nil {
			return e
		}
	}
	if e := v.SetScene(assembly); e != nil {
		return e
	}
	if e := v.FitScene(); e != nil {
		return e
	}
	v.Draw(p)
	if v.LastError() != nil || glfw.GetCurrentContext() != previous {
		return fmt.Errorf("composed instancing render failed or changed host context: %v", v.LastError())
	}
	if v.GPUCapabilities() != recorded.capabilities {
		return fmt.Errorf("widget reports wrong GPU capability")
	}
	snapshot, e := assembly.Snapshot()
	if e != nil {
		return e
	}
	// Geometry at the centre is deliberately a mesh instance; don't accept a
	// visually empty draw or a successful capability query as integration proof.
	if snapshot.InstanceCount() != 1000 || v.PickNode(80, 60) == "" {
		return fmt.Errorf("composed instance run is not pickable")
	}
	if recorded.capabilities.InstancedDrawing && (recorded.stats.InstancedInstances != 1000 || recorded.stats.DrawCalls != 1) {
		return fmt.Errorf("widget bypassed native instancing: %+v", recorded.stats)
	}
	fmt.Printf("SCENE_NATIVE_INSTANCING capabilities=%+v instances=%d stats=%+v\n", v.GPUCapabilities(), snapshot.InstanceCount(), recorded.stats)
	second := NewSceneView()
	second.SetParent(f)
	second.SetSize(160, 120)
	defer second.Close()
	second.Draw(p)
	if second.ActiveRenderBackend() != "opengl" || sceneDevice == nil || len(sceneDevice.clients) != 2 {
		return fmt.Errorf("views do not share one resource context")
	}
	second.Close()
	if sceneDevice == nil || len(sceneDevice.clients) != 1 {
		return fmt.Errorf("closing one view destroyed sibling resources")
	}
	v.SetRenderBackend("cpu")
	v.Draw(p)
	if v.LastError() != nil || v.ActiveRenderBackend() != "cpu" || sceneDevice != nil || v.GPUCapabilities().InstancedDrawing {
		return fmt.Errorf("CPU switch leaked GPU device")
	}
	v.SetRenderBackend("opengl")
	v.Draw(p)
	if v.LastError() != nil || sceneDevice == nil {
		return fmt.Errorf("explicit GPU retry failed")
	}
	closeSceneGPUDevice()
	if sceneDevice != nil {
		return fmt.Errorf("shutdown retained GPU device")
	}
	v.Close()
	second.Close() // safe after backend shutdown and before native destruction
	if glfw.GetCurrentContext() != previous {
		return fmt.Errorf("GPU teardown changed host context")
	}
	fmt.Println("SCENE_NATIVE_GPU_PASS composition context_restore shared_device pick_cache switch shutdown")
	return nil
}
