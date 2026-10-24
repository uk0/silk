package gui

import "github.com/uk0/silk/geom"

// IChildViewport clips inline children in the container's local coordinates.
// Painting and pointer hit-testing use the same rectangle. The container's own
// drawing/overlay and independently owned native popup windows are unaffected.
// This keeps scroll controls outside the content from being painted or hit
// through by an offscreen child. Return a zero rectangle to hide all children.
type IChildViewport interface {
	ChildViewport() geom.Rect
}

func widgetChildViewport(w IWidget) geom.Rect {
	r := geom.Rect{Width: w.Width(), Height: w.Height()}
	if v, ok := w.(IChildViewport); ok {
		clip := v.ChildViewport()
		if !validPopupArea(clip) {
			return geom.Rect{}
		}
		r.X, r.Y, r.Width, r.Height = intersectRect(r.X, r.Y, r.Width, r.Height, clip.X, clip.Y, clip.Width, clip.Height)
	}
	return r
}
