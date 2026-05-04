// Package glui is Silk's pure-OpenGL 2D rendering pipeline.
//
// This is an alternative to the Cairo-based paint package. Where paint
// uploads a CPU-rendered framebuffer to the GPU once per frame, glui drives
// the GPU directly with shader programs, vertex buffers, and a glyph atlas.
//
// # Architecture
//
//	glui.Context            — owns shader programs and global GL state
//	  └─ glui.Renderer      — per-frame command recorder + flush
//	       ├─ Rect / Round  — solid + bordered rectangles, rounded corners
//	       ├─ Path          — arbitrary 2D paths, triangulated to a fan VBO
//	       ├─ Glyph         — text via signed-distance-field atlas
//	       ├─ Image         — textured quads (icons, backgrounds)
//	       └─ Transform     — modelview stack for translate/scale/rotate/clip
//
// All draw calls are batched into a small number of vkPipeline-style buckets
// and flushed in one DrawArrays call per material change. The target is
// <50 GL calls per frame for typical UI content.
//
// # Why
//
// CPU rasterization (Cairo) caps at the framebuffer fill rate of a single
// CPU thread. On a 4K Retina display rendering a designer with 200 widgets,
// that is 8.3M pixels × per-pixel Cairo work, ~6-8ms even on M2 Max.
// Direct GPU rendering moves that to the GPU at sub-millisecond budgets and
// frees the CPU for layout, hit-testing, and application code.
//
// # Status
//
// Experimental, on the `opengl` branch. The Cairo pipeline (paint/) remains
// the production renderer on `main`.
package glui
