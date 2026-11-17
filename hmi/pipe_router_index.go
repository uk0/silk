package hmi

import "slices"

// A point query on Y selects potential horizontal collisions; X selects vertical
// collisions. This avoids testing every remote device for every grid edge.
// Both axes own their lists; construction never retains caller-owned slices.
type pipeObstacleIndex struct {
	center      float64
	low, high   []int
	left, right *pipeObstacleIndex
}

func pipeInterval(b Rect, horizontal bool) (float64, float64) {
	if horizontal {
		return b.Y, b.Y + b.Height
	}
	return b.X, b.X + b.Width
}

func (r *PipeRouter) buildObstacleIndex(horizontal bool) *pipeObstacleIndex {
	ids := make([]int, len(r.obstacles))
	for i := range ids {
		ids[i] = i
	}
	var build func([]int) *pipeObstacleIndex
	build = func(ids []int) *pipeObstacleIndex {
		if len(ids) == 0 {
			return nil
		}
		centerOf := func(i int) float64 { lo, hi := pipeInterval(r.obstacles[i].Bounds, horizontal); return lo + (hi-lo)/2 }
		slices.SortFunc(ids, func(a, b int) int {
			a0, b0 := centerOf(a), centerOf(b)
			if a0 < b0 {
				return -1
			}
			if a0 > b0 {
				return 1
			}
			return a - b
		})
		center := centerOf(ids[len(ids)/2])
		node := &pipeObstacleIndex{center: center}
		var left, right []int
		for _, i := range ids {
			lo, hi := pipeInterval(r.obstacles[i].Bounds, horizontal)
			if hi <= center {
				left = append(left, i)
			} else if lo >= center {
				right = append(right, i)
			} else {
				node.low = append(node.low, i)
			}
		}
		node.high = slices.Clone(node.low)
		slices.SortFunc(node.low, func(a, b int) int {
			a0, _ := pipeInterval(r.obstacles[a].Bounds, horizontal)
			b0, _ := pipeInterval(r.obstacles[b].Bounds, horizontal)
			if a0 < b0 {
				return -1
			}
			if a0 > b0 {
				return 1
			}
			return a - b
		})
		slices.SortFunc(node.high, func(a, b int) int {
			_, a1 := pipeInterval(r.obstacles[a].Bounds, horizontal)
			_, b1 := pipeInterval(r.obstacles[b].Bounds, horizontal)
			if a1 > b1 {
				return -1
			}
			if a1 < b1 {
				return 1
			}
			return a - b
		})
		node.left, node.right = build(left), build(right)
		return node
	}
	return build(ids)
}

func (r *PipeRouter) spendWork() bool {
	if r.work >= r.options.WorkLimit {
		return false
	}
	r.work++
	return true
}

func (r *PipeRouter) indexClear(n *pipeObstacleIndex, value float64, a, b XY, skip int, horizontal bool) (bool, bool) {
	if n == nil {
		return true, false
	}
	if !r.spendWork() {
		return false, true
	}
	ids := n.low
	if value > n.center {
		ids = n.high
	}
	for _, i := range ids {
		if !r.spendWork() {
			return false, true
		}
		box := r.obstacles[i].Bounds
		lo, hi := pipeInterval(box, horizontal)
		if value < n.center && lo >= value || value > n.center && hi <= value {
			break
		}
		if i != skip && pipeSegmentHits(a, b, box) {
			return false, false
		}
	}
	if value < n.center {
		return r.indexClear(n.left, value, a, b, skip, horizontal)
	}
	if value > n.center {
		return r.indexClear(n.right, value, a, b, skip, horizontal)
	}
	return true, false
}
