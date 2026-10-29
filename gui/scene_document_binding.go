package gui

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/scene3d"
)

// SceneBindings returns copied, inert metadata loaded from a scene document or
// installed by SetSceneBindings. Deserialization never subscribes to live tags.
func (v *SceneView) SceneBindings() []scene3d.Binding {
	return append([]scene3d.Binding(nil), v.bindingSpecs...)
}

// CaptureSceneDocument preserves authored transforms for SetSceneBindings
// sessions instead of live tag samples. Legacy BindNodePosition/Visibility are
// not document metadata; without a compiled session current values are captured.
// Call on the UI thread; SaveDocument may then run on a worker. No file
// I/O happens here. Model import warnings survive the portable geometry export.
func (v *SceneView) CaptureSceneDocument() (*scene3d.Document, error) {
	if v.closed {
		return nil, async.ErrClosed
	}
	var d *scene3d.Document
	var err error
	if v.bindingProgram != nil {
		d, err = v.bindingProgram.CaptureDocument()
	} else {
		d, err = scene3d.CaptureDocument(v.scene, v.bindingSpecs)
	}
	if err != nil {
		return nil, err
	}
	d.Warnings = append([]string(nil), v.warnings...)
	return d, nil
}

// SetSceneBindings validates every destination/tag before replacing ALL current
// view-owned bindings. No tags are created. Bindings remain explicit: call this
// after LoadModel's completion, passing SceneBindings() for a saved document.
// All tag sources share one coalesced UI delivery, with at most 256 samples per
// turn so a wide tag burst yields back to the event loop. Bad quality/non-scalar
// input holds the last good visual value. This is not a lossless alarm log.
// Cancellation is idempotent and safe from any goroutine; an already-started
// update may finish. Reparent, SetScene, successful reload and Close cancel it.
func (v *SceneView) SetSceneBindings(db *core.TagDB, specs []scene3d.Binding) (func(), error) {
	if v.closed {
		return nil, async.ErrClosed
	}
	if db == nil {
		return nil, ErrUIInvalid
	}
	program, err := scene3d.NewBindingSet(v.scene, specs)
	if err != nil {
		return nil, err
	}
	names := program.Tags()
	tags := make([]*core.Tag, len(names))
	for i, name := range names {
		var ok bool
		tags[i], ok = db.Get(name)
		if !ok {
			return nil, fmt.Errorf("%w: unknown scene tag %q", scene3d.ErrBinding, name)
		}
	}
	// Registry resolution must also succeed before restoring any live output.
	before := v.scene.Revision()
	if v.bindingProgram != nil {
		program, err = v.bindingProgram.Reconfigure(specs)
		if err != nil {
			return nil, err
		}
	}
	scene := v.scene
	type delivery struct {
		index int
		value core.Value
	}
	var mu sync.Mutex
	var stopped atomic.Bool
	values := make([]core.Value, len(tags))
	queued := make([]bool, len(tags))
	ring := make([]int, len(tags))
	head, tail, count := 0, 0, 0
	var batch [256]delivery // UI-owned reusable storage, separate from publishers.
	var latest *async.Latest[struct{}]
	latest, err = NewLatestUI(func(struct{}) {
		if stopped.Load() || v.closed || v.scene != scene {
			return
		}
		mu.Lock()
		n := min(count, len(batch))
		for i := 0; i < n; i++ {
			index := ring[head]
			head = (head + 1) % len(ring)
			count--
			batch[i] = delivery{index, values[index]}
			values[index] = core.Value{}
			queued[index] = false
		}
		mu.Unlock()
		defer clear(batch[:n])
		changed := false
		for _, item := range batch[:n] {
			if stopped.Load() || v.closed || v.scene != scene {
				return
			}
			if item.value.Quality != core.QualityGood {
				continue
			}
			update, e := program.Apply(names[item.index], item.value.Raw)
			if e != nil {
				if !errors.Is(e, scene3d.ErrSample) {
					v.fail(e)
				}
				continue
			}
			changed = changed || update
		}
		if changed && !stopped.Load() && !v.closed && v.scene == scene {
			v.RefreshScene()
		}
		mu.Lock()
		more := count > 0
		mu.Unlock()
		if more {
			latest.Publish(struct{}{})
		}
	})
	if err != nil {
		return nil, err
	}
	v.unbindAll()
	v.bindingProgram = program
	v.bindingSpecs = program.Bindings()
	if v.scene.Revision() != before {
		v.RefreshScene()
	}
	cancels := make([]core.CancelFunc, 0, len(tags))
	for i, tag := range tags {
		cancels = append(cancels, tag.Subscribe(func(value core.Value) {
			mu.Lock()
			if stopped.Load() {
				mu.Unlock()
				return
			}
			values[i] = value
			if !queued[i] {
				queued[i] = true
				ring[tail] = i
				tail = (tail + 1) % len(ring)
				count++
			}
			mu.Unlock()
			latest.Publish(struct{}{})
		}))
	}
	return v.ownBinding(func() {
		stopped.Store(true)
		latest.Close()
		for _, cancel := range cancels {
			cancel()
		}
		mu.Lock()
		clear(values)
		clear(queued)
		count = 0
		mu.Unlock()
	}), nil
}
