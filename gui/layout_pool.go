package gui

import "sync"

// layoutScratch holds reusable buffers used during a single Layout()/SizeHints()
// invocation. Buffers are zero-length but retain capacity across reuse, which
// avoids per-call slice allocations on the hot path.
type layoutScratch struct {
	visible []IWidget
	hints   []SizeHints
	sizes   []float64 // widths or heights, depending on container orientation
}

var layoutScratchPool = sync.Pool{
	New: func() interface{} {
		return &layoutScratch{
			visible: make([]IWidget, 0, 16),
			hints:   make([]SizeHints, 0, 16),
			sizes:   make([]float64, 0, 16),
		}
	},
}

// acquireLayoutScratch returns a scratch buffer set ready for use.
// The returned buffers all have len == 0; the caller may grow them via append.
func acquireLayoutScratch() *layoutScratch {
	return layoutScratchPool.Get().(*layoutScratch)
}

// releaseLayoutScratch resets the scratch buffers and returns them to the pool.
// The slices are reset to len == 0 to drop references but retain capacity so
// the next caller can append without immediate reallocation.
func releaseLayoutScratch(s *layoutScratch) {
	// Drop references to widgets so they can be GC'd if the user releases them.
	for i := range s.visible {
		s.visible[i] = nil
	}
	s.visible = s.visible[:0]
	s.hints = s.hints[:0]
	s.sizes = s.sizes[:0]
	layoutScratchPool.Put(s)
}

// eachVisibleChild walks the circular doubly-linked child list and invokes fn
// for every visible child without materializing a slice. The walk stops if fn
// returns false. Mirrors the traversal in Widget.Children() but skips the
// allocation entirely.
func (this *Widget) eachVisibleChild(fn func(IWidget) bool) {
	head := this.child
	if head == nil {
		return
	}
	end := head.prev
	for c := head; ; c = c.next {
		ic := c.self
		if ic.IsVisible() {
			if !fn(ic) {
				return
			}
		}
		if c == end {
			return
		}
	}
}
