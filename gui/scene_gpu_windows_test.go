//go:build windows && cgo && !silk_pure_go

package gui

import (
	"errors"
	"github.com/uk0/silk/scene3d/glrender"
	"os"
	"testing"

	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/win32"
)

// Unlike the strict standalone GPU test, this probe validates auto fallback on
// desktop drivers that expose only OpenGL 1.1 (including some VM/RDP sessions).
// Its log always distinguishes CPU fallback from successful GPU rendering.
func TestSceneNativeWindowsWGLCompositionAndFallback(t *testing.T) {
	if os.Getenv("SILK_TEST_SCENE_WGL") != "1" {
		t.Skip("set SILK_TEST_SCENE_WGL=1 on an interactive Windows desktop")
	}
	isolateWindowsPopupState(t)
	if sceneDevice != nil {
		t.Fatal("native scene device already in use")
	}
	previousDC, previousRC := win32.WglGetCurrentDC(), win32.WglGetCurrentContext()
	host, err := newWGLSceneDevice()
	if err != nil {
		t.Fatal("cannot create even a legacy host WGL context:", err)
	}
	defer func() {
		win32.WglMakeCurrent(previousDC, previousRC)
		host.destroy()
	}()
	if !win32.WglMakeCurrent(host.dc, host.rc) {
		t.Fatal("cannot bind host context")
	}
	checkHost := func() {
		t.Helper()
		if win32.WglGetCurrentDC() != host.dc || win32.WglGetCurrentContext() != host.rc {
			t.Fatal("scene operation changed host WGL binding")
		}
	}
	f := NewForm()
	f.SetVisible(false)
	if err := f.TryAttachWindow(WtForm); err != nil {
		t.Fatal(err)
	}
	defer f.Window().destroy()
	if err := f.Window().SetSceneGPUComposition(true); !errors.Is(err, glrender.ErrUnavailable) {
		t.Fatal("GDI claimed direct composition", err)
	}
	if enabled, reason := f.Window().SceneGPUCompositionStatus(); enabled || reason != "" {
		t.Fatal("unsupported composition changed state")
	}
	if err := f.Window().SetSceneGPUComposition(false); err != nil {
		t.Fatal(err)
	}
	defer closeSceneGPUDevice()
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
	windowCount := len(winMap)
	v.Draw(p)
	checkHost()
	if v.LastError() != nil || !v.gpuAttempted || len(winMap) != windowCount {
		t.Fatal("auto renderer or invisible device ownership failed", v.LastError())
	}
	initialTransfers, err := v.GPUTransfers()
	if err != nil {
		t.Fatal(err)
	}
	if v.ActiveRenderBackend() == "opengl" && (initialTransfers.ColorReads != 1 || initialTransfers.ColorBytes != 160*120*4 || initialTransfers.IDBytes != 0) {
		t.Fatal("drawing captured IDs", initialTransfers)
	}
	before := v.RenderCount()
	if v.PickNode(80, 60) != "model" || v.RenderCount() != before {
		t.Fatal("native pick rerendered or returned wrong node")
	}
	afterPick, err := v.GPUTransfers()
	if err != nil {
		t.Fatal(err)
	}
	if v.ActiveRenderBackend() == "opengl" && (afterPick.ColorBytes != initialTransfers.ColorBytes || afterPick.IDReads != 1 || afterPick.IDBytes != 4) {
		t.Fatal("picking did not use one raster pixel", afterPick)
	}
	checkHost()
	p.SetBrush1(paint.Color{R: 231, G: 12, B: 170, A: 255})
	p.Rectangle(70, 50, 20, 20)
	p.Fill()
	im, err := pm.Image()
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := im.At(80, 60).RGBA()
	if r>>8 != 231 || g>>8 != 12 || b>>8 != 170 {
		t.Fatal("scene escaped ordinary painter order")
	}
	if v.ActiveRenderBackend() == "cpu" {
		if v.GPUError() == nil || sceneDevice != nil {
			t.Fatal("fallback lacks diagnostic or leaked resource device")
		}
		t.Logf("SCENE_WGL_CPU_FALLBACK reason=%v", v.GPUError())
		v.Draw(p)
		if v.RenderCount() != before || sceneDevice != nil {
			t.Fatal("fallback recreated GPU or rerendered unchanged scene")
		}
		v.SetRenderBackend("opengl")
		v.Draw(p)
		if v.LastError() == nil || sceneDevice != nil {
			t.Fatal("forced unavailable GPU did not fail cleanly")
		}
	} else {
		t.Logf("SCENE_WGL_GPU driver=%+v", v.GPUInfo())
		second := NewSceneView()
		second.SetParent(f)
		second.SetSize(160, 120)
		defer second.Close()
		second.Draw(p)
		if second.ActiveRenderBackend() != "opengl" || sceneDevice == nil || len(sceneDevice.clients) != 2 {
			t.Fatal("views do not share one resource context")
		}
		second.Close()
		if sceneDevice == nil || len(sceneDevice.clients) != 1 {
			t.Fatal("closing sibling destroyed shared resources")
		}
	}
	checkHost()
	v.SetRenderBackend("cpu")
	v.Draw(p)
	if v.LastError() != nil || v.ActiveRenderBackend() != "cpu" || sceneDevice != nil {
		t.Fatal("CPU switch leaked GPU device or retained render error")
	}
	v.SetRenderBackend("auto")
	v.Draw(p)
	if v.LastError() != nil {
		t.Fatal("explicit retry failed to render", v.LastError())
	}
	closeSceneGPUDevice()
	v.Close()
	v.Close()
	if sceneDevice != nil {
		t.Fatal("shutdown retained GPU device")
	}
	checkHost()
	t.Log("SCENE_WGL_PASS composition context_restore pick_cache switch shutdown")
}
