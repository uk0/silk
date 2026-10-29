package scene3d

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"io/fs"
	"math"
	"strconv"
	"strings"
)

// GLTFOptions configures a bounded glTF 2.0 / GLB static-scene import.
// Buffers is optional: nil rejects external buffers. Only relative URI paths
// are accepted; callers supplying an OS filesystem should use os.Root.FS, not
// os.DirFS, if symlinks must be confined. Images are never opened.
// Limits cap aggregate unique geometry, not each primitive independently.
// MaxBytes covers the document plus external/decoded data (GLB BIN is counted once).
// MaxLineBytes is an OBJ option and has no effect on glTF.
type GLTFOptions struct {
	Limits     LoadLimits
	Buffers    fs.FS
	SceneIndex *int
}

// GLTFModel owns immutable templates. NewScene creates independent mutable
// nodes while retaining shared immutable meshes. IDs are based on source indices,
// not names (which need not be unique).
type GLTFModel struct {
	nodes    []Node
	warnings []string
}

func (m *GLTFModel) Nodes() []Node {
	if m == nil {
		return nil
	}
	return append([]Node(nil), m.nodes...)
}
func (m *GLTFModel) Warnings() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.warnings...)
}
func (m *GLTFModel) NewScene(rootID string) (*Scene, error) {
	if m == nil || rootID == "" || len(rootID) > 200 {
		return nil, ErrNode
	}
	s := NewScene()
	if err := s.Add(Node{ID: rootID, Name: "glTF model"}); err != nil {
		return nil, err
	}
	for _, n := range m.nodes {
		n.ID = rootID + "/" + n.ID
		if n.Parent == "" {
			n.Parent = rootID
		} else {
			n.Parent = rootID + "/" + n.Parent
		}
		if err := s.Add(n); err != nil {
			return nil, err
		}
	}
	if _, err := s.Snapshot(); err != nil {
		return nil, err
	}
	return s, nil
}

type gltfDocument struct {
	Asset struct {
		Version    string
		MinVersion string
	}
	Scene  *int
	Scenes []struct{ Nodes []int }
	Nodes  []gltfNode
	Meshes []struct {
		Primitives []gltfPrimitive
		Weights    []float64
	}
	Buffers []struct {
		URI        string
		ByteLength int64
	}
	BufferViews                                  []gltfView
	Accessors                                    []gltfAccessor
	Materials                                    []gltfMaterial
	ExtensionsRequired, ExtensionsUsed           []string
	Animations, Skins, Cameras, Images, Textures []json.RawMessage
}
type gltfNode struct {
	Name                                          string
	Children                                      []int
	Mesh, Skin, Camera                            *int
	Matrix, Translation, Rotation, Scale, Weights []float64
}
type gltfPrimitive struct {
	Attributes              map[string]int
	Indices, Material, Mode *int
	Targets                 []json.RawMessage
}
type gltfMaterial struct {
	PBRMetallicRoughness struct {
		BaseColorFactor                            []float64
		MetallicFactor, RoughnessFactor            *float64
		BaseColorTexture, MetallicRoughnessTexture json.RawMessage
	}
	AlphaMode                                        string
	EmissiveFactor                                   []float64
	NormalTexture, OcclusionTexture, EmissiveTexture json.RawMessage
}

// LoadGLTF accepts JSON glTF or GLB (detected by its magic), preserving static
// hierarchy, transforms, shared meshes and opaque base colors. It is not a full
// glTF validator or PBR renderer: inspect Warnings. Required extensions, skins,
// animations, morphs and non-opaque alpha modes fail rather than silently vanish.
// Cancellation is checked during reads and bounded parsing/geometry loops; a
// caller-owned blocking Reader must itself be closed/deadlined by its owner.
func LoadGLTF(ctx context.Context, r io.Reader, options GLTFOptions) (*GLTFModel, error) {
	if ctx == nil || r == nil {
		return nil, ErrGeometry
	}
	l, err := options.Limits.normalized()
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(&budgetReader{ctx: ctx, r: r, max: l.MaxBytes})
	if err != nil {
		return nil, err
	}
	document, bin, extra, err := splitGLB(raw)
	if err != nil {
		return nil, err
	}
	if err = checkGLTFJSON(ctx, document); err != nil {
		return nil, err
	}
	var d gltfDocument
	if err = json.Unmarshal(document, &d); err != nil {
		return nil, fmt.Errorf("glTF JSON: %w: %v", ErrGeometry, err)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if d.Asset.Version != "2.0" || (d.Asset.MinVersion != "" && d.Asset.MinVersion != "2.0") {
		return nil, fmt.Errorf("%w: glTF version", ErrUnsupported)
	}
	if len(d.ExtensionsRequired) > 0 {
		return nil, fmt.Errorf("%w: required extensions %v", ErrUnsupported, d.ExtensionsRequired)
	}
	if len(d.Animations) > 0 || len(d.Skins) > 0 {
		return nil, fmt.Errorf("%w: animations/skins", ErrUnsupported)
	}
	if len(d.Nodes) > MaxNodes-1 || len(d.Meshes) > MaxNodes || len(d.Accessors) > MaxNodes*8 || len(d.BufferViews) > MaxNodes*8 || len(d.Buffers) > 1024 || len(d.Scenes) > MaxNodes || len(d.Materials) > MaxNodes {
		return nil, ErrLimit
	}
	out := &GLTFModel{}
	out.warnings = append(out.warnings, "glTF preview uses flat shading, base colors and double-sided faces; PBR, textures, vertex colors and authored normals are not rendered")
	if extra {
		out.warnings = append(out.warnings, "unknown GLB chunks ignored")
	}
	if len(d.ExtensionsUsed) > 0 {
		out.warnings = append(out.warnings, "optional glTF extensions ignored")
	}
	if len(d.Cameras) > 0 {
		out.warnings = append(out.warnings, "authored cameras ignored; use the SceneView camera")
	}
	buffers, err := loadGLTFBuffers(ctx, &d, bin, options.Buffers, l.MaxBytes-int64(len(raw)))
	if err != nil {
		return nil, err
	}
	decoder := gltfDecoder{ctx: ctx, doc: &d, buffers: buffers}
	if err = decoder.validateViews(); err != nil {
		return nil, err
	}
	meshes := make([][]Node, len(d.Meshes))
	vertices, triangles := 0, 0
	for i, m := range d.Meshes {
		if len(m.Weights) > 0 {
			return nil, fmt.Errorf("%w: morph weights", ErrUnsupported)
		}
		if len(m.Primitives) == 0 {
			return nil, ErrGeometry
		}
		if len(m.Primitives) > MaxNodes {
			return nil, ErrLimit
		}
		for j, p := range m.Primitives {
			mesh, e := decoder.primitive(p, l.MaxVertices-vertices, l.MaxTriangles-triangles)
			if e != nil {
				return nil, fmt.Errorf("glTF mesh %d primitive %d: %w", i, j, e)
			}
			vertices += mesh.VertexCount()
			triangles += mesh.TriangleCount()
			c, e := gltfColor(&d, p.Material)
			if e != nil {
				return nil, e
			}
			meshes[i] = append(meshes[i], Node{Mesh: mesh, Color: c})
		}
	}
	parents, transforms, err := gltfHierarchy(ctx, &d)
	if err != nil {
		return nil, err
	}
	roots := []int{}
	if len(d.Scenes) > 0 {
		index := 0
		if d.Scene != nil {
			index = *d.Scene
		}
		if options.SceneIndex != nil {
			index = *options.SceneIndex
		}
		if index < 0 || index >= len(d.Scenes) {
			return nil, ErrNode
		}
		roots = d.Scenes[index].Nodes
		if d.Scene == nil && options.SceneIndex == nil {
			out.warnings = append(out.warnings, "no default scene; selected scene 0")
		}
	} else {
		if d.Scene != nil || options.SceneIndex != nil {
			return nil, ErrNode
		}
		for i, p := range parents {
			if p < 0 {
				roots = append(roots, i)
			}
		}
		out.warnings = append(out.warnings, "no scenes; selected all root nodes")
	}
	seen := make([]bool, len(d.Nodes))
	// Stack preserves file order without recursive descent on adversarial trees.
	stack := make([]int, 0, len(d.Nodes))
	for i := len(roots) - 1; i >= 0; i-- {
		n := roots[i]
		if n < 0 || n >= len(d.Nodes) || parents[n] >= 0 || seen[n] {
			return nil, ErrNode
		}
		seen[n] = true
		stack = append(stack, n)
	}
	for len(stack) > 0 {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		index := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n := d.Nodes[index]
		id := "node/" + strconv.Itoa(index)
		node := Node{ID: id, Name: n.Name, Transform: transforms[index]}
		if parents[index] >= 0 {
			node.Parent = "node/" + strconv.Itoa(parents[index])
		}
		needed := 1
		if n.Mesh != nil {
			needed += len(meshes[*n.Mesh])
		}
		if len(out.nodes)+needed+1 > MaxNodes {
			return nil, ErrLimit
		}
		out.nodes = append(out.nodes, node)
		if n.Mesh != nil {
			for j, p := range meshes[*n.Mesh] {
				p.ID = id + "/primitive/" + strconv.Itoa(j)
				p.Parent = id
				p.Name = n.Name
				out.nodes = append(out.nodes, p)
			}
		}
		for i := len(n.Children) - 1; i >= 0; i-- {
			stack = append(stack, n.Children[i])
		}
	}
	scene, err := out.NewScene("model")
	if err != nil {
		return nil, err
	}
	snap, err := scene.Snapshot()
	if err != nil {
		return nil, err
	}
	if _, _, err = snap.Bounds(); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Bound metadata amplification before json.Unmarshal allocates document tables.
// Also reject duplicate JSON keys: ambiguous accessor offsets are not accepted.
func checkGLTFJSON(ctx context.Context, b []byte) error {
	return checkBoundedJSON(ctx, b, 2_000_000, MaxNodes*16, 64, false)
}

// Shared metadata gate for bounded glTF and authored scene documents.
func checkBoundedJSON(ctx context.Context, b []byte, maxTokens, maxContainers, maxDepth int, foldKeys bool) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	type frame struct {
		object, key bool
		keys        map[string]bool
	}
	var stack []frame
	tokens, containers := 0, 0
	for {
		if tokens%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		t, e := dec.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return fmt.Errorf("%w: JSON: %v", ErrGeometry, e)
		}
		tokens++
		if tokens > maxTokens {
			return ErrLimit
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{', '[':
				containers++
				if containers > maxContainers || len(stack) >= maxDepth {
					return ErrLimit
				}
				if len(stack) > 0 {
					f := &stack[len(stack)-1]
					if f.object {
						f.key = true
					}
				}
				f := frame{object: delim == '{', key: delim == '{'}
				if f.object {
					f.keys = map[string]bool{}
				}
				stack = append(stack, f)
			case '}', ']':
				stack = stack[:len(stack)-1]
			}
		} else if len(stack) > 0 {
			f := &stack[len(stack)-1]
			if f.object {
				if f.key {
					key, ok := t.(string)
					if foldKeys {
						key = strings.ToLower(key)
					}
					if !ok || f.keys[key] {
						return fmt.Errorf("%w: duplicate/invalid JSON key", ErrGeometry)
					}
					f.keys[key] = true
				}
				f.key = !f.key
			}
		}
	}
	if len(stack) != 0 || tokens == 0 {
		return ErrGeometry
	}
	return nil
}

func gltfHierarchy(ctx context.Context, d *gltfDocument) ([]int, []Mat4, error) {
	parents := make([]int, len(d.Nodes))
	transforms := make([]Mat4, len(d.Nodes))
	for i := range parents {
		parents[i] = -1
	}
	for i, n := range d.Nodes {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if n.Skin != nil || len(n.Weights) > 0 {
			return nil, nil, fmt.Errorf("%w: skin/morph node", ErrUnsupported)
		}
		if len(n.Name) > 4096 {
			return nil, nil, ErrLimit
		}
		if n.Mesh != nil && (*n.Mesh < 0 || *n.Mesh >= len(d.Meshes)) {
			return nil, nil, ErrNode
		}
		if n.Camera != nil && (*n.Camera < 0 || *n.Camera >= len(d.Cameras)) {
			return nil, nil, ErrNode
		}
		m, err := gltfTransform(n)
		if err != nil {
			return nil, nil, err
		}
		transforms[i] = m
		for _, child := range n.Children {
			if child < 0 || child >= len(parents) || child == i || parents[child] >= 0 {
				return nil, nil, ErrNode
			}
			parents[child] = i
		}
	}
	queue := make([]int, 0, len(parents))
	for i, p := range parents {
		if p < 0 {
			queue = append(queue, i)
		}
	}
	for i := 0; i < len(queue); i++ {
		queue = append(queue, d.Nodes[queue[i]].Children...)
	}
	if len(queue) != len(parents) {
		return nil, nil, fmt.Errorf("%w: hierarchy cycle", ErrNode)
	}
	// Every declared scene must reference distinct actual roots, even if unselected.
	for _, s := range d.Scenes {
		seen := map[int]bool{}
		for _, i := range s.Nodes {
			if i < 0 || i >= len(parents) || parents[i] >= 0 || seen[i] {
				return nil, nil, ErrNode
			}
			seen[i] = true
		}
	}
	return parents, transforms, nil
}
func gltfTransform(n gltfNode) (Mat4, error) {
	if len(n.Matrix) > 0 {
		if len(n.Matrix) != 16 || len(n.Translation)+len(n.Rotation)+len(n.Scale) > 0 {
			return Mat4{}, ErrNode
		}
		var m Mat4
		copy(m[:], n.Matrix)
		if !m.affine() {
			return Mat4{}, ErrNode
		}
		return m, nil
	}
	t, s := Vec3{}, Vec3{1, 1, 1}
	q := [4]float64{0, 0, 0, 1}
	if len(n.Translation) > 0 {
		if len(n.Translation) != 3 {
			return Mat4{}, ErrNode
		}
		t = Vec3{n.Translation[0], n.Translation[1], n.Translation[2]}
	}
	if len(n.Scale) > 0 {
		if len(n.Scale) != 3 {
			return Mat4{}, ErrNode
		}
		s = Vec3{n.Scale[0], n.Scale[1], n.Scale[2]}
	}
	if len(n.Rotation) > 0 {
		if len(n.Rotation) != 4 {
			return Mat4{}, ErrNode
		}
		copy(q[:], n.Rotation)
	}
	x, y, z, w := q[0], q[1], q[2], q[3]
	norm := x*x + y*y + z*z + w*w
	if !finite(norm) || math.Abs(norm-1) > 1e-4 {
		return Mat4{}, ErrNode
	}
	r := Mat4{1 - 2*(y*y+z*z), 2 * (x*y + z*w), 2 * (x*z - y*w), 0, 2 * (x*y - z*w), 1 - 2*(x*x+z*z), 2 * (y*z + x*w), 0, 2 * (x*z + y*w), 2 * (y*z - x*w), 1 - 2*(x*x+y*y), 0, 0, 0, 0, 1}
	m := Translation(t).Mul(r).Mul(Scaling(s))
	if !m.affine() {
		return Mat4{}, ErrNode
	}
	return m, nil
}
func gltfColor(d *gltfDocument, index *int) (color.RGBA, error) {
	c := color.RGBA{255, 255, 255, 255}
	if index == nil {
		return c, nil
	}
	if *index < 0 || *index >= len(d.Materials) {
		return c, ErrGeometry
	}
	m := d.Materials[*index]
	if m.AlphaMode != "" && m.AlphaMode != "OPAQUE" {
		return c, fmt.Errorf("%w: material alpha mode %s", ErrUnsupported, m.AlphaMode)
	}
	p := m.PBRMetallicRoughness
	for _, f := range []*float64{p.MetallicFactor, p.RoughnessFactor} {
		if f != nil && (!finite(*f) || *f < 0 || *f > 1) {
			return c, ErrGeometry
		}
	}
	if len(p.BaseColorFactor) > 0 {
		if len(p.BaseColorFactor) != 4 {
			return c, ErrGeometry
		}
		for _, v := range p.BaseColorFactor {
			if !finite(v) || v < 0 || v > 1 {
				return c, ErrGeometry
			}
		}
		srgb := func(v float64) uint8 {
			if v <= .0031308 {
				v *= 12.92
			} else {
				v = 1.055*math.Pow(v, 1/2.4) - .055
			}
			return uint8(math.Round(v * 255))
		}
		c.R, c.G, c.B = srgb(p.BaseColorFactor[0]), srgb(p.BaseColorFactor[1]), srgb(p.BaseColorFactor[2])
	}
	return c, nil
}
