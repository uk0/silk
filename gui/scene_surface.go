package gui

import (
	"context"
	"errors"
	"math"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/scene3d"
)

// ErrSceneSurfaceStale means the prepared index is not for this view's current
// snapshot. Even a different scene with the same revision must be rebuilt.
var ErrSceneSurfaceStale = errors.New("gui: scene surface index is stale")

// SurfaceAt queries a prepared geometric index at logical viewport coordinates.
// Unlike PickNode it never renders, reads GPU pixels, builds an index or selects
// a node. It clips to the current camera's near/far planes; geometric edges and
// depth ties need not agree with raster/MSAA coverage. Outside points miss.
//
// Call on the UI thread. Capture Scene().Snapshot() there, build NewRayIndex or
// WithSnapshot on a worker, then return to the UI thread for SurfaceAt. A stale
// result is rejected, not silently rebuilt during pointer input. Camera changes
// do not require a rebuild; scene changes (including color) require WithSnapshot.
// The caller owns the immutable index, its build context and resource budget.
func (v *SceneView) SurfaceAt(ctx context.Context, index *scene3d.RayIndex, x, y float64, options scene3d.RayQuery) (scene3d.RayHit, bool, scene3d.RaycastStats, error) {
	miss := scene3d.RayHit{}
	stats := scene3d.RaycastStats{}
	if v.closed {
		return miss, false, stats, async.ErrClosed
	}
	if ctx == nil {
		return miss, false, stats, ErrUIInvalid
	}
	if err := ctx.Err(); err != nil {
		return miss, false, stats, err
	}
	w, h := v.Size()
	if !finiteSceneValue(x) || !finiteSceneValue(y) || !finiteSceneValue(w) || !finiteSceneValue(h) || w <= 0 || h <= 0 || !finiteSceneValue(options.MinDistance) || options.MinDistance < 0 || !finiteSceneValue(options.MaxDistance) || options.MaxDistance < 0 || options.MaxDistance != 0 && options.MaxDistance < options.MinDistance {
		return miss, false, stats, ErrUIInvalid
	}
	if x < 0 || y < 0 || x >= w || y >= h {
		return miss, false, stats, nil
	}
	snapshot, err := v.scene.Snapshot()
	if err != nil {
		return miss, false, stats, err
	}
	if index == nil || index.Snapshot() != snapshot {
		return miss, false, stats, ErrSceneSurfaceStale
	}
	camera := v.camera()
	ray, err := camera.Ray(x, y, w, h)
	if err != nil {
		return miss, false, stats, err
	}
	// Near/Far are camera-forward depths, not radial distances at off-axis pixels.
	cosine := ray.Direction.Dot(camera.Target.Sub(camera.Eye).Unit())
	if cosine <= 0 || !finiteSceneValue(cosine) {
		return miss, false, stats, scene3d.ErrCamera
	}
	near, far := camera.Near/cosine, camera.Far/cosine
	if !finiteSceneValue(near) || !finiteSceneValue(far) {
		return miss, false, stats, scene3d.ErrCamera
	}
	options.MinDistance = math.Max(options.MinDistance, near)
	if options.MaxDistance == 0 || options.MaxDistance > far {
		options.MaxDistance = far
	}
	if options.MinDistance > options.MaxDistance {
		return miss, false, stats, nil
	}
	return index.Cast(ctx, ray, options)
}
