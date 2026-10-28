package gui

import (
	"math"
	"sync"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/scene3d"
)

// BindNodePosition binds one local-space position axis (0=X, 1=Y, 2=Z).
// It preserves the other transform components, coalesces bursts on the UI queue,
// and holds the last good position for bad/uncertain/non-numeric samples.
// Returned cancellation is safe from any goroutine. SetScene, reparent and Close
// cancel all bindings owned by this view. Rotation/scale bindings are not yet
// part of this initial API.
func (v *SceneView) BindNodePosition(tag *core.Tag, id string, axis int, scale, offset float64) (func(), error) {
	if tag == nil || axis < 0 || axis > 2 || !finiteSceneValue(scale) || !finiteSceneValue(offset) {
		return nil, ErrUIInvalid
	}
	if v.closed {
		return nil, scene3d.ErrNode
	}
	if _, ok := v.scene.Node(id); !ok {
		return nil, scene3d.ErrNode
	}
	scene := v.scene
	cancel := BindTagUI(tag, func(sample core.Value) {
		if v.closed || v.scene != scene || sample.Quality != core.QualityGood {
			return
		}
		var n float64
		switch x := sample.Raw.(type) {
		case float64:
			n = x
		case int64:
			n = float64(x)
		case int:
			n = float64(x)
		default:
			return
		}
		n = n*scale + offset
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return
		}
		node, ok := scene.Node(id)
		if !ok {
			return
		}
		m := node.Transform
		m[12+axis] = n
		if err := v.SetNodeTransform(id, m); err != nil {
			v.fail(err)
		}
	})
	return v.ownBinding(cancel), nil
}
func (v *SceneView) BindNodeVisibility(tag *core.Tag, id string) (func(), error) {
	if tag == nil {
		return nil, ErrUIInvalid
	}
	if v.closed {
		return nil, scene3d.ErrNode
	}
	if _, ok := v.scene.Node(id); !ok {
		return nil, scene3d.ErrNode
	}
	scene := v.scene
	cancel := BindTagUI(tag, func(s core.Value) {
		if v.closed || v.scene != scene || s.Quality != core.QualityGood {
			return
		}
		visible, ok := s.Raw.(bool)
		if !ok {
			return
		}
		if err := v.SetNodeVisible(id, visible); err != nil {
			v.fail(err)
		}
	})
	return v.ownBinding(cancel), nil
}
func (v *SceneView) ownBinding(cancel func()) func() {
	v.bindingMu.Lock()
	if v.bindings == nil {
		v.bindings = make(map[uint64]func())
	}
	v.nextBinding++
	key := v.nextBinding
	var once sync.Once
	stop := func() {
		once.Do(func() { cancel(); v.bindingMu.Lock(); delete(v.bindings, key); v.bindingMu.Unlock() })
	}
	v.bindings[key] = stop
	v.bindingMu.Unlock()
	return stop
}
func (v *SceneView) unbindAll() {
	v.bindingMu.Lock()
	bindings := v.bindings
	v.bindings = nil
	v.bindingMu.Unlock()
	for _, stop := range bindings {
		stop()
	}
}
