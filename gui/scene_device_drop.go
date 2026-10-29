package gui

import (
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
	"github.com/uk0/silk/scene3d"
	"math"
)

type sceneDeviceDrag struct {
	editor               *SceneDocumentEditor
	id                   string
	template             *scene3d.Template
	generation, revision uint64
	prefix               string
}

func (v *SceneView) deviceDrag(dnd IDndContext) (sceneDeviceDrag, bool) {
	if v.closed || !v.IsEnabled() || v.translation == nil || dnd == nil || dnd.PosibleActions()&DndCopy == 0 {
		return sceneDeviceDrag{}, false
	}
	e := v.translation.editor
	if e.closed || e.library == nil || !e.libraryVisible || dnd.From() != e.library.list {
		return sceneDeviceDrag{}, false
	}
	data, ok := dnd.Data("[]interface{}").([]interface{})
	if !ok || len(data) != 1 {
		return sceneDeviceDrag{}, false
	}
	payload, ok := data[0].(sceneDeviceDrag)
	if !ok || payload.editor != e || payload.generation != e.library.generation || payload.revision != e.revision {
		return sceneDeviceDrag{}, false
	}
	for _, d := range e.library.devices {
		if d.ID == payload.id && d.Template == payload.template {
			return payload, true
		}
	}
	return sceneDeviceDrag{}, false
}
func (v *SceneView) deviceDropPoint(x, y float64) (scene3d.Vec3, bool) {
	if !finiteSceneValue(x) || !finiteSceneValue(y) || x < 0 || y < 0 || x >= v.Width() || y >= v.Height() {
		return scene3d.Vec3{}, false
	}
	ray, err := v.camera().Ray(x, y, v.Width(), v.Height())
	if err != nil {
		return scene3d.Vec3{}, false
	}
	p, ok := ray.IntersectPlane(scene3d.Vec3{}, scene3d.Vec3{Y: 1})
	if !ok {
		return scene3d.Vec3{}, false
	}
	step := v.translation.editor.translationSnapStep
	if IsKeyDown(KeyShift) {
		step = 1
	}
	p = sceneDeviceSnap(p, step)
	p.Y = 0 // eliminate round-off along the exact authored ground plane
	return p, finiteSceneValue(p.X) && finiteSceneValue(p.Z)
}
func (v *SceneView) OnDragEnter(x, y float64, dnd IDndContext) {
	if _, ok := v.deviceDrag(dnd); ok {
		v.OnHide()
	}
	v.OnDragMove(x, y, dnd)
}
func (v *SceneView) OnDragMove(x, y float64, dnd IDndContext) {
	if dnd == nil {
		return
	}
	dnd.SetAction(DndIgnore)
	if _, ok := v.deviceDrag(dnd); !ok {
		v.OnDragLeave()
		return
	}
	point, ok := v.deviceDropPoint(x, y)
	if !ok {
		v.OnDragLeave()
		return
	}
	if !v.deviceDropActive || v.deviceDropPosition != point {
		v.deviceDropActive = true
		v.deviceDropPosition = point
		v.Update()
	}
	dnd.SetAction(DndCopy)
}
func (v *SceneView) OnDragLeave() {
	if v.deviceDropActive {
		v.deviceDropActive = false
		v.Update()
	}
}
func (v *SceneView) OnDrop(x, y float64, dnd IDndContext) {
	v.OnDragLeave()
	if dnd == nil {
		return
	}
	dnd.SetAction(DndIgnore)
	payload, ok := v.deviceDrag(dnd)
	if !ok {
		return
	}
	point, ok := v.deviceDropPoint(x, y)
	if !ok {
		return
	}
	e := payload.editor
	for _, d := range e.library.devices {
		if d.ID == payload.id {
			if _, err := e.insertDevice(d, point, payload.prefix); err != nil {
				e.report(err)
				return
			}
			dnd.SetAction(DndCopy)
			return
		}
	}
}
func (v *SceneView) drawDeviceDrop(g paint.Painter) {
	if !v.deviceDropActive {
		return
	}
	p, visible, err := v.camera().Project(v.deviceDropPosition, v.Width(), v.Height())
	if err != nil || !visible {
		return
	}
	g.SetPen1(paint.Color{90, 222, 233, 255}, 2)
	g.Arc(p.X, p.Y, 9, 0, 2*math.Pi)
	g.Stroke()
	g.Line(p.X-16, p.Y, p.X+16, p.Y)
	g.Line(p.X, p.Y-16, p.X, p.Y+16)
	g.Stroke()
	g.SetFont(Theme().Font)
	g.SetBrush1(paint.Color{220, 247, 250, 255})
	g.DrawText1(p.X+20, p.Y-12, i18n.T("Drop device"))
}
