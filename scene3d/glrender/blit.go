//go:build cgo && !silk_pure_go

package glrender

import (
	"fmt"
	"github.com/uk0/silk/scene3d"
	gl "github.com/uk0/silk/scene3d/glrender/internal/gl21"
	"image"
	"runtime"
)

// CanBlitColor reports an immutable construction-time capability, without GL
// calls. It does not create a context or imply hardware acceleration.
func (r *Renderer) CanBlitColor() bool { return r != nil && r.canBlit }

// BlitColor copies opaque resident color into the destination framebuffer's
// already configured draw buffer(s), using nearest filtering. All coordinates
// are top-left physical pixels. dst may extend beyond the viewport; clip is
// intersected with it without changing the original scaling/offset.
//
// The caller owns the destination framebuffer and configures its draw buffer(s)
// (normally COLOR_ATTACHMENT0 or default BACK). The renderer's own FBO is never
// a valid destination. No texture name escapes and no CPU readback is performed.
// Read/draw FBO bindings, source read selector and scissor state are restored.
// This is an opaque copy, not alpha blending or a widget-ordering policy.
func (r *Renderer) BlitColor(f GPUFrame, target uint32, width, height int, dst, clip image.Rectangle) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := r.validateGPU(f); err != nil {
		return err
	}
	if !r.canBlit {
		return ErrUnavailable
	}
	if width <= 0 || height <= 0 || width > scene3d.MaxDimension || height > scene3d.MaxDimension || width > scene3d.MaxPixels/height || dst.Min.X >= dst.Max.X || dst.Min.Y >= dst.Max.Y || target == r.fbo {
		return ErrOptions
	}
	// Bound signed coordinate subtraction before converting to GL's int32 ABI.
	for _, v := range []int{dst.Min.X, dst.Min.Y, dst.Max.X, dst.Max.Y} {
		if v < -(1<<29) || v > 1<<29 {
			return ErrOptions
		}
	}
	clip = clip.Intersect(image.Rect(0, 0, width, height)).Intersect(dst)
	if clip.Empty() {
		return nil
	}
	var read, draw, enabled int32
	var box [4]int32
	r.api.GetIntegerv(gl.READ_FRAMEBUFFER_BINDING_EXT, &read)
	r.api.GetIntegerv(gl.DRAW_FRAMEBUFFER_BINDING_EXT, &draw)
	r.api.GetIntegerv(gl.SCISSOR_TEST, &enabled)
	r.api.GetIntegerv(gl.SCISSOR_BOX, &box[0])
	r.api.BindFramebufferEXT(gl.READ_FRAMEBUFFER_EXT, r.fbo)
	var sourceRead int32
	r.api.GetIntegerv(gl.READ_BUFFER, &sourceRead)
	defer func() {
		r.api.BindFramebufferEXT(gl.READ_FRAMEBUFFER_EXT, r.fbo)
		r.api.ReadBuffer(uint32(sourceRead))
		r.api.BindFramebufferEXT(gl.READ_FRAMEBUFFER_EXT, uint32(read))
		r.api.BindFramebufferEXT(gl.DRAW_FRAMEBUFFER_EXT, uint32(draw))
		r.api.Scissor(box[0], box[1], box[2], box[3])
		if enabled != 0 {
			r.api.Enable(gl.SCISSOR_TEST)
		} else {
			r.api.Disable(gl.SCISSOR_TEST)
		}
	}()
	r.api.ReadBuffer(gl.COLOR_ATTACHMENT0_EXT)
	r.api.BindFramebufferEXT(gl.DRAW_FRAMEBUFFER_EXT, target)
	r.api.Enable(gl.SCISSOR_TEST)
	r.api.Scissor(int32(clip.Min.X), int32(height-clip.Max.Y), int32(clip.Dx()), int32(clip.Dy()))
	r.api.BlitFramebufferEXT(0, 0, int32(r.width), int32(r.height), int32(dst.Min.X), int32(height-dst.Max.Y), int32(dst.Max.X), int32(height-dst.Min.Y), gl.COLOR_BUFFER_BIT, gl.NEAREST)
	if e := r.api.GetError(); e != gl.NO_ERROR {
		return fmt.Errorf("%w: color blit %#x", ErrUnavailable, e)
	}
	r.transfers.ColorBlits++
	return nil
}
