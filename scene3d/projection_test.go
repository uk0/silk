package scene3d

import (
	"math"
	"math/rand"
	"testing"
)

func projectionNear(t testing.TB, a, b Vec3, tolerance float64) {
	t.Helper()
	if a.Sub(b).Length() > tolerance || !a.valid() || !b.valid() {
		t.Fatalf("vector mismatch: %v != %v (tolerance %g)", a, b, tolerance)
	}
}
func TestCameraProjectRayAndViewportScale(t *testing.T) {
	c := DefaultCamera()
	forward := c.Target.Sub(c.Eye).Unit()
	for _, size := range [][2]float64{{640, 480}, {300, 700}, {1280, 960}, {.5, .25}} {
		for _, point := range []Vec3{{}, {X: .5}, {Y: 1}, {Z: -2}} {
			pixel, visible, err := c.Project(point, size[0], size[1])
			mustEdit(t, err)
			if !visible || pixel.Z < 0 || pixel.Z > 1 {
				t.Fatal("ordinary visible point rejected", point, pixel)
			}
			ray, err := c.Ray(pixel.X, pixel.Y, size[0], size[1])
			mustEdit(t, err)
			hit, ok := ray.IntersectPlane(point, forward)
			if !ok || math.Abs(ray.Direction.Length()-1) > 1e-12 || ray.Origin != c.Eye {
				t.Fatal("invalid ray", ray, hit, ok)
			}
			projectionNear(t, hit, point, 1e-10)
		}
	}
	for _, point := range []Vec3{c.Eye, c.Eye.Sub(forward), c.Eye.Add(forward.Mul(c.Near / 2)), c.Eye.Add(forward.Mul(c.Far * 2)), {X: 1e5}} {
		_, visible, err := c.Project(point, 640, 480)
		mustEdit(t, err)
		if visible {
			t.Fatal("unclipped point", point)
		}
	}
	// Captured drags may leave the viewport without clamping input to an edge.
	r, err := c.Ray(-640, 1200, 640, 480)
	mustEdit(t, err)
	p := r.Origin.Add(r.Direction.Mul(50))
	screen, visible, err := c.Project(p, 640, 480)
	mustEdit(t, err)
	if visible || math.Abs(screen.X+640) > 1e-9 || math.Abs(screen.Y-1200) > 1e-9 {
		t.Fatal("offscreen drag clamped", screen, visible)
	}
}
func TestProjectionInvalidAndPlaneHalfRay(t *testing.T) {
	c := DefaultCamera()
	for _, size := range [][2]float64{{0, 2}, {2, 0}, {-1, 1}, {math.NaN(), 1}, {1, math.Inf(1)}, {math.MaxFloat64, math.SmallestNonzeroFloat64}} {
		if _, _, err := c.Project(Vec3{}, size[0], size[1]); err == nil {
			t.Fatal("bad projection size accepted", size)
		}
		if _, err := c.Ray(0, 0, size[0], size[1]); err == nil {
			t.Fatal("bad ray size accepted", size)
		}
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, _, err := c.Project(Vec3{X: bad}, 640, 480); err == nil {
			t.Fatal("bad point accepted")
		}
		if _, err := c.Ray(bad, 0, 640, 480); err == nil {
			t.Fatal("bad cursor accepted")
		}
		broken := c
		broken.Near = bad
		if _, err := broken.Ray(0, 0, 640, 480); err == nil {
			t.Fatal("invalid camera accepted")
		}
	}
	for _, ray := range []Ray{{}, {Origin: Vec3{Z: 5}, Direction: Vec3{Z: 1}}, {Origin: Vec3{Z: 5}, Direction: Vec3{X: 1}}, {Origin: Vec3{Z: 5}, Direction: Vec3{X: 1, Z: -1e-12}}} {
		if _, ok := ray.IntersectPlane(Vec3{}, Vec3{Z: 1}); ok {
			t.Fatal("invalid/behind/parallel intersection", ray)
		}
	}
	for _, magnitude := range []float64{1, 1e300, 1e-300, math.SmallestNonzeroFloat64} {
		ray := Ray{Origin: Vec3{X: 3, Z: 5}, Direction: Vec3{Z: -magnitude}}
		hit, ok := ray.IntersectPlane(Vec3{}, Vec3{Z: magnitude})
		if !ok {
			t.Fatal("finite nonzero direction/normal scale rejected", magnitude)
		}
		projectionNear(t, hit, Vec3{X: 3}, 1e-12)
	}
	if _, ok := (Ray{Direction: Vec3{Z: -1}}).IntersectPlane(Vec3{}, Vec3{}); ok {
		t.Fatal("zero normal")
	}
}
func TestAxisDragOffsetSnappingAndNoJump(t *testing.T) {
	c := DefaultCamera()
	origin := Vec3{X: .2, Y: -.1}
	for _, axis := range []Vec3{{X: 1}, {Y: 1}, {Z: 1}, {X: 2, Y: 3, Z: -1}} {
		unit := axis.Unit()
		initial := origin.Add(unit.Mul(.3)) // grab partway along a handle
		pixel, _, err := c.Project(initial, 640, 480)
		mustEdit(t, err)
		start, err := c.Ray(pixel.X, pixel.Y, 640, 480)
		mustEdit(t, err)
		drag, err := NewAxisDrag(start, origin, axis)
		mustEdit(t, err)
		delta, err := drag.Offset(start, 0)
		mustEdit(t, err)
		projectionNear(t, delta, Vec3{}, 1e-12)
		for _, distance := range []float64{.7, -.6, .49, -.49, 0, .04} {
			pixel, _, err = c.Project(initial.Add(unit.Mul(distance)), 640, 480)
			mustEdit(t, err)
			ray, err := c.Ray(pixel.X, pixel.Y, 640, 480)
			mustEdit(t, err)
			for _, step := range []float64{0, .25} {
				delta, err = drag.Offset(ray, step)
				mustEdit(t, err)
				want := distance
				if step > 0 {
					want = math.Round(distance/step) * step
				}
				projectionNear(t, delta, unit.Mul(want), 1e-10)
			}
		}
		if allocs := testing.AllocsPerRun(100, func() { _, _ = drag.Offset(start, .25) }); allocs != 0 {
			t.Fatal("axis solver allocates", allocs)
		}
	}
}
func TestAxisDragRejectsAmbiguousAndInvalidInput(t *testing.T) {
	ray := Ray{Origin: Vec3{Z: 10}, Direction: Vec3{Z: -1}}
	for _, axis := range []Vec3{{}, {Z: 1}, {X: 1e-8, Z: 1}, {X: math.NaN()}} {
		if _, err := NewAxisDrag(ray, Vec3{}, axis); err == nil {
			t.Fatal("ambiguous/invalid axis accepted", axis)
		}
	}
	d, err := NewAxisDrag(ray, Vec3{}, Vec3{X: 1})
	mustEdit(t, err)
	for _, step := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := d.Offset(ray, step); err == nil {
			t.Fatal("invalid snap accepted", step)
		}
	}
	for _, bad := range []Ray{{}, {Origin: ray.Origin, Direction: Vec3{Z: 1}}, {Origin: ray.Origin, Direction: Vec3{X: 1}}, {Origin: Vec3{X: math.Inf(1)}, Direction: ray.Direction}} {
		if _, err := d.Offset(bad, 0); err == nil {
			t.Fatal("invalid drag ray accepted", bad)
		}
	}
	if _, err := (AxisDrag{}).Offset(ray, 0); err == nil {
		t.Fatal("uninitialized solver accepted")
	}
	if _, err := d.Offset(Ray{Origin: Vec3{X: 10, Z: 10}, Direction: ray.Direction}, math.SmallestNonzeroFloat64); err == nil {
		t.Fatal("snap ratio overflow accepted")
	}
}
func FuzzCameraProjectionRoundTrip(f *testing.F) {
	for _, seed := range []int64{0, 1, 7, 99, 104729, -1} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		c := DefaultCamera()
		c.Eye = Vec3{r.Float64()*20 - 10, r.Float64()*20 - 10, r.Float64()*20 + 2}
		c.FOVY = .1 + r.Float64()*2.8
		w, h := 20+r.Float64()*2000, 20+r.Float64()*2000
		x, y := (r.Float64()*3-1)*w, (r.Float64()*3-1)*h
		ray, err := c.Ray(x, y, w, h)
		mustEdit(t, err)
		point := ray.Origin.Add(ray.Direction.Mul(10 + r.Float64()*100))
		p, _, err := c.Project(point, w, h)
		mustEdit(t, err)
		if math.Abs(p.X-x) > 1e-7 || math.Abs(p.Y-y) > 1e-7 {
			t.Fatal("ray/projection convention mismatch", seed, p, x, y)
		}
	})
}
