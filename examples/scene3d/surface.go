package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/scene3d"
)

// Explicit, cancellable background preparation. The example retains at most one
// completed index and one bounded build, and never starts work from Draw/hover.
func (f *demo) enableSurfaceInspection() {
	f.surfaceStatus = gui.NewLabel("Surface: Inspect center builds a reusable geometry index, without rendering")
	f.surfaceStatus.SetParent(f)
	b := gui.NewButton1("Inspect center", nil)
	b.SetParent(f)
	gui.SetToolTip(b, "Query the center surface: world/local position, normal and triangle. No pixel readback.")
	b.Action().BindFunc0(func() {
		if f.surfaceBusy || f.view.IsLoading() {
			return
		}
		snap, err := f.view.Scene().Snapshot()
		if err != nil {
			f.surfaceStatus.SetText(err.Error())
			return
		}
		old := f.surfaceIndex
		f.surfaceBusy = true
		b.SetEnabled(false)
		f.surfaceStatus.SetText("Surface: preparing immutable BVH on a worker...")
		finish := func(idx *scene3d.RayIndex, err error) {
			if f.ctx.Err() != nil {
				return
			}
			f.surfaceBusy = false
			b.SetEnabled(true)
			before := f.view.RenderCount()
			var hit scene3d.RayHit
			var stats scene3d.RaycastStats
			found := false
			if err == nil {
				hit, found, stats, err = f.view.SurfaceAt(f.ctx, idx, f.view.Width()/2, f.view.Height()/2, scene3d.RayQuery{})
			}
			errorText := ""
			if err != nil {
				errorText = err.Error()
				f.surfaceText = "Surface: " + errorText
			} else {
				f.surfaceIndex = idx
				if found {
					f.surfaceText = fmt.Sprintf("Surface: %s · triangle %d · XYZ (%.3f, %.3f, %.3f) · normal (%.3f, %.3f, %.3f)", hit.NodeID, hit.Triangle, hit.Position.X, hit.Position.Y, hit.Position.Z, hit.Normal.X, hit.Normal.Y, hit.Normal.Z)
				} else {
					f.surfaceText = "Surface: no geometry at viewport center"
				}
			}
			f.surfaceQueries++
			f.surfaceStatus.SetText(f.surfaceText)
			gui.SetToolTip(f.surfaceStatus, f.surfaceText)
			state := map[string]any{"query": f.surfaceQueries, "found": found, "hit": hit, "stats": stats, "index": idx.Stats(), "error": errorText, "renders_before": before, "renders_after": f.view.RenderCount(), "on_ui": gui.IsUIThread()}
			data, _ := json.Marshal(state)
			fmt.Println("SURFACE_QUERY", string(data))
		}
		_, err = gui.RunAsync(f.ctx, nil, func(ctx context.Context) (*scene3d.RayIndex, error) { return old.WithSnapshot(ctx, snap) }, finish)
		if err != nil {
			finish(nil, err)
		}
	})
	f.buttons = append(f.buttons, b)
}
