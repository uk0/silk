package paint

import "image"

// PainterOperator queries a native painter without extending Painter's interface.
// Unknown/custom painters return ok=false and remain on their ordinary path.
func PainterOperator(p Painter) (op Operator, ok bool) {
	c, ok := p.(*cairoPainter)
	if !ok || c == nil || c.cairo == nil {
		return 0, false
	}
	return Operator(c.cairo.Operator()), true
}

// DeviceClipRectangle reports an exact rectangular physical-pixel clip, never
// merely its bounds. Unsupported backends (including pure Go) return false.
func DeviceClipRectangle(p Painter) (image.Rectangle, bool) {
	c, ok := p.(*cairoPainter)
	if !ok || c == nil || c.cairo == nil {
		return image.Rectangle{}, false
	}
	provider, ok := any(c.cairo).(interface {
		DeviceClipRectangle() (image.Rectangle, bool)
	})
	if !ok {
		return image.Rectangle{}, false
	}
	return provider.DeviceClipRectangle()
}

// ClearDeviceClip cuts a transparent hole only when the current clip is exact.
// It preserves the current path and all painter state; callers own presentation
// of the missing pixels. This is not a general-purpose replacement for Paint.
func ClearDeviceClip(p Painter) bool {
	if _, ok := DeviceClipRectangle(p); !ok {
		return false
	}
	p.Save()
	p.SetOperator(OpClear)
	p.Paint()
	p.Restore()
	return true
}
