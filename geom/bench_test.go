package geom

import "testing"

func BenchmarkMat3x2Multiply(b *testing.B) {
	var a, c Mat3x2
	a.InitTranslate(10, 20)
	c.InitScale(2, 3)
	for i := 0; i < b.N; i++ {
		a.Multiply(&c)
	}
}

func BenchmarkMat3x2Transform(b *testing.B) {
	var m Mat3x2
	m.InitTranslate(100, 200)
	m.Scale(2, 2)
	m.Rotate(0.5)
	for i := 0; i < b.N; i++ {
		m.Transform(50, 50)
	}
}

func BenchmarkMat3x2Invert(b *testing.B) {
	var m Mat3x2
	m.InitTranslate(10, 20)
	m.Scale(2, 3)
	m.Rotate(0.7)
	for i := 0; i < b.N; i++ {
		m.Invert()
	}
}

func BenchmarkRectIntersect(b *testing.B) {
	a := Rect{10, 10, 100, 80}
	c := Rect{50, 50, 120, 90}
	for i := 0; i < b.N; i++ {
		a.IntersectCopy(c)
	}
}

func BenchmarkRectContains(b *testing.B) {
	r := Rect{0, 0, 100, 100}
	for i := 0; i < b.N; i++ {
		r.Contains(50, 50)
	}
}
