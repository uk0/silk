package scene3d_test

import (
	"fmt"
	"github.com/uk0/silk/scene3d"
)

func ExampleNewFrustum() {
	vp, err := scene3d.DefaultCamera().ViewProjection(16.0 / 9)
	if err != nil {
		panic(err)
	}
	frustum := scene3d.NewFrustum(vp)
	box := scene3d.Box()
	fmt.Println(frustum.Intersects(box, scene3d.Identity()))
	fmt.Println(frustum.Intersects(box, scene3d.Translation(scene3d.Vec3{X: 10000})))
	// Output:
	// true
	// false
}
