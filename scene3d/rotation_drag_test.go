package scene3d

import (
	"math"
	"testing"
)

func TestRotationAxisAndMultiTurnDrag(t *testing.T) {
	for _, axis := range []Vec3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}} {
		m, err := RotationAxis(axis, .6)
		mustEdit(t, err)
		want := RotationXYZ(axis.Mul(.6))
		for i := range m {
			if math.Abs(m[i]-want[i]) > 1e-14 {
				t.Fatal(m, want)
			}
		}
		for _, magnitude := range []float64{1e-300, 1e300} {
			scaled, err := RotationAxis(axis.Mul(magnitude), .6)
			mustEdit(t, err)
			if scaled != m {
				t.Fatal("normalization unstable")
			}
		}
	}
	pivot := Vec3{3, 4, 5}
	normal := Vec3{0, 0, 1}
	ray := func(angle float64) Ray {
		return Ray{Origin: pivot.Add(Vec3{2 * math.Cos(angle), 2 * math.Sin(angle), 4}), Direction: Vec3{0, 0, -1}}
	}
	const start = .37
	solver, err := NewRotationDrag(ray(start), pivot, normal)
	mustEdit(t, err)
	previous := 0.0
	for i := 0; i <= 240; i++ {
		want := float64(i) * math.Pi / 20
		angle, err := solver.Angle(ray(start+want), previous)
		mustEdit(t, err)
		if math.Abs(angle-want) > 1e-12 {
			t.Fatal(i, angle, want)
		}
		previous = angle
	}
	for i := 240; i >= -160; i-- {
		want := float64(i) * math.Pi / 20
		angle, err := solver.Angle(ray(start+want), previous)
		mustEdit(t, err)
		if math.Abs(angle-want) > 1e-12 {
			t.Fatal(i, angle, want)
		}
		previous = angle
	}
	if n := testing.AllocsPerRun(100, func() { _, _ = solver.Angle(ray(start+.7), 0); _, _ = RotationAxis(normal, .7) }); n != 0 {
		t.Fatal("allocations", n)
	}
}
func TestRotationRejectsInvalidAndAmbiguousInput(t *testing.T) {
	for _, axis := range []Vec3{{}, {math.NaN(), 0, 0}, {0, math.Inf(1), 0}} {
		if _, err := RotationAxis(axis, 0); err == nil {
			t.Fatal("invalid axis")
		}
	}
	for _, angle := range []float64{math.NaN(), math.Inf(1)} {
		if _, err := RotationAxis(Vec3{1, 0, 0}, angle); err == nil {
			t.Fatal("invalid angle")
		}
	}
	for _, ray := range []Ray{{Origin: Vec3{1, 0, 2}, Direction: Vec3{1, 0, 0}}, {Origin: Vec3{0, 0, 2}, Direction: Vec3{0, 0, -1}}, {Origin: Vec3{1, 0, 2}, Direction: Vec3{0, 0, 1}}} {
		if _, err := NewRotationDrag(ray, Vec3{}, Vec3{0, 0, 1}); err == nil {
			t.Fatal("ambiguous ray")
		}
	}
	d, err := NewRotationDrag(Ray{Origin: Vec3{1, 0, 2}, Direction: Vec3{0, 0, -1}}, Vec3{}, Vec3{0, 0, 1})
	mustEdit(t, err)
	if _, err := d.Angle(Ray{}, 0); err == nil {
		t.Fatal("invalid ray")
	}
	if _, err := d.Angle(Ray{Direction: Vec3{0, 0, -1}}, math.NaN()); err == nil {
		t.Fatal("invalid previous")
	}
	var zero RotationDrag
	if _, err := zero.Angle(Ray{}, 0); err == nil {
		t.Fatal("zero solver")
	}
}

func FuzzRotationAxisAndUnwrap(f *testing.F) {
	f.Add(1.0, 2.0, 3.0, .7, 0.0)
	f.Add(1e-300, 0.0, 0.0, math.Pi, 3*math.Pi)
	f.Add(1e300, -1e300, 0.0, -.4, -12*math.Pi)
	f.Fuzz(func(t *testing.T, x, y, z, angle, previous float64) {
		m, err := RotationAxis(Vec3{x, y, z}, angle)
		if err != nil {
			return
		}
		if !m.affine() {
			t.Fatal("accepted nonfinite rotation")
		}
		for a := 0; a < 3; a++ {
			for b := 0; b < 3; b++ {
				dot := 0.0
				for r := 0; r < 3; r++ {
					dot += m[a*4+r] * m[b*4+r]
				}
				want := 0.0
				if a == b {
					want = 1
				}
				if math.Abs(dot-want) > 1e-12 {
					t.Fatal("rotation lost orthonormality")
				}
			}
		}
		solver, err := NewRotationDrag(Ray{Origin: Vec3{1, 0, 4}, Direction: Vec3{0, 0, -1}}, Vec3{}, Vec3{0, 0, 1})
		if err != nil {
			t.Fatal(err)
		}
		result, err := solver.Angle(Ray{Origin: Vec3{math.Cos(angle), math.Sin(angle), 4}, Direction: Vec3{0, 0, -1}}, previous)
		if err == nil {
			if !finite(result) {
				t.Fatal("nonfinite angle")
			}
			if math.Abs(previous) < 1e8 && math.Abs(result-previous) > math.Pi+1e-7 {
				t.Fatal("unwrapping discontinuity")
			}
		}
	})
}
