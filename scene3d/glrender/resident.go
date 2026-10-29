//go:build cgo && !silk_pure_go

package glrender

import (
	"fmt"
	gl "github.com/uk0/silk/scene3d/glrender/internal/gl21"
	"image"
	"runtime"
)

func (r *Renderer) gpuFrame(cached bool) GPUFrame {
	stats := r.stats
	stats.Cached = cached
	return GPUFrame{r, r.generation, r.width, r.height, r.sequence, stats}
}

// Call only with the OS thread locked. Ownership is checked before mutable
// renderer fields, preserving rejection of workers racing owner-thread Close.
func (r *Renderer) validateGPU(f GPUFrame) error {
	if r == nil || r.api == nil {
		return ErrClosed
	}
	if !r.api.OnThread() {
		return ErrContext
	}
	if r.closed {
		return ErrClosed
	}
	if !r.api.Current() {
		return ErrContext
	}
	if f.owner != r || f.generation != r.generation || r.key == nil {
		return ErrFrameStale
	}
	return nil
}

// Transfers must be called on the owning OS thread. Rejected callers do not read
// mutable counters. It remains available after Close on the owning thread.
func (r *Renderer) Transfers() (TransferStats, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if r == nil || r.api == nil {
		return TransferStats{}, ErrClosed
	}
	if !r.api.OnThread() {
		return TransferStats{}, ErrContext
	}
	return r.transfers, nil
}

func (r *Renderer) prepareReadback() {
	r.api.BindFramebufferEXT(gl.FRAMEBUFFER_EXT, r.fbo)
	r.api.BindBuffer(gl.PIXEL_PACK_BUFFER, 0)
	r.api.PixelStorei(gl.PACK_ALIGNMENT, 4)
	r.api.PixelStorei(gl.PACK_ROW_LENGTH, 0)
	r.api.PixelStorei(gl.PACK_SKIP_ROWS, 0)
	r.api.PixelStorei(gl.PACK_SKIP_PIXELS, 0)
}

// Readback captures both attachments for CPU composition/export. Host buffers
// are allocated lazily and reused until Close. Like the legacy Render result,
// the returned Frame is borrowed until another Readback/Render or Close.
// This changes GL framebuffer and pixel-pack state; it does not draw again.
func (r *Renderer) Readback(f GPUFrame) (*Frame, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := r.validateGPU(f); err != nil {
		return nil, err
	}
	if r.capturedGeneration == r.generation && r.frame.Image != nil {
		r.frame.Stats.Cached = true
		return &r.frame, nil
	}
	w, h := r.width, r.height
	if r.frame.Image == nil || r.frame.Image.Rect != image.Rect(0, 0, w, h) {
		r.frame.Image = image.NewRGBA(image.Rect(0, 0, w, h))
		r.frame.pickPixels = make([]byte, w*h*4)
		r.readback = make([]byte, w*h*4)
	}
	r.prepareReadback()
	r.api.ReadBuffer(gl.COLOR_ATTACHMENT0_EXT)
	r.api.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Pointer(r.readback))
	if e := r.api.GetError(); e != gl.NO_ERROR {
		return nil, fmt.Errorf("%w: color readback %#x", ErrUnavailable, e)
	}
	r.transfers.ColorReads++
	r.transfers.ColorBytes += uint64(w) * uint64(h) * 4
	r.api.ReadBuffer(gl.COLOR_ATTACHMENT1_EXT)
	r.api.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Pointer(r.frame.pickPixels))
	if e := r.api.GetError(); e != gl.NO_ERROR {
		return nil, fmt.Errorf("%w: ID readback %#x", ErrUnavailable, e)
	}
	r.transfers.IDReads++
	r.transfers.IDBytes += uint64(w) * uint64(h) * 4
	stride := w * 4
	for y := 0; y < h; y++ {
		copy(r.frame.Image.Pix[y*stride:(y+1)*stride], r.readback[(h-y-1)*stride:(h-y)*stride])
	}
	r.frame.snapshot = r.key
	r.frame.Sequence = r.sequence
	r.frame.Stats = r.stats
	r.frame.Stats.ReadbackBytes = int64(w) * int64(h) * 8
	r.capturedGeneration = r.generation
	return &r.frame, nil
}

// PickGPU synchronously reads one raster ID pixel using top-left physical-pixel
// coordinates. It never substitutes a geometric ray hit for raster visibility.
// Outside the frame returns an empty ID without a transfer. No CPU frame is
// allocated. As with Readback, framebuffer/pixel-pack state is changed.
func (r *Renderer) PickGPU(f GPUFrame, x, y int) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := r.validateGPU(f); err != nil {
		return "", err
	}
	if x < 0 || y < 0 || x >= r.width || y >= r.height {
		return "", nil
	}
	r.prepareReadback()
	r.api.ReadBuffer(gl.COLOR_ATTACHMENT1_EXT)
	var pixel [4]byte
	r.api.ReadPixels(int32(x), int32(r.height-1-y), 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, gl.Pointer(pixel[:]))
	if e := r.api.GetError(); e != gl.NO_ERROR {
		return "", fmt.Errorf("%w: pixel readback %#x", ErrUnavailable, e)
	}
	r.transfers.IDReads++
	r.transfers.IDBytes += 4
	id := int(pixel[0]) | int(pixel[1])<<8 | int(pixel[2])<<16
	n, ok := r.key.Instance(id - 1)
	if !ok {
		return "", nil
	}
	return n.ID, nil
}
