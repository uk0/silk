package scene3d

import (
	"context"
	"image/color"
	"math"
	"sync"
)

// DeviceDefinition associates an application/library key and display name with
// an immutable template. These are not node IDs or filesystem paths.
type DeviceDefinition struct {
	ID, Name string
	Template *Template
}

var builtinDevices = sync.OnceValues(func() ([]DeviceDefinition, error) {
	var result []DeviceDefinition
	box := Box()
	cylinder, err := Cylinder(1, 2, 24)
	if err != nil {
		return nil, err
	}
	blue := color.RGBA{65, 160, 220, 255}
	metal := color.RGBA{117, 140, 160, 255}
	green := color.RGBA{50, 189, 143, 255}
	red := color.RGBA{239, 82, 92, 255}
	build := func(id, name string, nodes []Node, bindings []Binding) error {
		s := NewScene()
		for _, n := range nodes {
			if err := s.Add(n); err != nil {
				return err
			}
		}
		d, err := CaptureDocument(s, bindings)
		if err != nil {
			return err
		}
		d.Warnings = []string{"Built-in schematic geometry, not a manufacturer engineering model. Binding units must be configured for the actual device."}
		t, err := NewTemplate(context.Background(), d)
		if err != nil {
			return err
		}
		result = append(result, DeviceDefinition{id, name, t})
		return nil
	}
	tr := func(x, y, z, sx, sy, sz float64) Mat4 {
		return Translation(Vec3{x, y, z}).Mul(Scaling(Vec3{sx, sy, sz}))
	}
	err = build("pump", "Pump", []Node{
		{ID: "base", Mesh: box, Transform: tr(0, .1, 0, 1.5, .1, .75), Color: metal},
		{ID: "housing", Mesh: cylinder, Transform: tr(0, .9, 0, .75, .8, .75).Mul(RotationXYZ(Vec3{Z: math.Pi / 2})), Color: blue},
		{ID: "outlet", Mesh: cylinder, Transform: tr(.7, 1.5, 0, .2, .5, .2), Color: metal},
		{ID: "rotor", Transform: Translation(Vec3{X: -.85, Y: .9})},
		{ID: "blade", Parent: "rotor", Mesh: box, Transform: Scaling(Vec3{X: .08, Y: .52, Z: .13}), Color: green},
	}, []Binding{{Node: "rotor", Tag: "angle", Property: RotationX, Gain: 1}, {Node: "housing", Tag: "alarm", Property: AlarmColor, FalseColor: blue, TrueColor: red}})
	if err != nil {
		return nil, err
	}
	err = build("tank", "Tank", []Node{
		{ID: "base", Mesh: box, Transform: tr(0, .1, 0, 1, .1, 1), Color: metal},
		{ID: "level", Transform: Translation(Vec3{Y: .2})},
		{ID: "liquid", Parent: "level", Mesh: cylinder, Transform: tr(0, .9, 0, .72, .9, .72), Color: blue},
		{ID: "lid", Mesh: cylinder, Transform: tr(0, 2.1, 0, .85, .1, .85), Color: metal},
		{ID: "post1", Mesh: cylinder, Transform: tr(-.85, 1.1, -.85, .06, 1, .06), Color: metal},
		{ID: "post2", Mesh: cylinder, Transform: tr(.85, 1.1, -.85, .06, 1, .06), Color: metal},
		{ID: "post3", Mesh: cylinder, Transform: tr(-.85, 1.1, .85, .06, 1, .06), Color: metal},
		{ID: "post4", Mesh: cylinder, Transform: tr(.85, 1.1, .85, .06, 1, .06), Color: metal},
	}, []Binding{{Node: "level", Tag: "level", Property: ScaleY, Gain: .01}, {Node: "liquid", Tag: "alarm", Property: AlarmColor, FalseColor: blue, TrueColor: red}})
	if err != nil {
		return nil, err
	}
	err = build("conveyor", "Conveyor", []Node{
		{ID: "frame", Mesh: box, Transform: tr(0, .55, 0, 2.5, .15, .85), Color: metal},
		{ID: "belt", Mesh: box, Transform: tr(0, .73, 0, 2.4, .03, .75), Color: color.RGBA{53, 70, 87, 255}},
		{ID: "leg1", Mesh: box, Transform: tr(-2, .2, -.6, .1, .2, .1), Color: metal},
		{ID: "leg2", Mesh: box, Transform: tr(2, .2, -.6, .1, .2, .1), Color: metal},
		{ID: "leg3", Mesh: box, Transform: tr(-2, .2, .6, .1, .2, .1), Color: metal},
		{ID: "leg4", Mesh: box, Transform: tr(2, .2, .6, .1, .2, .1), Color: metal},
		{ID: "indicator", Mesh: box, Transform: tr(2, .6, .9, .12, .1, .05), Color: green},
	}, []Binding{{Node: "indicator", Tag: "running", Property: AlarmColor, FalseColor: metal, TrueColor: green}})
	if err != nil {
		return nil, err
	}
	return result, nil
})

// BuiltinDeviceTemplates returns schematic pump/tank/conveyor definitions with
// inert point declarations. The slice is caller-owned; compiled resources are
// shared across calls/owners. These are quick assembly examples, not CAD assets.
func BuiltinDeviceTemplates() ([]DeviceDefinition, error) {
	devices, err := builtinDevices()
	return append([]DeviceDefinition(nil), devices...), err
}
