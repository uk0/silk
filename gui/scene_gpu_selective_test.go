package gui

import (
	"errors"
	"image/color"
	"math"
	"testing"

	"github.com/uk0/silk/scene3d"
	"github.com/uk0/silk/scene3d/glrender"
)

type selectiveProbeBackend struct {
	cached               bool
	err                  error
	picks, closes, draws int
	w, h, x, y           int
}

func (b *selectiveProbeBackend) Render(*scene3d.Snapshot, scene3d.Camera, int, int, color.RGBA) (sceneViewFrame, error) {
	b.draws++
	return nil, errors.New("unexpected color capture during pick")
}
func (b *selectiveProbeBackend) Pick(_ *scene3d.Snapshot, _ scene3d.Camera, w, h int, _ color.RGBA, x, y int) (string, bool, error) {
	b.picks++
	b.w, b.h, b.x, b.y = w, h, x, y
	return "model", b.cached, b.err
}
func (*selectiveProbeBackend) Transfers() (glrender.TransferStats, error) {
	return glrender.TransferStats{IDReads: 1, IDBytes: 4}, nil
}
func (*selectiveProbeBackend) Info() glrender.Info { return glrender.Info{} }
func (b *selectiveProbeBackend) Close()            { b.closes++ }

func TestSceneSelectivePickHiDPIAndCounts(t *testing.T) {
	v := NewSceneView()
	defer v.Close()
	v.SetSize(100, 80)
	b := &selectiveProbeBackend{}
	v.gpu = b
	v.gpuAttempted = true
	v.renderScaleX, v.renderScaleY = 1.5, 2
	for _, point := range [][2]float64{{-1, 0}, {100, 0}, {0, 80}, {math.NaN(), 0}, {0, math.Inf(1)}} {
		if id := v.PickNode(point[0], point[1]); id != "" {
			t.Fatal("invalid point picked", point, id)
		}
	}
	if b.picks != 0 || b.draws != 0 {
		t.Fatal("invalid point touched backend")
	}
	if v.PickNode(99.9, 79.9) != "model" || v.RenderCount() != 1 || b.w != 150 || b.h != 160 || b.x != 149 || b.y != 159 || b.draws != 0 {
		t.Fatal("pick captured color or wrong HiDPI coordinates", b, v.RenderCount())
	}
	b.cached = true
	if v.PickNode(50, 40) != "model" || v.RenderCount() != 1 || b.x != 75 || b.y != 80 {
		t.Fatal("cached pick rerendered", b, v.RenderCount())
	}
	tr, err := v.GPUTransfers()
	if err != nil || tr.IDBytes != 4 {
		t.Fatal(tr, err)
	}
	v.Close()
	if b.closes != 1 || v.PickNode(50, 40) != "" {
		t.Fatal("closed view used backend")
	}
	tr, err = v.GPUTransfers()
	if err != nil || tr != (glrender.TransferStats{}) {
		t.Fatal("closed transfers", tr, err)
	}
}

func TestSceneSelectivePickFailureFallbackAndForcedBackend(t *testing.T) {
	for _, mode := range []string{"auto", "opengl"} {
		t.Run(mode, func(t *testing.T) {
			v := NewSceneView()
			defer v.Close()
			v.SetSize(160, 120)
			v.SetYaw(0)
			v.SetPitch(0)
			failure := errors.New("injected pick readback failure")
			b := &selectiveProbeBackend{err: failure}
			v.gpu = b
			v.gpuAttempted = true
			v.backendMode = mode
			id := v.PickNode(80, 60)
			if b.closes != 1 || v.gpu != nil || v.GPUError() != failure || !v.gpuAttempted {
				t.Fatal("failed backend was not retired", b, v.GPUError())
			}
			if mode == "auto" {
				if id != "model" || v.RenderCount() != 1 {
					t.Fatal("CPU fallback", id, v.RenderCount())
				}
			} else if id != "" || v.RenderCount() != 0 {
				t.Fatal("forced GPU silently fell back", id, v.RenderCount())
			}
			_ = v.PickNode(80, 60)
			if b.picks != 1 || b.closes != 1 {
				t.Fatal("retried failed backend")
			}
		})
	}
}

func TestSceneGPUTransfersDoNotCreateContext(t *testing.T) {
	v := NewSceneView()
	defer v.Close()
	tr, err := v.GPUTransfers()
	if err != nil || tr != (glrender.TransferStats{}) || v.gpu != nil || v.gpuAttempted {
		t.Fatal("statistics created backend", tr, err)
	}
	v.SetRenderBackend("cpu")
	paintScene(t, v)
	tr, err = v.GPUTransfers()
	if err != nil || tr != (glrender.TransferStats{}) {
		t.Fatal(tr, err)
	}
}
