//go:build cgo && !silk_pure_go

package glrender

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"runtime"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/uk0/silk/scene3d"
	gl "github.com/uk0/silk/scene3d/glrender/internal/gl21"
)

var nativeResidentRan, nativeResidentMissingBlit bool
var nativeResidentErr error

func TestNativeGPUResidentFramesBlitPickAndRetirement(t *testing.T) {
	if !nativeResidentRan {
		t.Skip("set SILK_TEST_SCENE_GPU=1 for actual driver/context validation")
	}
	if nativeResidentErr != nil {
		t.Fatal(nativeResidentErr)
	}
	if nativeResidentMissingBlit {
		if os.Getenv("SILK_TEST_REQUIRE_BLIT") == "1" {
			t.Fatal("framebuffer blit required but not advertised")
		}
		t.Skip("framebuffer blit unavailable on this context")
	}
}

func nativeResidentProbe() error {
	r, err := NewCurrent()
	if err != nil {
		return err
	}
	defer r.Close()
	if !r.CanBlitColor() {
		nativeResidentMissingBlit = true
		return nil
	}
	s := scene3d.NewScene()
	if err = s.Add(scene3d.Node{ID: "resident", Mesh: scene3d.Box(), Color: color.RGBA{217, 64, 35, 255}}); err != nil {
		return err
	}
	if err = s.Add(scene3d.Node{ID: "asymmetric", Mesh: scene3d.Box(), Color: color.RGBA{35, 210, 217, 255}, Transform: scene3d.Translation(scene3d.Vec3{X: 1.5, Y: 1.1, Z: 1.2}).Mul(scene3d.Scaling(scene3d.Vec3{X: .25, Y: .35, Z: .3}))}); err != nil {
		return err
	}
	snap, err := s.Snapshot()
	if err != nil {
		return err
	}
	c := scene3d.DefaultCamera()
	c.Eye = scene3d.Vec3{Z: 6}
	bg := color.RGBA{20, 28, 41, 255}
	f, err := r.RenderGPU(snap, c, 64, 48, bg)
	if err != nil {
		return err
	}
	if f.sequence != 1 || f.stats.ReadbackBytes != 0 || f.stats.Cached || r.frame.Image != nil || len(r.readback) != 0 || len(r.frame.pickPixels) != 0 {
		return fmt.Errorf("resident render allocated/read CPU frame")
	}
	tr, err := r.Transfers()
	if err != nil || tr != (TransferStats{}) {
		return fmt.Errorf("resident transfer counters %+v %v", tr, err)
	}
	cached, err := r.RenderGPU(snap, c, 64, 48, bg)
	if err != nil || !cached.stats.Cached || cached.generation != f.generation {
		return fmt.Errorf("resident cache lease %v", err)
	}
	if _, err = r.RenderGPU(nil, c, 64, 48, bg); !errors.Is(err, scene3d.ErrNode) {
		return fmt.Errorf("invalid snapshot %v", err)
	}
	if _, err = r.Readback(GPUFrame{}); !errors.Is(err, ErrFrameStale) {
		return fmt.Errorf("zero frame %v", err)
	}
	other, err := NewCurrent()
	if err != nil {
		return err
	}
	defer other.Close()
	if _, err = other.PickGPU(f, 0, 0); !errors.Is(err, ErrFrameStale) {
		return fmt.Errorf("foreign renderer lease %v", err)
	}
	r.api.PixelStorei(gl.PACK_SKIP_ROWS, 2)
	r.api.PixelStorei(gl.PACK_SKIP_PIXELS, 3)
	id, err := r.PickGPU(f, 32, 24)
	if err != nil || id != "resident" {
		return fmt.Errorf("resident raster pick %q %v", id, err)
	}
	if id, err = r.PickGPU(f, -1, 24); err != nil || id != "" {
		return fmt.Errorf("outside resident pick %q %v", id, err)
	}
	tr, _ = r.Transfers()
	if tr.ColorBytes != 0 || tr.IDBytes != 4 || tr.IDReads != 1 || r.frame.Image != nil {
		return fmt.Errorf("pixel pick performed full readback %+v", tr)
	}
	// A context switch is rejected before using the lease, even on the owner thread.
	current := glfw.GetCurrentContext()
	w, err := glfw.CreateWindow(16, 16, "resident wrong-context probe", nil, nil)
	if err != nil {
		return err
	}
	w.MakeContextCurrent()
	_, readErr := r.Readback(f)
	_, pickErr := r.PickGPU(f, 32, 24)
	blitErr := r.BlitColor(f, 0, 16, 16, image.Rect(0, 0, 16, 16), image.Rect(0, 0, 16, 16))
	current.MakeContextCurrent()
	w.Destroy()
	if !errors.Is(readErr, ErrContext) || !errors.Is(pickErr, ErrContext) || !errors.Is(blitErr, ErrContext) {
		return fmt.Errorf("wrong context was not rejected: %v %v %v", readErr, pickErr, blitErr)
	}
	if err = residentBlitProbe(r, f); err != nil {
		return err
	}
	cpu, err := r.Readback(f)
	if err != nil {
		return err
	}
	pixels := append([]byte(nil), cpu.Image.Pix...)
	tr, _ = r.Transfers()
	if tr.ColorReads != 1 || tr.IDReads != 2 || tr.ColorBytes != 64*48*4 || tr.IDBytes != 64*48*4+4 || tr.ColorBlits != 3 {
		return fmt.Errorf("capture transfer counts %+v", tr)
	}
	again, err := r.Readback(f)
	after, _ := r.Transfers()
	if err != nil || !again.IsCached() || after != tr || !bytes.Equal(again.Image.Pix, pixels) {
		return fmt.Errorf("cached capture %v %+v", err, after)
	}
	for _, p := range []image.Point{{0, 0}, {32, 24}, {20, 20}, {42, 31}, {63, 47}} {
		id, err := r.PickGPU(f, p.X, p.Y)
		if err != nil || id != cpu.Pick(p.X, p.Y) {
			return fmt.Errorf("pixel/capture ID mismatch at %v: %q %v", p, id, err)
		}
	}
	next, err := r.RenderGPU(snap, c, 80, 60, bg)
	if err != nil {
		return err
	}
	if next.sequence != 2 || next.generation == f.generation {
		return fmt.Errorf("changed size did not retire lease")
	}
	if _, err = r.Readback(f); !errors.Is(err, ErrFrameStale) {
		return fmt.Errorf("stale capture %v", err)
	}
	if _, err = r.PickGPU(f, 32, 24); !errors.Is(err, ErrFrameStale) {
		return fmt.Errorf("stale pick %v", err)
	}
	if err = r.BlitColor(f, 0, 32, 32, image.Rect(0, 0, 32, 32), image.Rect(0, 0, 32, 32)); !errors.Is(err, ErrFrameStale) {
		return fmt.Errorf("stale blit %v", err)
	}
	// Concurrent misuse is rejected without reading owner-mutated state, including Close.
	ready, done := make(chan struct{}), make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		close(ready)
		for i := 0; i < 64; i++ {
			_, a := r.RenderGPU(snap, c, 80, 60, bg)
			_, b := r.Readback(next)
			_, d := r.PickGPU(next, 4, 4)
			_, e := r.Transfers()
			x := r.BlitColor(next, 0, 32, 32, image.Rect(0, 0, 32, 32), image.Rect(0, 0, 32, 32))
			for _, err := range []error{a, b, d, e, x} {
				if !errors.Is(err, ErrContext) {
					done <- fmt.Errorf("worker error %v", err)
					return
				}
			}
		}
		done <- nil
	}()
	<-ready
	if err = r.CloseCurrent(); err != nil {
		return err
	}
	if err = <-done; err != nil {
		return err
	}
	if _, err = r.Readback(next); !errors.Is(err, ErrClosed) {
		return fmt.Errorf("closed capture %v", err)
	}
	fmt.Println("GPU_RESIDENT_PASS zero_full_readback one_pixel_pick blit_clip_scale_state stale_context_thread_close")
	return nil
}

func residentBlitProbe(r *Renderer, f GPUFrame) error {
	const width, height = 128, 96
	var target, texture uint32
	r.api.GenFramebuffersEXT(1, &target)
	r.api.GenTextures(1, &texture)
	defer r.api.DeleteFramebuffersEXT(1, &target)
	defer r.api.DeleteTextures(1, &texture)
	r.api.BindFramebufferEXT(gl.FRAMEBUFFER_EXT, target)
	r.api.BindBuffer(gl.PIXEL_UNPACK_BUFFER, 0)
	r.api.BindTexture(gl.TEXTURE_2D, texture)
	r.api.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, width, height, 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	r.api.FramebufferTexture2DEXT(gl.FRAMEBUFFER_EXT, gl.COLOR_ATTACHMENT0_EXT, gl.TEXTURE_2D, texture, 0)
	r.api.DrawBuffer(gl.COLOR_ATTACHMENT0_EXT)
	if r.api.CheckFramebufferStatusEXT(gl.FRAMEBUFFER_EXT) != gl.FRAMEBUFFER_COMPLETE_EXT {
		return fmt.Errorf("external blit target incomplete")
	}
	var captures [][]byte
	rects := []image.Rectangle{image.Rect(13, 7, 77, 55), image.Rect(-4, -6, 124, 90), image.Rect(0, 0, 128, 96)}
	clips := []image.Rectangle{image.Rect(21, 15, 70, 50), image.Rect(3, 4, 110, 90), image.Rect(0, 0, 128, 96)}
	for i, dst := range rects {
		r.api.BindFramebufferEXT(gl.FRAMEBUFFER_EXT, target)
		r.api.Disable(gl.SCISSOR_TEST)
		r.api.ClearColor(17.0/255, 29.0/255, 83.0/255, 1)
		r.api.Clear(gl.COLOR_BUFFER_BIT)
		// Distinct read/draw bindings and a nontrivial inherited scissor must survive.
		r.api.BindFramebufferEXT(gl.READ_FRAMEBUFFER_EXT, r.fbo)
		r.api.ReadBuffer(gl.COLOR_ATTACHMENT1_EXT)
		r.api.Scissor(1, 2, 3, 4)
		r.api.Enable(gl.SCISSOR_TEST)
		if err := r.BlitColor(f, target, width, height, dst, clips[i]); err != nil {
			return err
		}
		var read, draw, enabled, selector int32
		var box [4]int32
		r.api.GetIntegerv(gl.READ_FRAMEBUFFER_BINDING_EXT, &read)
		r.api.GetIntegerv(gl.DRAW_FRAMEBUFFER_BINDING_EXT, &draw)
		r.api.GetIntegerv(gl.SCISSOR_TEST, &enabled)
		r.api.GetIntegerv(gl.SCISSOR_BOX, &box[0])
		r.api.GetIntegerv(gl.READ_BUFFER, &selector)
		if uint32(read) != r.fbo || uint32(draw) != target || enabled != 1 || box != [4]int32{1, 2, 3, 4} || selector != gl.COLOR_ATTACHMENT1_EXT {
			return fmt.Errorf("blit leaked host state %d %d %d %v %d", read, draw, enabled, box, selector)
		}
		if r.frame.Image != nil || len(r.readback) != 0 || len(r.frame.pickPixels) != 0 {
			return fmt.Errorf("GPU blit allocated full CPU frame")
		}
		pixels := make([]byte, width*height*4)
		r.api.BindFramebufferEXT(gl.READ_FRAMEBUFFER_EXT, target)
		r.api.ReadBuffer(gl.COLOR_ATTACHMENT0_EXT)
		r.api.BindBuffer(gl.PIXEL_PACK_BUFFER, 0)
		r.api.PixelStorei(gl.PACK_ROW_LENGTH, 0)
		r.api.PixelStorei(gl.PACK_ALIGNMENT, 4)
		r.api.ReadPixels(0, 0, width, height, gl.RGBA, gl.UNSIGNED_BYTE, gl.Pointer(pixels))
		captures = append(captures, pixels)
	}
	before, _ := r.Transfers()
	if err := r.BlitColor(f, target, width, height, rects[0], image.Rectangle{}); err != nil {
		return err
	}
	if after, _ := r.Transfers(); after != before {
		return fmt.Errorf("empty clip caused transfer")
	}
	for _, dst := range []image.Rectangle{{}, image.Rect(0, 0, 1<<31, 1), {Min: image.Pt(10, 10), Max: image.Pt(5, 5)}} {
		if err := r.BlitColor(f, target, width, height, dst, clips[0]); !errors.Is(err, ErrOptions) {
			return fmt.Errorf("bad blit rectangle: %v", err)
		}
	}
	if err := r.BlitColor(f, r.fbo, width, height, rects[0], clips[0]); !errors.Is(err, ErrOptions) {
		return fmt.Errorf("source destination alias: %v", err)
	}
	r.api.PixelStorei(gl.PACK_SKIP_ROWS, 5)
	r.api.PixelStorei(gl.PACK_SKIP_PIXELS, 7)
	ref, err := r.Readback(f)
	if err != nil {
		return err
	}
	seenAsymmetric := false
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			if ref.Pick(x, y) == "asymmetric" {
				seenAsymmetric = true
			}
		}
	}
	if !seenAsymmetric {
		return fmt.Errorf("orientation fixture did not expose asymmetric marker")
	}
	for i, pixels := range captures {
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				want := color.RGBA{17, 29, 83, 255}
				p := image.Pt(x, y)
				if p.In(rects[i]) && p.In(clips[i]) {
					want = ref.Image.RGBAAt((x-rects[i].Min.X)*64/rects[i].Dx(), (y-rects[i].Min.Y)*48/rects[i].Dy())
				}
				k := ((height-1-y)*width + x) * 4
				got := color.RGBA{pixels[k], pixels[k+1], pixels[k+2], pixels[k+3]}
				if got != want {
					return fmt.Errorf("blit case=%d pixel=%v got=%v want=%v", i, p, got, want)
				}
			}
		}
	}
	return nil
}
