package scene3d

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func gltfFixture() (map[string]any, []byte) {
	data := make([]byte, 42)
	for i, v := range []float32{-1, -1, 0, 1, -1, 0, 0, 1, 0} {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(v))
	}
	for i, v := range []uint16{0, 1, 2} {
		binary.LittleEndian.PutUint16(data[36+i*2:], v)
	}
	d := map[string]any{
		"asset": map[string]any{"version": "2.0"}, "scene": 0,
		"scenes":      []any{map[string]any{"nodes": []int{0}}},
		"nodes":       []any{map[string]any{"name": "设备", "children": []int{1, 2}, "translation": []float64{2, 0, 0}}, map[string]any{"mesh": 0}, map[string]any{"mesh": 0, "translation": []float64{0, 3, 0}}},
		"meshes":      []any{map[string]any{"primitives": []any{map[string]any{"attributes": map[string]any{"POSITION": 0}, "indices": 1, "material": 0}}}},
		"materials":   []any{map[string]any{"pbrMetallicRoughness": map[string]any{"baseColorFactor": []float64{.25, 0, 1, .2}}}},
		"buffers":     []any{map[string]any{"byteLength": len(data), "uri": "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(data)}},
		"bufferViews": []any{map[string]any{"buffer": 0, "byteLength": 36}, map[string]any{"buffer": 0, "byteOffset": 36, "byteLength": 6}},
		"accessors":   []any{map[string]any{"bufferView": 0, "componentType": 5126, "count": 3, "type": "VEC3"}, map[string]any{"bufferView": 1, "componentType": 5123, "count": 3, "type": "SCALAR"}},
	}
	return d, data
}
func gltfJSON(d map[string]any) []byte {
	b, e := json.Marshal(d)
	if e != nil {
		panic(e)
	}
	return b
}
func gltfEntry(d map[string]any, key string, index int) map[string]any {
	return d[key].([]any)[index].(map[string]any)
}
func gltfPrimitiveEntry(d map[string]any) map[string]any {
	return gltfEntry(d, "meshes", 0)["primitives"].([]any)[0].(map[string]any)
}
func gltfBinary(d map[string]any, bin []byte) []byte {
	delete(gltfEntry(d, "buffers", 0), "uri")
	j := gltfJSON(d)
	for len(j)%4 != 0 {
		j = append(j, ' ')
	}
	bin = append([]byte(nil), bin...)
	for len(bin)%4 != 0 {
		bin = append(bin, 0)
	}
	out := make([]byte, 12)
	copy(out, "glTF")
	binary.LittleEndian.PutUint32(out[4:], 2)
	add := func(kind uint32, b []byte) {
		header := make([]byte, 8)
		binary.LittleEndian.PutUint32(header, uint32(len(b)))
		binary.LittleEndian.PutUint32(header[4:], kind)
		out = append(out, header...)
		out = append(out, b...)
	}
	add(0x4e4f534a, j)
	add(0x004e4942, bin)
	binary.LittleEndian.PutUint32(out[8:], uint32(len(out)))
	return out
}
func TestGLTFHierarchySharedMeshesAndGLBParity(t *testing.T) {
	for _, binaryFormat := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "glb"}[binaryFormat], func(t *testing.T) {
			d, bin := gltfFixture()
			raw := gltfJSON(d)
			if binaryFormat {
				raw = gltfBinary(d, bin)
			}
			model, e := LoadGLTF(context.Background(), bytes.NewReader(raw), GLTFOptions{})
			if e != nil {
				t.Fatal(e)
			}
			nodes := model.Nodes()
			if len(nodes) != 5 {
				t.Fatal(len(nodes))
			}
			if nodes[2].Mesh != nodes[4].Mesh {
				t.Fatal("shared mesh duplicated")
			}
			if nodes[0].Name != "设备" {
				t.Fatal("lost name")
			}
			if nodes[2].Color != (color.RGBA{137, 0, 255, 255}) {
				t.Fatal("linear base color / opaque alpha", nodes[2].Color)
			}
			nodes[0].ID = "corrupt"
			if model.Nodes()[0].ID != "node/0" {
				t.Fatal("mutable templates")
			}
			warnings := model.Warnings()
			warnings[0] = "corrupt"
			if model.Warnings()[0] == "corrupt" {
				t.Fatal("mutable warnings")
			}
			s, e := model.NewScene("device")
			if e != nil {
				t.Fatal(e)
			}
			snap, e := s.Snapshot()
			if e != nil {
				t.Fatal(e)
			}
			lo, hi, e := snap.Bounds()
			if e != nil || lo != (Vec3{1, -1, 0}) || hi != (Vec3{3, 4, 0}) {
				t.Fatal(lo, hi, e)
			}
			another, _ := model.NewScene("other")
			_ = s.SetTransform("device", Translation(Vec3{X: 20}))
			second, _ := another.Snapshot()
			otherLo, _, _ := second.Bounds()
			if otherLo != lo {
				t.Fatal("new scene shares mutable nodes")
			}
			// Compare actual pixels/IDs with an independently assembled reference scene.
			manual := NewScene()
			mesh, _ := NewMesh([]Vec3{{-1, -1, 0}, {1, -1, 0}, {0, 1, 0}}, []uint32{0, 1, 2})
			for i, pos := range []Vec3{{2, 0, 0}, {2, 3, 0}} {
				id := []string{"device/node/1/primitive/0", "device/node/2/primitive/0"}[i]
				if e := manual.Add(Node{ID: id, Mesh: mesh, Transform: Translation(pos), Color: color.RGBA{137, 0, 255, 255}}); e != nil {
					t.Fatal(e)
				}
			}
			ref, _ := manual.Snapshot()
			var a, b Renderer
			cam := DefaultCamera()
			cam.Target = Vec3{2, 1.5, 0}
			cam.Eye = Vec3{2, 1.5, 10}
			fa, e := a.Render(snap, cam, 160, 160, color.RGBA{})
			if e != nil {
				t.Fatal(e)
			}
			fb, e := b.Render(ref, cam, 160, 160, color.RGBA{})
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(fa.Image.Pix, fb.Image.Pix) {
				t.Fatal("hierarchical render differs")
			}
			picked := false
			for y := 0; y < 160; y++ {
				for x := 0; x < 160; x++ {
					if fa.Pick(x, y) != fb.Pick(x, y) {
						t.Fatal("ID mismatch")
					}
					picked = picked || fa.Pick(x, y) != ""
				}
			}
			if !picked {
				t.Fatal("no actual mesh pixels")
			}
		})
	}
}
func TestGLTFAccessorLayouts(t *testing.T) {
	for _, width := range []int{1, 2, 4} {
		t.Run("index"+string(rune('0'+width)), func(t *testing.T) {
			d, data := gltfFixture()
			data = data[:36]
			for _, v := range []uint32{0, 1, 2} {
				var b [4]byte
				binary.LittleEndian.PutUint32(b[:], v)
				data = append(data, b[:width]...)
			}
			gltfEntry(d, "buffers", 0)["uri"] = "data:application/gltf-buffer;base64," + base64.StdEncoding.EncodeToString(data)
			gltfEntry(d, "buffers", 0)["byteLength"] = len(data)
			gltfEntry(d, "bufferViews", 1)["byteLength"] = 3 * width
			gltfEntry(d, "accessors", 1)["componentType"] = map[int]int{1: 5121, 2: 5123, 4: 5125}[width]
			if _, e := LoadGLTF(context.Background(), bytes.NewReader(gltfJSON(d)), GLTFOptions{}); e != nil {
				t.Fatal(e)
			}
		})
	}
	t.Run("interleaved", func(t *testing.T) {
		d, data := gltfFixture()
		var b []byte
		for i := 0; i < 3; i++ {
			b = append(b, data[i*12:(i+1)*12]...)
			b = append(b, 0, 0, 0, 0)
		}
		b = append(b, data[36:]...)
		gltfEntry(d, "buffers", 0)["uri"] = "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(b)
		gltfEntry(d, "buffers", 0)["byteLength"] = len(b)
		gltfEntry(d, "bufferViews", 0)["byteLength"] = 48
		gltfEntry(d, "bufferViews", 0)["byteStride"] = 16
		gltfEntry(d, "bufferViews", 1)["byteOffset"] = 48
		m, e := LoadGLTF(context.Background(), bytes.NewReader(gltfJSON(d)), GLTFOptions{})
		if e != nil {
			t.Fatal(e)
		}
		v, _ := m.Nodes()[2].Mesh.Vertex(2)
		if v != (Vec3{0, 1, 0}) {
			t.Fatal(v)
		}
	})
	t.Run("sparse", func(t *testing.T) {
		d, data := gltfFixture()
		data = append(data, 0, 0, 0, 1, 2, 0) // indices at 44; position values at 0
		a := gltfEntry(d, "accessors", 0)
		delete(a, "bufferView")
		a["sparse"] = map[string]any{"count": 3, "indices": map[string]any{"bufferView": 2, "componentType": 5121}, "values": map[string]any{"bufferView": 0}}
		d["bufferViews"] = append(d["bufferViews"].([]any), map[string]any{"buffer": 0, "byteOffset": 44, "byteLength": 3})
		gltfEntry(d, "buffers", 0)["uri"] = "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(data)
		gltfEntry(d, "buffers", 0)["byteLength"] = len(data)
		m, e := LoadGLTF(context.Background(), bytes.NewReader(gltfJSON(d)), GLTFOptions{})
		if e != nil {
			t.Fatal(e)
		}
		v, _ := m.Nodes()[2].Mesh.Vertex(0)
		if v != (Vec3{-1, -1, 0}) {
			t.Fatal(v)
		}
		data[45] = 0
		gltfEntry(d, "buffers", 0)["uri"] = "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(data)
		if _, e = LoadGLTF(context.Background(), bytes.NewReader(gltfJSON(d)), GLTFOptions{}); !errors.Is(e, ErrGeometry) {
			t.Fatal("duplicate sparse indices", e)
		}
	})
	t.Run("nonindexed strip and fan", func(t *testing.T) {
		for _, mode := range []int{4, 5, 6} {
			d, _ := gltfFixture()
			p := gltfPrimitiveEntry(d)
			delete(p, "indices")
			p["mode"] = mode
			m, e := LoadGLTF(context.Background(), bytes.NewReader(gltfJSON(d)), GLTFOptions{})
			if e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(m.Nodes()[2].Mesh.CopyIndices(nil), []uint32{0, 1, 2}) {
				t.Fatal("topology")
			}
		}
	})
}
func TestGLTFMalformedAndUnsupported(t *testing.T) {
	cases := []struct {
		name string
		edit func(map[string]any)
		want error
	}{
		{"version", func(d map[string]any) { d["asset"] = map[string]any{"version": "1.0"} }, ErrUnsupported},
		{"required extension", func(d map[string]any) { d["extensionsRequired"] = []string{"KHR_draco_mesh_compression"} }, ErrUnsupported},
		{"animation", func(d map[string]any) { d["animations"] = []any{map[string]any{}} }, ErrUnsupported},
		{"skin", func(d map[string]any) { gltfEntry(d, "nodes", 1)["skin"] = 0 }, ErrUnsupported},
		{"cycle", func(d map[string]any) { gltfEntry(d, "nodes", 1)["children"] = []int{0} }, ErrNode},
		{"multiple parent", func(d map[string]any) { gltfEntry(d, "nodes", 1)["children"] = []int{2} }, ErrNode},
		{"child range", func(d map[string]any) { gltfEntry(d, "nodes", 0)["children"] = []int{-1} }, ErrNode},
		{"duplicate root", func(d map[string]any) { gltfEntry(d, "scenes", 0)["nodes"] = []int{0, 0} }, ErrNode},
		{"root has parent", func(d map[string]any) { gltfEntry(d, "scenes", 0)["nodes"] = []int{1} }, ErrNode},
		{"matrix trs", func(d map[string]any) { m := Identity(); gltfEntry(d, "nodes", 0)["matrix"] = m }, ErrNode},
		{"quaternion", func(d map[string]any) { gltfEntry(d, "nodes", 0)["rotation"] = []int{0, 0, 0, 0} }, ErrNode},
		{"mesh range", func(d map[string]any) { gltfEntry(d, "nodes", 1)["mesh"] = 90 }, ErrNode},
		{"scene range", func(d map[string]any) { d["scene"] = 9 }, ErrNode},
		{"view missing buffer", func(d map[string]any) { delete(gltfEntry(d, "bufferViews", 0), "buffer") }, ErrGeometry},
		{"view negative", func(d map[string]any) { gltfEntry(d, "bufferViews", 0)["byteOffset"] = -1 }, ErrGeometry},
		{"view overflow", func(d map[string]any) { gltfEntry(d, "bufferViews", 0)["byteLength"] = int64(math.MaxInt64) }, ErrGeometry},
		{"stride", func(d map[string]any) { gltfEntry(d, "bufferViews", 0)["byteStride"] = 8 }, ErrGeometry},
		{"unaligned", func(d map[string]any) { gltfEntry(d, "accessors", 0)["byteOffset"] = 1 }, ErrGeometry},
		{"count", func(d map[string]any) { gltfEntry(d, "accessors", 0)["count"] = 250001 }, ErrLimit},
		{"truncated indices", func(d map[string]any) { gltfEntry(d, "accessors", 1)["count"] = 6 }, ErrGeometry},
		{"index type", func(d map[string]any) { gltfEntry(d, "accessors", 1)["componentType"] = 5126 }, ErrGeometry},
		{"position missing", func(d map[string]any) { gltfPrimitiveEntry(d)["attributes"] = map[string]any{} }, ErrGeometry},
		{"lines", func(d map[string]any) { gltfPrimitiveEntry(d)["mode"] = 1 }, ErrUnsupported},
		{"morph", func(d map[string]any) { gltfPrimitiveEntry(d)["targets"] = []any{map[string]any{}} }, ErrUnsupported},
		{"blend", func(d map[string]any) { gltfEntry(d, "materials", 0)["alphaMode"] = "BLEND" }, ErrUnsupported},
		{"buffer range", func(d map[string]any) { gltfEntry(d, "bufferViews", 0)["buffer"] = 2 }, ErrGeometry},
		{"no glb bin", func(d map[string]any) { delete(gltfEntry(d, "buffers", 0), "uri") }, ErrGeometry},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, _ := gltfFixture()
			c.edit(d)
			m, e := LoadGLTF(context.Background(), bytes.NewReader(gltfJSON(d)), GLTFOptions{})
			if m != nil || !errors.Is(e, c.want) {
				t.Fatal(m, e, c.want)
			}
		})
	}
	d, data := gltfFixture()
	binary.LittleEndian.PutUint32(data, math.Float32bits(float32(math.NaN())))
	gltfEntry(d, "buffers", 0)["uri"] = "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(data)
	if _, e := LoadGLTF(context.Background(), bytes.NewReader(gltfJSON(d)), GLTFOptions{}); !errors.Is(e, ErrGeometry) {
		t.Fatal("binary NaN", e)
	}
	for _, raw := range []string{`{"asset":{"version":"2.0","version":"1.0"}}`, `{} {}`, strings.Repeat("[", 65) + strings.Repeat("]", 65), `{"nodes":[`} {
		if _, e := LoadGLTF(context.Background(), strings.NewReader(raw), GLTFOptions{}); e == nil {
			t.Fatal("invalid JSON accepted", raw)
		}
	}
}
func TestGLTFExternalBuffersBudgetsAndFileBoundary(t *testing.T) {
	d, bin := gltfFixture()
	buffer := gltfEntry(d, "buffers", 0)
	buffer["uri"] = "geometry%20data.bin"
	raw := gltfJSON(d)
	fsys := fstest.MapFS{"geometry data.bin": &fstest.MapFile{Data: bin}}
	if _, e := LoadGLTF(context.Background(), bytes.NewReader(raw), GLTFOptions{}); !errors.Is(e, ErrUnsupported) {
		t.Fatal(e)
	}
	if _, e := LoadGLTF(context.Background(), bytes.NewReader(raw), GLTFOptions{Buffers: fsys}); e != nil {
		t.Fatal(e)
	}
	for _, uri := range []string{"../escape.bin", "%2e%2e/escape.bin", "/absolute", "C:/file", "file:///tmp/b", "https://example.invalid/b", "//host/file", "a\\b", "%5c%5chost", "b?x", "b#x", "%00", "."} {
		buffer["uri"] = uri
		if _, e := LoadGLTF(context.Background(), bytes.NewReader(gltfJSON(d)), GLTFOptions{Buffers: fsys}); !errors.Is(e, ErrUnsupported) {
			t.Fatal(uri, e)
		}
	}
	buffer["uri"] = "geometry%20data.bin"
	for _, limit := range []LoadLimits{{MaxBytes: int64(len(raw) - 1)}, {MaxBytes: int64(len(raw) + len(bin) - 1)}, {MaxVertices: 2}, {MaxTriangles: -1}} {
		if _, e := LoadGLTF(context.Background(), bytes.NewReader(raw), GLTFOptions{Limits: limit, Buffers: fsys}); !errors.Is(e, ErrLimit) {
			t.Fatal(limit, e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := LoadGLTF(ctx, bytes.NewReader(raw), GLTFOptions{}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	dir := t.TempDir()
	if e := os.WriteFile(filepath.Join(dir, "model.gltf"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "geometry data.bin"), bin, 0600); e != nil {
		t.Fatal(e)
	}
	loaded, e := LoadModelFile(context.Background(), filepath.Join(dir, "model.gltf"), LoadLimits{})
	if e != nil || loaded.Scene == nil {
		t.Fatal(e)
	}
	outside := filepath.Join(t.TempDir(), "outside.bin")
	if e = os.WriteFile(outside, bin, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(outside, filepath.Join(dir, "escape.bin")); e != nil {
		t.Log("symlink boundary unavailable on this account:", e)
		return
	}
	buffer["uri"] = "escape.bin"
	if e = os.WriteFile(filepath.Join(dir, "model.gltf"), gltfJSON(d), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = LoadModelFile(context.Background(), filepath.Join(dir, "model.gltf"), LoadLimits{}); e == nil {
		t.Fatal("followed escaped symlink")
	}
}
func TestGLBTruncationAndFraming(t *testing.T) {
	d, bin := gltfFixture()
	raw := gltfBinary(d, bin)
	for i := 0; i < len(raw); i++ {
		if m, e := LoadGLTF(context.Background(), bytes.NewReader(raw[:i]), GLTFOptions{}); e == nil || m != nil {
			t.Fatal("accepted truncation", i)
		}
	}
	for _, offset := range []int{4, 8, 12, 16} {
		bad := append([]byte(nil), raw...)
		binary.LittleEndian.PutUint32(bad[offset:], 0xffffffff)
		if _, e := LoadGLTF(context.Background(), bytes.NewReader(bad), GLTFOptions{}); e == nil {
			t.Fatal(offset)
		}
	}
	bad := append([]byte(nil), raw...)
	bad[len(bad)-1] = 3
	if _, e := LoadGLTF(context.Background(), bytes.NewReader(bad), GLTFOptions{}); e == nil {
		t.Fatal("nonzero padding")
	}
}
func TestGLTFTransformsAndSceneSelection(t *testing.T) {
	n := gltfNode{Translation: []float64{1, 2, 3}, Rotation: []float64{0, 0, math.Sqrt(.5), math.Sqrt(.5)}, Scale: []float64{-2, 3, 4}}
	m, e := gltfTransform(n)
	if e != nil {
		t.Fatal(e)
	}
	got := m.Point(Vec3{1, 0, 0})
	if got.Sub(Vec3{1, 0, 3}).Length() > 1e-10 {
		t.Fatal(got)
	}
	n = gltfNode{Matrix: append([]float64(nil), m[:]...)}
	other, e := gltfTransform(n)
	if e != nil || m != other {
		t.Fatal(other, e)
	}
	d, _ := gltfFixture()
	gltfEntry(d, "nodes", 0)["children"] = []int{1}
	d["scenes"] = append(d["scenes"].([]any), map[string]any{"nodes": []int{2}})
	index := 1
	model, e := LoadGLTF(context.Background(), bytes.NewReader(gltfJSON(d)), GLTFOptions{SceneIndex: &index})
	if e != nil {
		t.Fatal(e)
	}
	if len(model.Nodes()) != 2 || model.Nodes()[0].ID != "node/2" {
		t.Fatal(model.Nodes())
	}
	// Only the root binding changes; imported child local transforms stay intact.
	s, _ := model.NewScene("device")
	_ = s.SetTransform("device", Translation(Vec3{X: 20}))
	snap, _ := s.Snapshot()
	lo, hi, e := snap.Bounds()
	if e != nil || lo.X != 19 || hi.Y != 4 {
		t.Fatal(lo, hi, e)
	}
}
func TestSnapshotBoundsEmptyHiddenAndNonfinite(t *testing.T) {
	if _, _, e := (*Snapshot)(nil).Bounds(); !errors.Is(e, ErrGeometry) {
		t.Fatal(e)
	}
	s := NewScene()
	_ = s.Add(Node{ID: "a", Mesh: Box(), Transform: Translation(Vec3{1, 2, 3}).Mul(RotationXYZ(Vec3{Z: math.Pi / 2})).Mul(Scaling(Vec3{2, 3, 4}))})
	snap, _ := s.Snapshot()
	lo, hi, e := snap.Bounds()
	if e != nil || lo.Sub(Vec3{-2, 0, -1}).Length() > 1e-10 || hi.Sub(Vec3{4, 4, 7}).Length() > 1e-10 {
		t.Fatal(lo, hi, e)
	}
	_ = s.SetVisible("a", false)
	snap, _ = s.Snapshot()
	if _, _, e = snap.Bounds(); !errors.Is(e, ErrGeometry) {
		t.Fatal(e)
	}
	_ = s.SetVisible("a", true)
	_ = s.SetTransform("a", Scaling(Vec3{math.MaxFloat64, math.MaxFloat64, math.MaxFloat64}))
	snap, _ = s.Snapshot()
	if _, _, e = snap.Bounds(); !errors.Is(e, ErrGeometry) {
		t.Fatal(e)
	}
}
func FuzzLoadGLTF(f *testing.F) {
	d, bin := gltfFixture()
	f.Add(gltfJSON(d))
	f.Add(gltfBinary(d, bin))
	f.Add([]byte(`{"asset":{"version":"2.0"}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			t.Skip()
		}
		m, e := LoadGLTF(context.Background(), bytes.NewReader(data), GLTFOptions{Limits: LoadLimits{MaxBytes: 64 << 10, MaxVertices: 512, MaxTriangles: 512}})
		if e == nil {
			if _, e = m.NewScene("model"); e != nil {
				t.Fatal(e)
			}
		} else if m != nil {
			t.Fatal("partial model on error")
		}
	})
}
func BenchmarkGLTFSharedHierarchy(b *testing.B) {
	d, _ := gltfFixture()
	nodes := []any{map[string]any{"children": []int{}}}
	children := make([]int, 500)
	for i := range children {
		children[i] = i + 1
		nodes = append(nodes, map[string]any{"mesh": 0, "translation": []float64{float64(i), 0, 0}})
	}
	nodes[0].(map[string]any)["children"] = children
	d["nodes"] = nodes
	raw := gltfJSON(d)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		if _, e := LoadGLTF(context.Background(), bytes.NewReader(raw), GLTFOptions{}); e != nil {
			b.Fatal(e)
		}
	}
}

func TestGLTFBoundedMetadataAndAggregateGeometry(t *testing.T) {
	aliases, _ := gltfFixture()
	attrs := map[string]any{"POSITION": 0}
	for i := 0; i < 32; i++ {
		attrs["_ALIAS"+string(rune('A'+i))] = 0
	}
	gltfPrimitiveEntry(aliases)["attributes"] = attrs
	if _, e := LoadGLTF(context.Background(), bytes.NewReader(gltfJSON(aliases)), GLTFOptions{}); !errors.Is(e, ErrLimit) {
		t.Fatal("unbounded repeated attribute work", e)
	}
	d, _ := gltfFixture()
	d["meshes"] = append(d["meshes"].([]any), gltfEntry(d, "meshes", 0))
	if _, e := LoadGLTF(context.Background(), bytes.NewReader(gltfJSON(d)), GLTFOptions{Limits: LoadLimits{MaxVertices: 5}}); !errors.Is(e, ErrLimit) {
		t.Fatal("budget applied per primitive instead of aggregate", e)
	}
	d, _ = gltfFixture()
	nodes := make([]any, MaxNodes)
	for i := range nodes {
		nodes[i] = map[string]any{}
	}
	d["nodes"] = nodes
	if _, e := LoadGLTF(context.Background(), bytes.NewReader(gltfJSON(d)), GLTFOptions{}); !errors.Is(e, ErrLimit) {
		t.Fatal("node budget", e)
	}
	// Depth limit is checked before typed decoding, including ignored extras.
	document := `{"asset":{"version":"2.0"},"extras":` + strings.Repeat("[", 65) + `0` + strings.Repeat("]", 65) + `}`
	if _, e := LoadGLTF(context.Background(), strings.NewReader(document), GLTFOptions{}); !errors.Is(e, ErrLimit) {
		t.Fatal("metadata depth", e)
	}
}
func TestGLTFStripFanWindingAndRestartIndex(t *testing.T) {
	for _, mode := range []int{5, 6} {
		d, _ := gltfFixture()
		data := make([]byte, 48)
		for i, v := range []float32{-1, -1, 0, 1, -1, 0, -1, 1, 0, 1, 1, 0} {
			binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(v))
		}
		d["buffers"] = []any{map[string]any{"byteLength": len(data), "uri": "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(data)}}
		d["bufferViews"] = []any{map[string]any{"buffer": 0, "byteLength": 48}}
		d["accessors"] = []any{map[string]any{"bufferView": 0, "componentType": 5126, "type": "VEC3", "count": 4}}
		p := gltfPrimitiveEntry(d)
		delete(p, "indices")
		p["mode"] = mode
		model, e := LoadGLTF(context.Background(), bytes.NewReader(gltfJSON(d)), GLTFOptions{})
		if e != nil {
			t.Fatal(e)
		}
		want := []uint32{0, 1, 2, 2, 1, 3}
		if mode == 6 {
			want = []uint32{0, 1, 2, 0, 2, 3}
		}
		if got := model.Nodes()[2].Mesh.CopyIndices(nil); !reflect.DeepEqual(got, want) {
			t.Fatal(mode, got, want)
		}
	}
	d, data := gltfFixture()
	data[36], data[37] = 255, 255
	gltfEntry(d, "buffers", 0)["uri"] = "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(data)
	if _, e := LoadGLTF(context.Background(), bytes.NewReader(gltfJSON(d)), GLTFOptions{}); !errors.Is(e, ErrGeometry) {
		t.Fatal("primitive restart index", e)
	}
}
func TestGLTFUnknownChunkAndExplicitFileFormats(t *testing.T) {
	d, bin := gltfFixture()
	raw := gltfBinary(d, bin)
	header := make([]byte, 8)
	binary.LittleEndian.PutUint32(header[4:], 0x12345678)
	raw = append(raw, header...)
	binary.LittleEndian.PutUint32(raw[8:], uint32(len(raw)))
	m, e := LoadGLTF(context.Background(), bytes.NewReader(raw), GLTFOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(strings.Join(m.Warnings(), ";"), "unknown GLB chunks") {
		t.Fatal(m.Warnings())
	}
	p := filepath.Join(t.TempDir(), "assembly.GLB")
	if e = os.WriteFile(p, raw, 0600); e != nil {
		t.Fatal(e)
	}
	loaded, e := LoadModelFile(context.Background(), p, LoadLimits{})
	if e != nil {
		t.Fatal(e)
	}
	snap, e := loaded.Scene.Snapshot()
	if e != nil || snap.InstanceCount() != 2 {
		t.Fatal(e)
	}
	if _, e = LoadModelFile(nil, p, LoadLimits{}); e == nil {
		t.Fatal("nil ctx")
	}
	if _, e = LoadModelFile(context.Background(), p+".exe", LoadLimits{}); !errors.Is(e, ErrUnsupported) {
		t.Fatal(e)
	}
}
