package geom

import (
	"math"
)

// 3x2的平面变换矩阵, 兼容cairo库
type Mat3x2 struct {
	Xx, Yx, Xy, Yy, X0, Y0 float64
}

func (m *Mat3x2) Init(xx, yx, xy, yy, x0, y0 float64) {
	m.Xx, m.Yx = xx, yx
	m.Xy, m.Yy = xy, yy
	m.X0, m.Y0 = x0, y0
}

func (m *Mat3x2) InitIdentity() {
	m.Xx, m.Yx = 1, 0
	m.Xy, m.Yy = 0, 1
	m.X0, m.Y0 = 0, 0
}

func (m *Mat3x2) InitTranslate(tx, ty float64) {
	m.Xx, m.Yx = 1, 0
	m.Xy, m.Yy = 0, 1
	m.X0, m.Y0 = tx, ty
}

func (m *Mat3x2) InitScale(sx, sy float64) {
	m.Xx, m.Yx = sx, 0
	m.Xy, m.Yy = 0, sy
	m.X0, m.Y0 = 0, 0
}

func (m *Mat3x2) InitRotate(radians float64) {
	c := math.Cos(radians)
	s := math.Sin(radians)
	m.Xx, m.Yx = c, s
	m.Xy, m.Yy = -s, c
	m.X0, m.Y0 = 0, 0
}

func multiplyMat3x2(result, a, b *Mat3x2) {
	result.Xx = a.Xx*b.Xx + a.Yx*b.Xy //+ 0*b.X0
	result.Xy = a.Xy*b.Xx + a.Yy*b.Xy //+ 0*b.X0
	result.X0 = a.X0*b.Xx + a.Y0*b.Xy + b.X0
	result.Yx = a.Xx*b.Yx + a.Yx*b.Yy //+ 0*b.Y0
	result.Yy = a.Xy*b.Yx + a.Yy*b.Yy //+ 0*b.Y0
	result.Y0 = a.X0*b.Yx + a.Y0*b.Yy + b.Y0
}

func (a *Mat3x2) MultiplyWidth(b *Mat3x2) {
	var c Mat3x2
	multiplyMat3x2(&c, a, b)
	*a = c
}

func (a *Mat3x2) Multiply(b *Mat3x2) (c Mat3x2) {
	multiplyMat3x2(&c, a, b)
	return
}

func (m *Mat3x2) Translate(tx, ty float64) {
	var a Mat3x2
	a.InitTranslate(tx, ty)
	m.MultiplyWidth(&a)
}

func (m *Mat3x2) Scale(sx, sy float64) {
	var a Mat3x2
	a.InitScale(sx, sy)
	m.MultiplyWidth(&a)
}

func (m *Mat3x2) Rotate(radians float64) {
	var a Mat3x2
	a.InitRotate(radians)
	m.MultiplyWidth(&a)
}

func (m *Mat3x2) Invert() bool {
	x := m.Det()

	if x == 0 {
		m.InitIdentity()
		return false
	}

	invdet := 1.0 / x

	var b Mat3x2
	b.Xx = m.Yy * invdet
	b.Xy = -m.Xy * invdet
	b.X0 = (m.Xy*m.Y0 - m.Yy*m.X0) * invdet
	b.Yx = -m.Yx * invdet
	b.Yy = m.Xx * invdet
	b.Y0 = (m.X0*m.Yx - m.Y0*m.Xx) * invdet
	*m = b
	return true
}

func (m *Mat3x2) Det() (x float64) {
	x = m.Xx*m.Yy - m.Xy*m.Yx
	return
}

func (m *Mat3x2) Transform(x, y float64) (x1, y1 float64) {
	x1 = m.Xx*x + m.Xy*y + m.X0
	y1 = m.Yx*x + m.Yy*y + m.Y0
	return
}

func (m *Mat3x2) TransformVec(x, y float64) (x1, y1 float64) {
	x1 = m.Xx*x + m.Xy*y
	y1 = m.Yx*x + m.Yy*y
	return
}
