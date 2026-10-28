package scene3d

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// LoadLimits bounds bytes, line size and mesh complexity before allocation grows.
// Zero fields select defaults; negative or excessive fields are rejected.
type LoadLimits struct {
	MaxBytes                                int64
	MaxLineBytes, MaxVertices, MaxTriangles int
}

func (l LoadLimits) normalized() (LoadLimits, error) {
	if l.MaxBytes == 0 {
		l.MaxBytes = 32 << 20
	}
	if l.MaxLineBytes == 0 {
		l.MaxLineBytes = 1 << 20
	}
	if l.MaxVertices == 0 {
		l.MaxVertices = MaxVertices
	}
	if l.MaxTriangles == 0 {
		l.MaxTriangles = MaxTriangles
	}
	if l.MaxBytes < 1 || l.MaxBytes > 256<<20 || l.MaxLineBytes < 1 || l.MaxLineBytes > 4<<20 || l.MaxVertices < 1 || l.MaxVertices > MaxVertices || l.MaxTriangles < 1 || l.MaxTriangles > MaxTriangles {
		return l, ErrLimit
	}
	return l, nil
}

type OBJModel struct {
	Mesh     *Mesh
	Warnings []string
}
type budgetReader struct {
	ctx    context.Context
	r      io.Reader
	n, max int64
}

func (b *budgetReader) Read(p []byte) (int, error) {
	if err := b.ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(p)) > b.max-b.n+1 {
		p = p[:b.max-b.n+1]
	}
	n, err := b.r.Read(p)
	b.n += int64(n)
	if b.n > b.max {
		return n, ErrLimit
	}
	return n, err
}

// LoadOBJ loads bounded triangle/convex-quad geometry, including negative and
// v/vt/vn indices. Normals, UVs, materials and grouping are validated/reported but
// not rendered by the initial flat-shaded geometry path. MTL files are NEVER
// opened implicitly. Concave, non-planar and larger polygons fail explicitly.
// Cancellation is checked between reads/records; it cannot interrupt a blocked
// arbitrary io.Reader (the caller must close or deadline that reader).
func LoadOBJ(ctx context.Context, r io.Reader, limits LoadLimits) (*OBJModel, error) {
	if ctx == nil || r == nil {
		return nil, fmt.Errorf("%w: nil context or reader", ErrGeometry)
	}
	l, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	b := &budgetReader{ctx: ctx, r: r, max: l.MaxBytes}
	scan := bufio.NewScanner(b)
	scan.Buffer(make([]byte, min(8192, l.MaxLineBytes)), l.MaxLineBytes)
	var verts []Vec3
	var indices []uint32
	nv, nt, line := 0, 0, 0
	out := &OBJModel{}
	warned := map[string]bool{}
	warn := func(s string) {
		if !warned[s] {
			warned[s] = true
			out.Warnings = append(out.Warnings, s)
		}
	}
	fail := func(e error) (*OBJModel, error) { return nil, fmt.Errorf("OBJ line %d: %w", line, e) }
	number := func(s string) (float64, error) {
		v, e := strconv.ParseFloat(s, 64)
		if e != nil || !finite(v) {
			return 0, ErrGeometry
		}
		return v, nil
	}
	index := func(s string, count int) (int, error) {
		v, e := strconv.ParseInt(s, 10, 32)
		if e != nil || v == 0 {
			return 0, ErrGeometry
		}
		if v < 0 {
			v += int64(count)
		} else {
			v--
		}
		if v < 0 || v >= int64(count) {
			return 0, ErrGeometry
		}
		return int(v), nil
	}
	for scan.Scan() {
		line++
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		text := strings.SplitN(scan.Text(), "#", 2)[0]
		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "v":
			if len(verts) >= l.MaxVertices {
				return fail(ErrLimit)
			}
			if len(fields) != 4 && len(fields) != 5 {
				return fail(ErrUnsupported)
			}
			x, e := number(fields[1])
			if e != nil {
				return fail(e)
			}
			y, e := number(fields[2])
			if e != nil {
				return fail(e)
			}
			z, e := number(fields[3])
			if e != nil {
				return fail(e)
			}
			v := Vec3{x, y, z}
			if len(fields) == 5 {
				w, e := number(fields[4])
				if e != nil || w == 0 {
					return fail(ErrGeometry)
				}
				v = v.Mul(1 / w)
			}
			if !v.valid() || v.Length() > 1e12 {
				return fail(ErrGeometry)
			}
			verts = append(verts, v)
		case "vn", "vt":
			if (fields[0] == "vn" && len(fields) != 4) || (fields[0] == "vt" && (len(fields) < 2 || len(fields) > 4)) {
				return fail(ErrGeometry)
			}
			for _, s := range fields[1:] {
				if _, e := number(s); e != nil {
					return fail(e)
				}
			}
			if fields[0] == "vn" {
				nv++
				if nv > l.MaxVertices {
					return fail(ErrLimit)
				}
				warn("vertex normals ignored by flat-shaded renderer")
			} else {
				nt++
				if nt > l.MaxVertices {
					return fail(ErrLimit)
				}
				warn("texture coordinates ignored by untextured renderer")
			}
		case "f":
			n := len(fields) - 1
			if n < 3 || n > 4 {
				return fail(fmt.Errorf("%w: only triangles and convex planar quads", ErrUnsupported))
			}
			if len(indices)/3+n-2 > l.MaxTriangles {
				return fail(ErrLimit)
			}
			var face [4]uint32
			for i, s := range fields[1:] {
				parts := strings.Split(s, "/")
				if len(parts) > 3 {
					return fail(ErrGeometry)
				}
				j, e := index(parts[0], len(verts))
				if e != nil {
					return fail(e)
				}
				face[i] = uint32(j)
				if len(parts) >= 2 {
					if parts[1] == "" {
						if len(parts) != 3 {
							return fail(ErrGeometry)
						}
					} else {
						if _, e := index(parts[1], nt); e != nil {
							return fail(e)
						}
					}
				}
				if len(parts) == 3 {
					if _, e := index(parts[2], nv); e != nil {
						return fail(e)
					}
				}
			}
			if n == 4 {
				normal := verts[face[1]].Sub(verts[face[0]]).Cross(verts[face[2]].Sub(verts[face[0]])).Unit()
				scale := verts[face[1]].Sub(verts[face[0]]).Length() + verts[face[2]].Sub(verts[face[0]]).Length()
				if normal == (Vec3{}) || math.Abs(verts[face[3]].Sub(verts[face[0]]).Dot(normal)) > 1e-6*scale {
					return fail(ErrGeometry)
				}
				for i := 0; i < 4; i++ {
					a, b, c := verts[face[i]], verts[face[(i+1)%4]], verts[face[(i+2)%4]]
					if b.Sub(a).Cross(c.Sub(b)).Dot(normal) <= 0 {
						return fail(fmt.Errorf("%w: non-convex quad", ErrUnsupported))
					}
				}
			}
			indices = append(indices, face[0], face[1], face[2])
			if n == 4 {
				indices = append(indices, face[0], face[2], face[3])
			}
		case "o", "g":
			warn("object/group names flattened into one geometry mesh")
		case "s":
			warn("smoothing groups ignored by flat-shaded renderer")
		case "mtllib", "usemtl":
			warn("materials not loaded; no external files opened")
		default:
			return fail(fmt.Errorf("%w: directive %q", ErrUnsupported, fields[0]))
		}
	}
	if e := scan.Err(); e != nil {
		return fail(e)
	}
	if e := ctx.Err(); e != nil {
		return fail(e)
	}
	out.Mesh, err = NewMesh(verts, indices)
	if err != nil {
		return fail(err)
	}
	return out, nil
}
