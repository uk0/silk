//go:build cgo && !silk_pure_go

package gui

import (
	"github.com/uk0/silk/scene3d"
	"github.com/uk0/silk/scene3d/glrender"
	"image/color"
)

// Keep the existing ordered Cairo composition, but never download a full ID
// attachment merely to draw. Native context binding is shared with picking.
func (b *nativeSceneGPU) Render(s *scene3d.Snapshot, c scene3d.Camera, w, h int, bg color.RGBA) (sceneViewFrame, error) {
	var frame *glrender.ColorFrame
	err := b.withCurrent(func(r *glrender.Renderer) error {
		gpu, err := r.RenderGPU(s, c, w, h, bg)
		if err != nil {
			return err
		}
		frame, err = r.ReadColor(gpu)
		if err == nil {
			// A preceding pick may have rendered without capturing color. RenderCount
			// counts draws, not captures; upload still keys off the render sequence.
			b.frame = *frame
			b.frame.Stats.Cached = gpu.Statistics().Cached
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &b.frame, nil
}

func (b *nativeSceneGPU) Pick(s *scene3d.Snapshot, c scene3d.Camera, w, h int, bg color.RGBA, x, y int) (id string, cached bool, err error) {
	err = b.withCurrent(func(r *glrender.Renderer) error {
		gpu, e := r.RenderGPU(s, c, w, h, bg)
		if e != nil {
			return e
		}
		cached = gpu.Statistics().Cached
		id, e = r.PickGPU(gpu, x, y)
		return e
	})
	return
}

func (b *nativeSceneGPU) Transfers() (stats glrender.TransferStats, err error) {
	err = b.withCurrent(func(r *glrender.Renderer) error {
		var e error
		stats, e = r.Transfers()
		return e
	})
	return
}
