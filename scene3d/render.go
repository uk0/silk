package scene3d

import (
	"image"
	"image/color"
	"math"
)

const MaxPixels = 16 * 1024 * 1024
const MaxDimension = 8192

// RenderStats is per actual render; Cached is true on a repeated unchanged call.
type RenderStats struct {
	Triangles, Clipped, Pixels int
	Cached                     bool
	// CulledTriangles counts source index triangles, including degenerate ones.
	// Cached frames retain the statistics from the last actual render.
	CulledInstances, CulledTriangles int
}

// Frame is renderer-owned and valid until its next Render. Treat pixels as
// read-only. Export/copy before handing a frame to another goroutine.
type Frame struct {
	Sequence uint64 // advances only after a successful rasterization

	Image    *image.RGBA
	Stats    RenderStats
	depth    []float32
	ids      []uint32
	snapshot *Snapshot
}

func (f *Frame) Pick(x, y int) string {
	if f == nil || f.Image == nil || !image.Pt(x, y).In(f.Image.Rect) {
		return ""
	}
	i := f.ids[y*f.Image.Rect.Dx()+x]
	if i == 0 {
		return ""
	}
	return f.snapshot.instances[i-1].id
}

// Renderer reuses framebuffer, depth, pick and vertex scratch buffers. It creates
// no goroutines/timers. The CPU implementation is a reference/fallback backend,
// not a claim of GPU or large-scene performance.
type Renderer struct {
	frame  Frame
	key    *Snapshot
	camera Camera
	bg     color.RGBA
	world  []Vec3
	clip   []vec4
}

func (r *Renderer) Sequence() uint64 { return r.frame.Sequence }
func (r *Renderer) Reset()           { *r = Renderer{} }
func (r *Renderer) Render(s *Snapshot, c Camera, width, height int, bg color.RGBA) (*Frame, error) {
	if s == nil {
		return nil, ErrNode
	}
	if width <= 0 || height <= 0 || width > MaxDimension || height > MaxDimension || width > MaxPixels/height {
		return nil, ErrLimit
	}
	bg.A = 255
	if r.key == s && r.camera == c && r.bg == bg && r.frame.Image != nil && r.frame.Image.Rect.Dx() == width && r.frame.Image.Rect.Dy() == height {
		r.frame.Stats.Cached = true
		return &r.frame, nil
	}
	vp, err := c.matrix(float64(width) / float64(height))
	if err != nil {
		return nil, err
	}
	if r.frame.Image == nil || r.frame.Image.Rect.Dx() != width || r.frame.Image.Rect.Dy() != height {
		r.frame.Image = image.NewRGBA(image.Rect(0, 0, width, height))
		r.frame.depth = make([]float32, width*height)
		r.frame.ids = make([]uint32, width*height)
	}
	f := &r.frame
	f.Stats = RenderStats{}
	f.snapshot = s
	clear(f.ids)
	// Reverse the normalized depth so the far-plane clear is zero. Built-in
	// clear/copy use the runtime's tuned bulk memory paths without unsafe aliases.
	clear(f.depth)
	pixels := f.Image.Pix
	pixels[0], pixels[1], pixels[2], pixels[3] = bg.R, bg.G, bg.B, 255
	for filled := 4; filled < len(pixels); {
		filled += copy(pixels[filled:], pixels[:filled])
	}
	light := Vec3{.35, .8, .45}.Unit()
	frustum := NewFrustum(vp)
	for ii, in := range s.instances {
		if !frustum.Intersects(in.mesh, in.world) {
			f.Stats.CulledInstances++
			f.Stats.CulledTriangles += in.mesh.TriangleCount()
			continue
		}
		count := len(in.mesh.vertices)
		if cap(r.world) < count {
			r.world = make([]Vec3, count)
			r.clip = make([]vec4, count)
		}
		r.world = r.world[:count]
		r.clip = r.clip[:count]
		valid := true
		for i, v := range in.mesh.vertices {
			w := in.world.Point(v)
			p := vp.project(w)
			if !w.valid() || !finite(p.x) || !finite(p.y) || !finite(p.z) || !finite(p.w) {
				valid = false
				break
			}
			r.world[i], r.clip[i] = w, p
		}
		if !valid {
			r.key = nil
			return nil, ErrGeometry
		}
		for i := 0; i < len(in.mesh.indices); i += 3 {
			a, b, d := in.mesh.indices[i], in.mesh.indices[i+1], in.mesh.indices[i+2]
			normal := r.world[b].Sub(r.world[a]).Cross(r.world[d].Sub(r.world[a])).Unit()
			if normal == (Vec3{}) {
				continue
			}
			intensity := .25 + .75*math.Abs(normal.Dot(light))
			col := color.RGBA{uint8(float64(in.color.R) * intensity), uint8(float64(in.color.G) * intensity), uint8(float64(in.color.B) * intensity), 255}
			f.Stats.Triangles++
			p, q, t := r.clip[a], r.clip[b], r.clip[d]
			ca, cb, cc := outcode(p), outcode(q), outcode(t)
			if ca&cb&cc != 0 {
				continue
			}
			if ca|cb|cc == 0 {
				r.triangle(p, q, t, uint32(ii+1), col)
				continue
			}
			f.Stats.Clipped++
			var polygon [16]vec4
			polygon[0], polygon[1], polygon[2] = p, q, t
			n := clipPolygon(&polygon, 3)
			for j := 1; j+1 < n; j++ {
				r.triangle(polygon[0], polygon[j], polygon[j+1], uint32(ii+1), col)
			}
		}
	}
	f.Sequence++
	r.key, r.camera, r.bg = s, c, bg
	return f, nil
}
func plane(v vec4, i int) float64 {
	switch i {
	case 0:
		return v.w + v.x
	case 1:
		return v.w - v.x
	case 2:
		return v.w + v.y
	case 3:
		return v.w - v.y
	case 4:
		return v.w + v.z
	default:
		return v.w - v.z
	}
}
func outcode(v vec4) uint8 {
	var b uint8
	for i := 0; i < 6; i++ {
		if plane(v, i) < 0 {
			b |= 1 << i
		}
	}
	return b
}
func clipPolygon(p *[16]vec4, n int) int {
	var q [16]vec4
	for planeID := 0; planeID < 6 && n > 0; planeID++ {
		count := 0
		a := p[n-1]
		da := plane(a, planeID)
		for i := 0; i < n; i++ {
			b := p[i]
			db := plane(b, planeID)
			if (da >= 0) != (db >= 0) {
				t := da / (da - db)
				q[count] = vec4{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t, a.z + (b.z-a.z)*t, a.w + (b.w-a.w)*t}
				count++
			}
			if db >= 0 {
				q[count] = b
				count++
			}
			a, da = b, db
		}
		copy(p[:], q[:count])
		n = count
	}
	return n
}

type screenVertex struct{ x, y, z float64 }

func edge(a, b screenVertex, x, y float64) float64 { return (b.x-a.x)*(y-a.y) - (b.y-a.y)*(x-a.x) }
func (r *Renderer) triangle(p, q, t vec4, id uint32, c color.RGBA) {
	if p.w <= 0 || q.w <= 0 || t.w <= 0 {
		return
	}
	f := &r.frame
	w, h := f.Image.Rect.Dx(), f.Image.Rect.Dy()
	project := func(v vec4) screenVertex {
		return screenVertex{(v.x/v.w + 1) * .5 * float64(w), (1 - v.y/v.w) * .5 * float64(h), v.z / v.w}
	}
	a, b, d := project(p), project(q), project(t)
	area := edge(a, b, d.x, d.y)
	if math.Abs(area) < 1e-10 {
		return
	}
	if area < 0 {
		b, d = d, b
		area = -area
	}
	x0 := max(0, int(math.Ceil(min(a.x, b.x, d.x)-.5)))
	x1 := min(w-1, int(math.Floor(max(a.x, b.x, d.x)-.5)))
	y0 := max(0, int(math.Ceil(min(a.y, b.y, d.y)-.5)))
	y1 := min(h-1, int(math.Floor(max(a.y, b.y, d.y)-.5)))
	if x0 > x1 || y0 > y1 {
		return
	}
	e0, e1, e2 := edge(b, d, float64(x0)+.5, float64(y0)+.5), edge(d, a, float64(x0)+.5, float64(y0)+.5), edge(a, b, float64(x0)+.5, float64(y0)+.5)
	dx0, dx1, dx2 := b.y-d.y, d.y-a.y, a.y-b.y
	dy0, dy1, dy2 := d.x-b.x, a.x-d.x, b.x-a.x
	z0, z1, z2 := a.z/area, b.z/area, d.z/area
	for y := y0; y <= y1; y++ {
		v0, v1, v2 := e0, e1, e2
		idx := y*w + x0
		for x := x0; x <= x1; x++ {
			if v0 >= -1e-8 && v1 >= -1e-8 && v2 >= -1e-8 {
				z := float32((1 - (v0*z0 + v1*z1 + v2*z2)) * .5)
				if z > f.depth[idx] {
					f.depth[idx] = z
					f.ids[idx] = id
					off := idx * 4
					f.Image.Pix[off], f.Image.Pix[off+1], f.Image.Pix[off+2], f.Image.Pix[off+3] = c.R, c.G, c.B, 255
					f.Stats.Pixels++
				}
			}
			v0 += dx0
			v1 += dx1
			v2 += dx2
			idx++
		}
		e0 += dy0
		e1 += dy1
		e2 += dy2
	}
}
