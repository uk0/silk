package scene3d_test

import (
	"fmt"

	"github.com/uk0/silk/scene3d"
)

func ExampleCuller() {
	s := scene3d.NewScene()
	mesh := scene3d.Box()
	for i := 0; i < 128; i++ {
		pose := scene3d.Translation(scene3d.Vec3{X: 1000 + float64(i)*3})
		if i == 42 {
			pose = scene3d.Identity()
		}
		if err := s.Add(scene3d.Node{ID: fmt.Sprintf("pump-%d", i), Mesh: mesh, Transform: pose}); err != nil {
			panic(err)
		}
	}
	snapshot, err := s.Snapshot()
	if err != nil {
		panic(err)
	}
	var query scene3d.Culler // one owner; snapshots may be shared between owners
	defer query.Reset()
	indices, stats := query.Cull(nil, snapshot, scene3d.NewFrustum(scene3d.Identity()))
	node, _ := snapshot.Instance(indices[0])
	fmt.Println(len(indices), node.ID, stats.CulledInstances)
	// Reuse both the hierarchy and caller-owned result capacity on a new camera.
	indices, stats = query.Cull(indices, snapshot, scene3d.NewFrustum(scene3d.Identity()))
	fmt.Println(indices[0], stats.RefittedNodes)
	// Output:
	// 1 pump-42 127
	// 42 0
}
