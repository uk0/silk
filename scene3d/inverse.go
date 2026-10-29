package scene3d

import "math"

// InverseAffine returns the inverse of a finite affine matrix. Scaled partial
// pivoting rejects singular/ill-conditioned linear components; non-finite
// results and inverses with a linear residual above 1e-9 are rejected. No
// projective inverse or pseudoinverse is provided. It allocates no heap memory.
func (m Mat4) InverseAffine() (Mat4, error) {
	if !m.affine() {
		return Mat4{}, ErrNode
	}
	var a [3][6]float64
	var scale [3]float64
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			a[r][c] = m[c*4+r]
			scale[r] = math.Max(scale[r], math.Abs(a[r][c]))
		}
		if scale[r] == 0 {
			return Mat4{}, ErrNode
		}
		a[r][r+3] = 1
	}
	for c := 0; c < 3; c++ {
		pivot := c
		best := 0.0
		for r := c; r < 3; r++ {
			v := math.Abs(a[r][c]) / scale[r]
			if v > best {
				best = v
				pivot = r
			}
		}
		if best <= 1e-12 || !finite(best) {
			return Mat4{}, ErrNode
		}
		a[c], a[pivot] = a[pivot], a[c]
		scale[c], scale[pivot] = scale[pivot], scale[c]
		v := a[c][c]
		for k := 0; k < 6; k++ {
			a[c][k] /= v
		}
		for r := 0; r < 3; r++ {
			if r == c {
				continue
			}
			v := a[r][c]
			for k := 0; k < 6; k++ {
				a[r][k] -= v * a[c][k]
			}
		}
	}
	out := Identity()
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			out[c*4+r] = a[r][c+3]
		}
	}
	for r := 0; r < 3; r++ {
		out[12+r] = -(out[r]*m[12] + out[4+r]*m[13] + out[8+r]*m[14])
	}
	if !out.affine() {
		return Mat4{}, ErrNode
	}
	for _, residual := range []Mat4{m.Mul(out), out.Mul(m)} {
		for r := 0; r < 3; r++ {
			for c := 0; c < 3; c++ {
				want := 0.0
				if r == c {
					want = 1
				}
				if !finite(residual[c*4+r]) || math.Abs(residual[c*4+r]-want) > 1e-9 {
					return Mat4{}, ErrNode
				}
			}
		}
	}
	return out, nil
}
