//go:build ignore

// glui_demo — Standalone demonstration of the silk/glui pure-OpenGL UI
// renderer.
//
// This example bypasses the silk widget framework entirely and drives the
// glui renderer directly: shapes, text, transforms, and color samples are
// drawn each frame from a hand-rolled main loop.
//
// Build:
//
//	go build -o /tmp/glui_demo examples/glui_demo/main.go
//
// Run (requires a display):
//
//	/tmp/glui_demo
package main

import (
	"log"
	"runtime"

	"silk/glui"

	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

func init() {
	// GLFW + macOS Cocoa require all window/event calls on the main OS thread.
	runtime.LockOSThread()
}

func main() {
	if err := glfw.Init(); err != nil {
		log.Fatal(err)
	}
	defer glfw.Terminate()

	// GL 2.1 is the floor silk/glui supports.
	glfw.WindowHint(glfw.ContextVersionMajor, 2)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)

	win, err := glfw.CreateWindow(800, 600, "Silk glui Demo", nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	win.MakeContextCurrent()

	if err := gl.Init(); err != nil {
		log.Fatal(err)
	}

	ctx := glui.NewContext()
	if err := ctx.Init(); err != nil {
		log.Fatal(err)
	}
	defer ctx.Destroy()

	font := glui.NewFont(14)

	glfw.SwapInterval(1)

	for !win.ShouldClose() {
		fbw, fbh := win.GetFramebufferSize()
		gl.Viewport(0, 0, int32(fbw), int32(fbh))
		gl.ClearColor(0.95, 0.95, 0.97, 1.0)
		gl.Clear(gl.COLOR_BUFFER_BIT)

		// Logical viewport is 800x600; fbw/800 yields the Retina scale.
		ctx.Resize(800, 600, float32(fbw)/800)
		r := ctx.Begin(800, 600)

		// Title.
		r.DrawText(font, "Silk glui — Pure OpenGL UI Renderer", 20, 30, glui.RGBA8(33, 37, 41, 255))

		// Color palette: a row of rounded swatches with shifting hues.
		for i := 0; i < 8; i++ {
			x := float32(20 + i*70)
			col := glui.RGBA8(uint8(50+i*20), uint8(120+i*10), 220-uint8(i*10), 255)
			r.FillRoundedRect(glui.Rect{X: x, Y: 60, W: 60, H: 60}, 8, col)
		}

		// Stroke vs fill comparison plus a circle.
		r.FillRect(glui.Rect{X: 20, Y: 140, W: 100, H: 60}, glui.RGBA8(80, 160, 240, 255))
		r.StrokeRect(glui.Rect{X: 140, Y: 140, W: 100, H: 60}, 2, glui.RGBA8(80, 160, 240, 255))
		r.FillCircle(310, 170, 30, glui.RGBA8(240, 100, 60, 255))

		// Variable-thickness line fan from a single anchor.
		for i := 0; i < 12; i++ {
			r.Line(20, 240, 20+float32(i*30), 320, float32(i+1)*0.5,
				glui.RGBA8(uint8(i*20), uint8(i*15), 200, 255))
		}

		// Text samples — anti-aliased opentype rasterisation through the atlas.
		r.DrawText(font, "Hello, World — anti-aliased opentype rendering", 20, 360, glui.RGBA8(33, 37, 41, 255))
		r.DrawText(font, "01234567890 !@#$%^&*()", 20, 384, glui.RGBA8(108, 117, 125, 255))

		// Transform demo: 12 spokes radiating from a translated origin,
		// each rotated 30 degrees from the previous one. Save/Restore
		// keeps every spoke independent so the rotations don't compound.
		r.Save()
		r.Translate(500, 300)
		for i := 0; i < 12; i++ {
			r.Save()
			r.Rotate(float32(i) * 0.5236) // ~30 degrees per step
			r.FillRect(glui.Rect{X: -8, Y: 60, W: 16, H: 60},
				glui.RGBA8(80, 160, 240, uint8(255-i*10)))
			r.Restore()
		}
		r.Restore()

		r.End()
		win.SwapBuffers()
		glfw.PollEvents()
	}
}
