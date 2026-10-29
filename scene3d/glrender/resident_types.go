package glrender

import (
	"errors"
	"image"
)

// ErrFrameStale rejects zero, foreign, retired or overwritten GPU frame leases.
var ErrFrameStale = errors.New("scene3d: stale or foreign GPU frame")

// GPUFrame is an opaque borrowed lease, not a framebuffer or texture name. It is
// valid only on the renderer's owning thread/context until a changed render or
// Close. Identical cached renders keep it valid. Metadata is an immutable copy.
// It intentionally does not implement scene3d.FrameView: no CPU image exists.
type GPUFrame struct {
	owner         *Renderer
	generation    uint64
	width, height int
	sequence      uint64
	stats         Stats
}

func (f GPUFrame) Size() (int, int)       { return f.width, f.height }
func (f GPUFrame) RenderSequence() uint64 { return f.sequence }
func (f GPUFrame) Statistics() Stats      { return f.stats }

// TransferStats counts successful GPU blits and synchronous readbacks over the renderer's
// lifetime. A single PickGPU transfers four ID bytes, not two full attachments.
// These are API counters, not driver timing or whole-application performance.
type TransferStats struct {
	ColorBlits          uint64
	ColorReads, IDReads uint64
	ColorBytes, IDBytes uint64
}

// ColorFrame is a borrowed top-left RGBA capture without a CPU ID buffer. It
// deliberately does not implement scene3d.FrameView: use PickGPU for raster IDs
// or Readback for a complete CPU frame. Image storage is reused by subsequent
// ReadColor/Readback/Render calls and released on Close; copy before retaining it
// or handing it to another goroutine. Sequence and Stats are value metadata.
type ColorFrame struct {
	Image    *image.RGBA
	Sequence uint64
	Stats    Stats
}

func (f *ColorFrame) RGBA() *image.RGBA      { return f.Image }
func (f *ColorFrame) RenderSequence() uint64 { return f.Sequence }
func (f *ColorFrame) IsCached() bool         { return f.Stats.Cached }
