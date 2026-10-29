//go:build cgo && !silk_pure_go

package glrender

import (
	"math"
	"testing"

	"github.com/uk0/silk/scene3d"
)

func TestInstancingPackingPreservesColumnMatrixColorsAndOriginalIDs(t *testing.T) {
	in := scene3d.RenderInstance{Transform: scene3d.Translation(scene3d.Vec3{X: 3, Y: -4, Z: 5}).Mul(scene3d.Scaling(scene3d.Vec3{X: -2, Y: .25, Z: 7})), Color: [4]byte{17, 203, 241, 255}}
	storage := make([]float32, 0, maxInstanceBatch*instanceFloats)
	for _, id := range []int{1, 255, 256, 257, 4095, scene3d.MaxNodes} {
		packed := appendInstance(storage[:0], in, id)
		if len(packed) != instanceFloats {
			t.Fatal("stride", len(packed))
		}
		for i, x := range in.Transform {
			if packed[i] != float32(x) {
				t.Fatal("matrix layout", i)
			}
		}
		for i := 0; i < 3; i++ {
			if packed[16+i] != float32(in.Color[i])/255 {
				t.Fatal("material", i)
			}
		}
		recovered := int(math.Round(float64(packed[20])*255)) | int(math.Round(float64(packed[21])*255))<<8 | int(math.Round(float64(packed[22])*255))<<16
		if recovered != id {
			t.Fatal("ID renumbered", id, recovered)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() { storage = appendInstance(storage[:0], in, 257) }); allocs != 0 {
		t.Fatal("staging allocated", allocs)
	}
}

func BenchmarkInstancingPack1000(b *testing.B) {
	in := scene3d.RenderInstance{Transform: scene3d.Identity(), Color: [4]byte{17, 203, 241, 255}}
	data := make([]float32, 0, maxInstanceBatch*instanceFloats)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		data = data[:0]
		for i := 0; i < 1000; i++ {
			data = appendInstance(data, in, i+1)
		}
	}
}
