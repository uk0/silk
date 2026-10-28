//go:build cgo && !silk_pure_go && !windows

package gui

import (
	"fmt"
	"os"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/paint"
)

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
	if v.LastError() != nil || v.ActiveRenderBackend() != "cpu" || sceneDevice != nil {
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
