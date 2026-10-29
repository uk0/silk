package gui

import (
	"context"
	"errors"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/scene3d"
)

// SceneDevicePlacementMode changes only authoring placement, never live data.
// Surface modes require a prepared index and never fall back to the ground.
type SceneDevicePlacementMode uint8

const (
	SceneDeviceGround SceneDevicePlacementMode = iota
	SceneDeviceSurfaceUpright
	SceneDeviceSurfaceAligned
)

var ErrSceneSurfaceMiss = errors.New("gui: no surface at placement point")

type sceneDeviceSurface struct {
	index      *scene3d.RayIndex
	cancel     context.CancelFunc
	generation uint64
	busy       bool
	err        error
}

func (e *SceneDocumentEditor) DevicePlacementMode() SceneDevicePlacementMode {
	return e.devicePlacementMode
}
func (e *SceneDocumentEditor) SetDevicePlacementMode(mode SceneDevicePlacementMode) error {
	if e.closed {
		return async.ErrClosed
	}
	if mode > SceneDeviceSurfaceAligned {
		return ErrUIInvalid
	}
	e.ensureDeviceLibrary()
	if mode == e.devicePlacementMode {
		return nil
	}
	e.devicePlacementMode = mode
	e.library.placementGeneration++ // retire drags without invalidating model imports
	e.preview.OnDragLeave()
	e.library.placement.SetActiveIndex(int(mode))
	e.library.retireSurface(mode == SceneDeviceGround)
	if mode != SceneDeviceGround && e.libraryVisible {
		return e.PrepareDeviceSurface(context.Background())
	}
	return nil
}

// PrepareDeviceSurface captures the current authored snapshot on the UI thread
// and schedules one cancellable worker build/refit. Called on mode/show/edit,
// never on hover, draw or idle. A late completion cannot replace a newer result.
// Caller cancellation is reported; hide/close/mode changes retire the request.
func (e *SceneDocumentEditor) PrepareDeviceSurface(ctx context.Context) error {
	if e.closed {
		return async.ErrClosed
	}
	if ctx == nil {
		return ErrUIInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.devicePlacementMode == SceneDeviceGround || !e.libraryVisible {
		return ErrUIInvalid
	}
	e.ensureDeviceLibrary()
	l := e.library
	snapshot, err := e.scene.Snapshot()
	if err != nil {
		return err
	}
	l.retireSurface(false)
	if l.surface.index != nil && l.surface.index.Snapshot() == snapshot {
		l.surfaceStatus()
		return nil
	}
	generation := l.surface.generation
	old := l.surface.index
	child, cancel := context.WithCancel(ctx)
	l.surface.cancel, l.surface.busy = cancel, true
	l.surfaceStatus()
	_, err = RunAsync(child, nil, func(ctx context.Context) (*scene3d.RayIndex, error) {
		return old.WithSnapshot(ctx, snapshot)
	}, func(index *scene3d.RayIndex, err error) {
		defer cancel()
		if canceled := child.Err(); canceled != nil {
			err = canceled
		}
		l.finishSurface(generation, snapshot, index, err)
	})
	if err != nil {
		cancel()
		l.finishSurface(generation, snapshot, nil, err)
	}
	return err
}
func (l *sceneDeviceLibrary) retireSurface(release bool) {
	l.surface.generation++
	if l.surface.cancel != nil {
		l.surface.cancel()
		l.surface.cancel = nil
	}
	l.surface.busy, l.surface.err = false, nil
	if release {
		l.surface.index = nil
	}
	l.surfaceStatus()
}
func (l *sceneDeviceLibrary) finishSurface(generation uint64, snapshot *scene3d.Snapshot, index *scene3d.RayIndex, err error) {
	e := l.editor
	if e.closed || !e.libraryVisible || e.devicePlacementMode == SceneDeviceGround || generation != l.surface.generation || !l.surface.busy {
		return
	}
	l.surface.busy, l.surface.cancel = false, nil
	current, currentErr := e.scene.Snapshot()
	if currentErr != nil {
		err = currentErr
	} else if current != snapshot || (err == nil && (index == nil || index.Snapshot() != snapshot)) {
		err = ErrSceneSurfaceStale
	}
	l.surface.err = err
	if err == nil {
		l.surface.index = index
	}
	l.surfaceStatus()
}
func (e *SceneDocumentEditor) DeviceSurfaceReady() bool {
	if e.closed || !e.libraryVisible || e.devicePlacementMode == SceneDeviceGround || e.library == nil {
		return false
	}
	s := &e.library.surface
	if s.busy || s.err != nil || s.index == nil {
		return false
	}
	snapshot, err := e.scene.Snapshot()
	return err == nil && snapshot == s.index.Snapshot()
}
func (e *SceneDocumentEditor) IsDeviceSurfacePreparing() bool {
	return !e.closed && e.library != nil && e.library.surface.busy
}
func (e *SceneDocumentEditor) DeviceSurfaceError() error {
	if e.closed {
		return async.ErrClosed
	}
	if e.library == nil {
		return nil
	}
	return e.library.surface.err
}
func (l *sceneDeviceLibrary) surfaceStatus() {
	if l.prepareSurface == nil {
		return
	}
	e := l.editor
	text := "Prepare surfaces"
	if l.surface.busy {
		text = "Preparing surfaces..."
	} else if l.surface.err != nil {
		text = "Retry surfaces"
	} else if e.DeviceSurfaceReady() {
		text = "Surfaces ready"
	}
	l.prepareSurface.SetText(i18n.T(text))
	l.prepareSurface.SetEnabled(!e.closed && e.devicePlacementMode != SceneDeviceGround && !l.surface.busy)
	tip := i18n.T(text) + "\n" + i18n.T("Surface placement has no ground fallback; Shift grid snapping applies only to ground mode.")
	if l.surface.err != nil {
		tip += "\n" + l.surface.err.Error()
	}
	SetToolTip(l.prepareSurface, tip)
	if l.list != nil {
		if e.devicePlacementMode == SceneDeviceGround {
			SetToolTip(l.list, i18n.T("Drag onto the ground plane; Shift snaps to 1 unit. No live points are started."))
		} else {
			SetToolTip(l.list, tip)
		}
	}
	if l.insert != nil {
		l.describe()
	}
}

type sceneDevicePlacement struct {
	point, normal scene3d.Vec3
	transform     scene3d.Mat4
	node          string
}

func (v *SceneView) devicePlacementAt(x, y float64, template *scene3d.Template) (sceneDevicePlacement, error) {
	e := v.translation.editor
	if e.devicePlacementMode == SceneDeviceGround {
		p, ok := v.deviceDropPoint(x, y)
		if !ok {
			return sceneDevicePlacement{}, ErrSceneSurfaceMiss
		}
		return sceneDevicePlacement{point: p, transform: scene3d.Translation(p)}, nil
	}
	if !e.DeviceSurfaceReady() {
		return sceneDevicePlacement{}, ErrSceneSurfaceStale
	}
	// Only this input query owns a timer; no periodic renderer/polling is started.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	hit, found, _, err := v.SurfaceAt(ctx, e.library.surface.index, x, y, scene3d.RayQuery{})
	if err != nil {
		return sceneDevicePlacement{}, err
	}
	if !found {
		return sceneDevicePlacement{}, ErrSceneSurfaceMiss
	}
	anchor, err := template.BaseAnchor()
	if err != nil {
		return sceneDevicePlacement{}, err
	}
	matrix, err := scene3d.SurfacePlacementTransform(hit.Position, hit.Normal, scene3d.SurfacePlacementOptions{Anchor: anchor, AlignNormal: e.devicePlacementMode == SceneDeviceSurfaceAligned})
	if err != nil {
		return sceneDevicePlacement{}, err
	}
	return sceneDevicePlacement{point: hit.Position, normal: hit.Normal, transform: matrix, node: hit.NodeID}, nil
}
