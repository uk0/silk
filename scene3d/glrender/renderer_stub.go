//go:build !cgo || silk_pure_go

package glrender

import (
	"github.com/uk0/silk/scene3d"
	"image"
	"image/color"
)

type Renderer struct{}

func NewCurrent() (*Renderer, error) { return nil, ErrUnavailable }
func NewCurrentWithOptions(o Options) (*Renderer, error) {
	if !o.valid() {
		return nil, ErrOptions
	}
	return nil, ErrUnavailable
}
func (*Renderer) Capabilities() Capabilities { return Capabilities{} }
func (*Renderer) Info() Info                 { return Info{} }
func (*Renderer) Close()                     {}
func (*Renderer) CloseCurrent() error        { return nil }
func (*Renderer) Render(*scene3d.Snapshot, scene3d.Camera, int, int, color.RGBA) (*Frame, error) {
	return nil, ErrUnavailable
}

func (*Renderer) RenderGPU(*scene3d.Snapshot, scene3d.Camera, int, int, color.RGBA) (GPUFrame, error) {
	return GPUFrame{}, ErrUnavailable
}
func (*Renderer) Readback(GPUFrame) (*Frame, error)          { return nil, ErrUnavailable }
func (*Renderer) PickGPU(GPUFrame, int, int) (string, error) { return "", ErrUnavailable }
func (*Renderer) Transfers() (TransferStats, error)          { return TransferStats{}, ErrUnavailable }

func (*Renderer) CanBlitColor() bool { return false }
func (*Renderer) BlitColor(GPUFrame, uint32, int, int, image.Rectangle, image.Rectangle) error {
	return ErrUnavailable
}
