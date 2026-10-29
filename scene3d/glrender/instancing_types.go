package glrender

import "errors"

// InstancingMode selects ordered, shared-mesh hardware draw instancing. A driver
// may implement OpenGL in software; this setting alone is not a hardware claim.
type InstancingMode uint8

const (
	// InstancingAuto uses instancing when the actual context supports it, otherwise
	// retains ordinary per-instance GPU draws. It never changes the render backend.
	InstancingAuto InstancingMode = iota
	InstancingDisabled
	// InstancingRequired rejects unavailable capability instead of silently falling
	// back. Useful for explicit application requirements and acceptance tests.
	InstancingRequired
)

type Options struct{ Instancing InstancingMode }

// Capabilities describes features enabled in this renderer, not device identity.
type Capabilities struct{ InstancedDrawing bool }

var ErrOptions = errors.New("scene3d: invalid OpenGL renderer options")

func (o Options) valid() bool { return o.Instancing <= InstancingRequired }
