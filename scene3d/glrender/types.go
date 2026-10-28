// Package glrender implements a real OpenGL offscreen backend for scene3d.
// It requires a dedicated OpenGL 2.1 compatibility context with EXT framebuffer
// objects and two draw buffers. The host owns the context and OS thread; call
// NewCurrent, Render and Close only while that same context is current.
// The CPU readback preserves ordinary UI composition, not a floating overlay.
package glrender

import (
	"errors"
	"github.com/uk0/silk/scene3d"
	"image"
)

var ErrUnavailable = errors.New("scene3d: OpenGL backend unavailable")
var ErrClosed = errors.New("scene3d: OpenGL renderer closed")

// Info reports the driver's own identity; OpenGL may be software-emulated, so
// this alone does not certify hardware acceleration.
type Info struct{ Vendor, Renderer, Version string }

type Stats struct {
	DrawCalls, Triangles, MeshUploads int
	ResidentMeshes                    int
	ResidentBytes, ReadbackBytes      int64
	Cached                            bool
}

type Frame struct {
	Image      *image.RGBA
	Sequence   uint64
	Stats      Stats
	pickPixels []byte // bottom-up RGBA IDs from the second color attachment
	snapshot   *scene3d.Snapshot
}

func (f *Frame) RGBA() *image.RGBA      { return f.Image }
func (f *Frame) RenderSequence() uint64 { return f.Sequence }
func (f *Frame) Pick(x, y int) string {
	if f == nil || f.Image == nil || !image.Pt(x, y).In(f.Image.Rect) {
		return ""
	}
	i := ((f.Image.Rect.Dy()-1-y)*f.Image.Rect.Dx() + x) * 4
	id := int(f.pickPixels[i]) | int(f.pickPixels[i+1])<<8 | int(f.pickPixels[i+2])<<16
	n, ok := f.snapshot.Instance(id - 1)
	if !ok {
		return ""
	}
	return n.ID
}

func (f *Frame) IsCached() bool { return f.Stats.Cached }
