package scene3d

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/url"
	"strings"
)

func splitGLB(raw []byte) (document, bin []byte, unknown bool, err error) {
	if len(raw) < 4 || string(raw[:4]) != "glTF" {
		return raw, nil, false, nil
	}
	fail := func() ([]byte, []byte, bool, error) {
		return nil, nil, false, fmt.Errorf("%w: GLB framing", ErrGeometry)
	}
	if len(raw) < 12 || binary.LittleEndian.Uint32(raw[4:8]) != 2 || uint64(binary.LittleEndian.Uint32(raw[8:12])) != uint64(len(raw)) {
		return fail()
	}
	count := 0
	for pos := 12; pos < len(raw); {
		if len(raw)-pos < 8 {
			return fail()
		}
		size := uint64(binary.LittleEndian.Uint32(raw[pos:]))
		kind := binary.LittleEndian.Uint32(raw[pos+4:])
		pos += 8
		if size%4 != 0 || size > uint64(len(raw)-pos) {
			return fail()
		}
		chunk := raw[pos : pos+int(size)]
		pos += int(size)
		if count == 0 && kind != 0x4e4f534a {
			return fail()
		}
		switch kind {
		case 0x4e4f534a:
			if count != 0 {
				return fail()
			}
			document = chunk
		case 0x004e4942:
			if count != 1 || bin != nil {
				return fail()
			}
			bin = chunk
		default:
			unknown = true
		}
		count++
	}
	if len(document) == 0 {
		return fail()
	}
	return document, bin, unknown, nil
}
func loadGLTFBuffers(ctx context.Context, d *gltfDocument, bin []byte, fsys fs.FS, remaining int64) ([][]byte, error) {
	out := make([][]byte, len(d.Buffers))
	for i, b := range d.Buffers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if b.ByteLength < 1 {
			return nil, ErrGeometry
		}
		if b.URI == "" {
			if i != 0 || bin == nil || b.ByteLength > int64(len(bin)) || int64(len(bin))-b.ByteLength > 3 {
				return nil, fmt.Errorf("%w: GLB buffer length", ErrGeometry)
			}
			for _, v := range bin[b.ByteLength:] {
				if v != 0 {
					return nil, ErrGeometry
				}
			}
			out[i] = bin[:b.ByteLength]
			continue
		}
		if b.ByteLength > remaining {
			return nil, ErrLimit
		}
		if strings.HasPrefix(b.URI, "data:") {
			prefix, encoded, ok := strings.Cut(b.URI, ",")
			if !ok || (prefix != "data:application/octet-stream;base64" && prefix != "data:application/gltf-buffer;base64") {
				return nil, fmt.Errorf("%w: buffer data URI", ErrUnsupported)
			}
			var err error
			// Stream decoding under the declared byte budget, including invalid data.
			out[i], err = io.ReadAll(&budgetReader{ctx: ctx, r: base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded)), max: b.ByteLength})
			if err != nil {
				return nil, fmt.Errorf("glTF data buffer: %w", err)
			}
		} else {
			name, err := gltfBufferPath(b.URI)
			if err != nil {
				return nil, err
			}
			if fsys == nil {
				return nil, fmt.Errorf("%w: external buffer requires Buffers filesystem", ErrUnsupported)
			}
			out[i], err = readGLTFBuffer(ctx, fsys, name, b.ByteLength)
			if err != nil {
				return nil, err
			}
		}
		if int64(len(out[i])) != b.ByteLength {
			return nil, fmt.Errorf("%w: buffer byteLength", ErrGeometry)
		}
		remaining -= int64(len(out[i]))
	}
	return out, nil
}
func gltfBufferPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || strings.ContainsAny(uri, "?#") {
		return "", fmt.Errorf("%w: nonlocal buffer URI", ErrUnsupported)
	}
	name := u.Path // url.Parse decodes percent escapes once; fs paths are never URL-decoded again.
	if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\:\x00") {
		return "", fmt.Errorf("%w: unsafe buffer path", ErrUnsupported)
	}
	return name, nil
}
func readGLTFBuffer(ctx context.Context, fsys fs.FS, name string, size int64) ([]byte, error) {
	info, err := fs.Stat(fsys, name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: nonregular buffer", ErrUnsupported)
	}
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: nonregular buffer", ErrUnsupported)
	}
	if info.Size() != size {
		return nil, fmt.Errorf("%w: external buffer length", ErrGeometry)
	}
	return io.ReadAll(&budgetReader{ctx: ctx, r: f, max: size})
}

type gltfView struct {
	Buffer                             *int
	ByteOffset, ByteLength, ByteStride int
	Target                             int
}
type gltfAccessor struct {
	BufferView                       *int
	ByteOffset, ComponentType, Count int
	Type                             string
	Normalized                       bool
	Sparse                           *struct {
		Count   int
		Indices struct {
			BufferView                *int
			ByteOffset, ComponentType int
		}
		Values struct {
			BufferView *int
			ByteOffset int
		}
	}
}
type gltfDecoder struct {
	ctx     context.Context
	doc     *gltfDocument
	buffers [][]byte
}

func (g *gltfDecoder) validateViews() error {
	for _, v := range g.doc.BufferViews {
		if v.Buffer == nil || *v.Buffer < 0 || *v.Buffer >= len(g.buffers) {
			return ErrGeometry
		}
		b := g.buffers[*v.Buffer]
		if v.ByteOffset < 0 || v.ByteLength < 1 || v.ByteOffset > len(b) || v.ByteLength > len(b)-v.ByteOffset {
			return ErrGeometry
		}
		if v.ByteStride != 0 && (v.ByteStride < 4 || v.ByteStride > 252 || v.ByteStride%4 != 0) {
			return ErrGeometry
		}
		if v.Target != 0 && v.Target != 34962 && v.Target != 34963 {
			return ErrGeometry
		}
	}
	return nil
}
func gltfComponentSize(t int) int {
	switch t {
	case 5120, 5121:
		return 1
	case 5122, 5123:
		return 2
	case 5125, 5126:
		return 4
	}
	return 0
}
func gltfComponents(t string) int {
	switch t {
	case "SCALAR":
		return 1
	case "VEC2":
		return 2
	case "VEC3":
		return 3
	case "VEC4":
		return 4
	}
	return 0
}
func (g *gltfDecoder) accessor(i int) (gltfAccessor, error) {
	if i < 0 || i >= len(g.doc.Accessors) {
		return gltfAccessor{}, ErrGeometry
	}
	a := g.doc.Accessors[i]
	if a.Count < 1 || a.ByteOffset < 0 || gltfComponentSize(a.ComponentType) == 0 || gltfComponents(a.Type) == 0 {
		return a, ErrGeometry
	}
	if a.Normalized && (a.ComponentType == 5126 || a.ComponentType == 5125) {
		return a, ErrGeometry
	}
	return a, nil
}

// rangeData validates before multiplying: hostile offsets/counts cannot wrap or allocate.
func (g *gltfDecoder) rangeData(view *int, offset, count, element, component int, vertex, tight bool) ([]byte, int, error) {
	if view == nil || *view < 0 || *view >= len(g.doc.BufferViews) || offset < 0 || count < 1 {
		return nil, 0, ErrGeometry
	}
	v := g.doc.BufferViews[*view]
	if offset > v.ByteLength {
		return nil, 0, ErrGeometry
	}
	stride := v.ByteStride
	if stride == 0 {
		stride = element
	}
	if tight && v.ByteStride != 0 {
		return nil, 0, ErrGeometry
	}
	if offset%component != 0 || (v.ByteOffset+offset)%component != 0 || stride < element || stride%component != 0 {
		return nil, 0, ErrGeometry
	}
	if vertex && (offset%4 != 0 || stride%4 != 0) {
		return nil, 0, ErrGeometry
	}
	if offset > v.ByteLength || element > v.ByteLength-offset || count-1 > (v.ByteLength-offset-element)/stride {
		return nil, 0, ErrGeometry
	}
	start := v.ByteOffset + offset
	return g.buffers[*v.Buffer][start : start+(count-1)*stride+element], stride, nil
}
func (g *gltfDecoder) elements(a gltfAccessor, vertex bool, apply func(int, []byte) error) error {
	component := gltfComponentSize(a.ComponentType)
	element := component * gltfComponents(a.Type)
	if a.BufferView != nil {
		b, stride, err := g.rangeData(a.BufferView, a.ByteOffset, a.Count, element, component, vertex, !vertex)
		if err != nil {
			return err
		}
		for i := 0; i < a.Count; i++ {
			if i%1024 == 0 {
				if e := g.ctx.Err(); e != nil {
					return e
				}
			}
			if e := apply(i, b[i*stride:i*stride+element]); e != nil {
				return e
			}
		}
	} else if a.ByteOffset != 0 {
		return ErrGeometry
	}
	if s := a.Sparse; s != nil {
		if s.Count < 1 || s.Count > a.Count || (s.Indices.ComponentType != 5121 && s.Indices.ComponentType != 5123 && s.Indices.ComponentType != 5125) {
			return ErrGeometry
		}
		width := gltfComponentSize(s.Indices.ComponentType)
		indices, _, err := g.rangeData(s.Indices.BufferView, s.Indices.ByteOffset, s.Count, width, width, false, true)
		if err != nil {
			return err
		}
		values, _, err := g.rangeData(s.Values.BufferView, s.Values.ByteOffset, s.Count, element, component, false, true)
		if err != nil {
			return err
		}
		previous := -1
		for i := 0; i < s.Count; i++ {
			if i%1024 == 0 {
				if e := g.ctx.Err(); e != nil {
					return e
				}
			}
			index := uint64(gltfUint(indices[i*width:], width))
			if index >= uint64(a.Count) || int(index) <= previous {
				return ErrGeometry
			}
			previous = int(index)
			if err = apply(int(index), values[i*element:i*element+element]); err != nil {
				return err
			}
		}
	}
	return nil
}
func gltfUint(b []byte, width int) uint32 {
	switch width {
	case 1:
		return uint32(b[0])
	case 2:
		return uint32(binary.LittleEndian.Uint16(b))
	default:
		return binary.LittleEndian.Uint32(b)
	}
}
func (g *gltfDecoder) primitive(p gltfPrimitive, maxVertices, maxTriangles int) (*Mesh, error) {
	// Attribute aliases can reference the same large accessor repeatedly.
	// Cap them before walking data so validation work is bounded as well as bytes.
	if len(p.Attributes) > 32 {
		return nil, fmt.Errorf("%w: primitive attribute count", ErrLimit)
	}
	if err := g.ctx.Err(); err != nil {
		return nil, err
	}
	if len(p.Targets) > 0 {
		return nil, fmt.Errorf("%w: morph targets", ErrUnsupported)
	}
	mode := 4
	if p.Mode != nil {
		mode = *p.Mode
	}
	if mode < 4 || mode > 6 {
		return nil, fmt.Errorf("%w: primitive mode %d", ErrUnsupported, mode)
	}
	position, ok := p.Attributes["POSITION"]
	if !ok {
		return nil, ErrGeometry
	}
	a, err := g.accessor(position)
	if err != nil {
		return nil, err
	}
	if a.ComponentType != 5126 || a.Type != "VEC3" || a.Normalized {
		return nil, fmt.Errorf("%w: POSITION must be FLOAT VEC3", ErrUnsupported)
	}
	if a.Count > maxVertices {
		return nil, ErrLimit
	}
	verts := make([]Vec3, a.Count)
	err = g.elements(a, true, func(i int, b []byte) error {
		value := func(j int) float64 { return float64(math.Float32frombits(binary.LittleEndian.Uint32(b[j:]))) }
		v := Vec3{value(0), value(4), value(8)}
		if !v.valid() || v.Length() > 1e12 {
			return ErrGeometry
		}
		verts[i] = v
		return nil
	})
	if err != nil {
		return nil, err
	}
	for name, index := range p.Attributes {
		if name == "POSITION" {
			continue
		}
		attr, e := g.accessor(index)
		if e != nil {
			return nil, e
		}
		if attr.Count != a.Count {
			return nil, ErrGeometry
		}
		if strings.HasPrefix(name, "JOINTS_") || strings.HasPrefix(name, "WEIGHTS_") {
			return nil, fmt.Errorf("%w: skin attributes", ErrUnsupported)
		}
		// Validate storage even for attributes not consumed by the flat-shaded backend.
		if e = g.elements(attr, true, func(_ int, _ []byte) error { return nil }); e != nil {
			return nil, e
		}
	}
	count := a.Count
	var ia gltfAccessor
	if p.Indices != nil {
		ia, err = g.accessor(*p.Indices)
		if err != nil {
			return nil, err
		}
		if ia.Type != "SCALAR" || ia.Normalized || (ia.ComponentType != 5121 && ia.ComponentType != 5123 && ia.ComponentType != 5125) {
			return nil, ErrGeometry
		}
		count = ia.Count
	}
	triangleCount := count / 3
	if mode == 4 {
		if count%3 != 0 {
			return nil, ErrGeometry
		}
	} else {
		triangleCount = count - 2
	}
	if triangleCount < 1 {
		return nil, ErrGeometry
	}
	if triangleCount > maxTriangles {
		return nil, ErrLimit
	}
	indices := make([]uint32, count)
	if p.Indices == nil {
		for i := range indices {
			indices[i] = uint32(i)
		}
	} else {
		width := gltfComponentSize(ia.ComponentType)
		err = g.elements(ia, false, func(i int, b []byte) error {
			v := gltfUint(b, width)
			if uint64(v) >= uint64(len(verts)) || uint64(v) == (uint64(1)<<(width*8))-1 {
				return ErrGeometry
			}
			indices[i] = v
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if mode != 4 {
		converted := make([]uint32, 0, triangleCount*3)
		for i := 0; i < triangleCount; i++ {
			if i%1024 == 0 {
				if err = g.ctx.Err(); err != nil {
					return nil, err
				}
			}
			if mode == 6 {
				converted = append(converted, indices[0], indices[i+1], indices[i+2])
			} else if i%2 == 0 {
				converted = append(converted, indices[i], indices[i+1], indices[i+2])
			} else {
				converted = append(converted, indices[i+1], indices[i], indices[i+2])
			}
		}
		indices = converted
	}
	return NewMesh(verts, indices)
}
