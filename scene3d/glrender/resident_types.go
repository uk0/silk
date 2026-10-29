package glrender

import "errors"

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
