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

// gridScratch holds reusable per-call buffers for GridLayout. Tracking column
// widths, row heights, and cumulative offsets without per-call allocation.
type gridScratch struct {
	colW, rowH []float64
	colX, rowY []float64
}

var gridScratchPool = sync.Pool{
	New: func() interface{} { return new(gridScratch) },
}

// acquireGridScratch returns a gridScratch with all buffers truncated to len 0
// (capacity preserved for reuse).
func acquireGridScratch() *gridScratch {
	s := gridScratchPool.Get().(*gridScratch)
	s.colW = s.colW[:0]
	s.rowH = s.rowH[:0]
	s.colX = s.colX[:0]
	s.rowY = s.rowY[:0]
	return s
}

// releaseGridScratch returns the buffer to the pool. Oversized buffers are
// dropped to avoid pinning large allocations after a one-time massive grid.
func releaseGridScratch(s *gridScratch) {
	if cap(s.colW) > 256 || cap(s.rowH) > 256 ||
		cap(s.colX) > 256 || cap(s.rowY) > 256 {
		return // don't pool oversized buffers
	}
	gridScratchPool.Put(s)
}

// growF64 resizes buf to length n, reusing existing capacity when possible.
// Returned slice is zero-filled.
func growF64(buf []float64, n int) []float64 {
	if cap(buf) >= n {
		buf = buf[:n]
		for i := range buf {
			buf[i] = 0
		}
		return buf
	}
	return make([]float64, n)
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

// eachChild walks all children (including hidden), invoking fn with index.
// Stops if fn returns false. Used where index-based access is needed.
func (this *Widget) eachChild(fn func(int, IWidget) bool) {
	head := this.child
	if head == nil {
		return
	}
	end := head.prev
	idx := 0
	for c := head; ; c = c.next {
		if !fn(idx, c.self) {
			return
		}
		idx++
		if c == end {
			return
		}
	}
}

// childCount returns the number of children without allocating a slice.
func (this *Widget) childCount() int {
	head := this.child
	if head == nil {
		return 0
	}
	end := head.prev
	n := 0
	for c := head; ; c = c.next {
		n++
		if c == end {
			return n
		}
	}
}
