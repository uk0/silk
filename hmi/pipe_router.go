package hmi

import (
	"container/heap"
	"fmt"
	"math"
	"slices"
	"sort"
)

// Persisted Link routing modes. Empty also selects legacy orthogonal doglegs.
const (
	RoutingOrthogonal = "orthogonal"
	RoutingAvoid      = "avoid"
)

// PipeObstacle uses conservative document-space bounds, including rotated bodies
// and their labels. IDs are unique; callers omit invisible equipment.
type PipeObstacle struct {
	ID     string
	Bounds Rect
}

// PipeAnchor's Direction is north/south/east/west, pointing OUT of the owner at
// both ends. Owner may be empty; otherwise it must identify a supplied obstacle.
// Rotated clients choose the nearest cardinal outward direction explicitly.
type PipeAnchor struct {
	Point     XY
	Direction string
	Owner     string
}

// PipeRouteOptions bounds one geometry snapshot's total collision work, not just
// one pipe. Zero numeric options select defaults: clearance 8, stub 12,
// turn penalty 8, 2,048 visited states per request, 200,000 collision checks total.
// Index-node and candidate visits also count as collision work.
// A zero Bounds leaves the plane unbounded; a nonzero rectangle bounds all paths.
type PipeRouteOptions struct {
	Bounds                       Rect
	Clearance, Stub, TurnPenalty float64
	MaxVisited, WorkLimit        int
}

// PipeRouteStatus distinguishes usable geometry from an explicit failed request.
type PipeRouteStatus string

const (
	PipeRouteOK      PipeRouteStatus = "ok"
	PipeRouteBlocked PipeRouteStatus = "blocked"
	PipeRouteBudget  PipeRouteStatus = "budget"
	PipeRouteInvalid PipeRouteStatus = "invalid"
)

// PipeRouteResult owns Points. A blocked/budget result deliberately contains the
// legacy dogleg fallback, NOT a claimed collision-free path. Invalid input has
// no drawable points. Work and Visited expose deterministic costs, not timing.
type PipeRouteResult struct {
	Points        []XY
	Status        PipeRouteStatus
	Message       string
	Work, Visited int
}

// PipeRouter owns an immutable copy of geometry and a cumulative work budget.
// It is caller-owned, not concurrent-safe, and starts no workers or timers.
// Reuse it for one geometry revision; cache results separately from live colors.
type PipeRouter struct {
	obstacles      []PipeObstacle
	owners         map[string]int
	options        PipeRouteOptions
	work           int
	indexX, indexY *pipeObstacleIndex
}

const pipeCoordinateLimit = 1e6

func pipePointValid(p XY) bool {
	return finite(p.X) && finite(p.Y) && math.Abs(p.X) <= pipeCoordinateLimit && math.Abs(p.Y) <= pipeCoordinateLimit
}
func pipeRectValid(b Rect) bool {
	return b.Width > 0 && b.Height > 0 && pipePointValid(XY{b.X, b.Y}) && pipePointValid(XY{b.X + b.Width, b.Y + b.Height})
}
func pipeContains(b Rect, p XY) bool {
	return p.X >= b.X && p.Y >= b.Y && p.X <= b.X+b.Width && p.Y <= b.Y+b.Height
}

// NewPipeRouter validates and indexes at most 512 independently owned obstacles.
// Drawing order and later mutation of the input slice cannot alter the snapshot.
func NewPipeRouter(obstacles []PipeObstacle, o PipeRouteOptions) (*PipeRouter, error) {
	if o.Clearance == 0 {
		o.Clearance = 8
	}
	if o.Stub == 0 {
		o.Stub = 12
	}
	if o.TurnPenalty == 0 {
		o.TurnPenalty = 8
	}
	if o.MaxVisited == 0 {
		o.MaxVisited = 2048
	}
	if o.WorkLimit == 0 {
		o.WorkLimit = 200000
	}
	if len(obstacles) > 512 || !finite(o.Clearance) || o.Clearance < 0 || o.Clearance > 1000 || !finite(o.Stub) || o.Stub < 0 || o.Stub > 1000 || !finite(o.TurnPenalty) || o.TurnPenalty < 0 || o.TurnPenalty > 1000 || o.MaxVisited < 1 || o.MaxVisited > 32768 || o.WorkLimit < 1 || o.WorkLimit > 2000000 || o.Bounds != (Rect{}) && !pipeRectValid(o.Bounds) {
		return nil, fmt.Errorf("hmi: invalid pipe router options or obstacle budget")
	}
	r := &PipeRouter{obstacles: slices.Clone(obstacles), owners: make(map[string]int, len(obstacles)), options: o}
	// Input draw/map order must not change routes or budget accounting.
	slices.SortFunc(r.obstacles, func(a, b PipeObstacle) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	for i := range r.obstacles {
		b := &r.obstacles[i]
		if !validID(b.ID) || !pipeRectValid(b.Bounds) {
			return nil, fmt.Errorf("hmi: invalid pipe obstacle")
		}
		if _, ok := r.owners[b.ID]; ok {
			return nil, fmt.Errorf("hmi: duplicate pipe obstacle %q", b.ID)
		}
		r.owners[b.ID] = i
		// Round the envelope OUT, not just its width. Reassociating x+width+
		// clearance can otherwise put a boundary path one ULP inside the body.
		left := math.Nextafter(b.Bounds.X-o.Clearance, math.Inf(-1))
		top := math.Nextafter(b.Bounds.Y-o.Clearance, math.Inf(-1))
		right := math.Nextafter(b.Bounds.X+b.Bounds.Width+o.Clearance, math.Inf(1))
		bottom := math.Nextafter(b.Bounds.Y+b.Bounds.Height+o.Clearance, math.Inf(1))
		b.Bounds = Rect{left, top, math.Nextafter(right-left, math.Inf(1)), math.Nextafter(bottom-top, math.Inf(1))}
	}
	r.indexX = r.buildObstacleIndex(false)
	r.indexY = r.buildObstacleIndex(true)
	return r, nil
}

// WorkUsed reports cumulative indexed collision work for this geometry snapshot.
func (r *PipeRouter) WorkUsed() int {
	if r == nil {
		return 0
	}
	return r.work
}
func (r *PipeRouter) inBounds(p XY) bool {
	return r.options.Bounds == (Rect{}) || pipeContains(r.options.Bounds, p)
}

// pipeSegmentHits admits travel along the inflated boundary, never its interior.
func pipeSegmentHits(a, b XY, box Rect) bool {
	if a.Y == b.Y {
		return a.Y > box.Y && a.Y < box.Y+box.Height && min(a.X, b.X) < box.X+box.Width && max(a.X, b.X) > box.X
	}
	if a.X == b.X {
		return a.X > box.X && a.X < box.X+box.Width && min(a.Y, b.Y) < box.Y+box.Height && max(a.Y, b.Y) > box.Y
	}
	return true
}

func (r *PipeRouter) clear(a, b XY, skip int) (bool, bool) {
	if !r.inBounds(a) || !r.inBounds(b) {
		return false, false
	}
	if a.Y == b.Y {
		return r.indexClear(r.indexY, a.Y, a, b, skip, true)
	}
	if a.X == b.X {
		return r.indexClear(r.indexX, a.X, a, b, skip, false)
	}
	return false, false
}

func (r *PipeRouter) escape(a PipeAnchor) (XY, int, bool) {
	if !pipePointValid(a.Point) || !validPort(a.Direction) || !r.inBounds(a.Point) {
		return XY{}, -1, false
	}
	i := -1
	if a.Owner != "" {
		var ok bool
		i, ok = r.owners[a.Owner]
		if !ok || !pipeContains(r.obstacles[i].Bounds, a.Point) {
			return XY{}, -1, false
		}
	}
	p := a.Point
	switch a.Direction {
	case "north":
		p.Y -= r.options.Stub
		if i >= 0 {
			p.Y = min(p.Y, r.obstacles[i].Bounds.Y)
		}
	case "south":
		p.Y += r.options.Stub
		if i >= 0 {
			p.Y = max(p.Y, r.obstacles[i].Bounds.Y+r.obstacles[i].Bounds.Height)
		}
	case "west":
		p.X -= r.options.Stub
		if i >= 0 {
			p.X = min(p.X, r.obstacles[i].Bounds.X)
		}
	case "east":
		p.X += r.options.Stub
		if i >= 0 {
			p.X = max(p.X, r.obstacles[i].Bounds.X+r.obstacles[i].Bounds.Width)
		}
	}
	return p, i, pipePointValid(p)
}

// Route honors waypoint order and endpoint stubs. Successful grid sections are
// rectilinear; search is bounded and deterministic, not an optimal-routing claim.
// An impossible waypoint, obstructed stub or exhausted budget falls back visibly.
func (r *PipeRouter) Route(from, to PipeAnchor, bends []XY) PipeRouteResult {
	invalid := PipeRouteResult{Status: PipeRouteInvalid, Message: "invalid pipe anchors or waypoints"}
	if r == nil || len(bends) > 32 {
		return invalid
	}
	a, ai, ok := r.escape(from)
	if !ok {
		return invalid
	}
	b, bi, ok := r.escape(to)
	if !ok {
		return invalid
	}
	for _, p := range bends {
		if !pipePointValid(p) {
			return invalid
		}
	}
	before := r.work
	fallback := func(status PipeRouteStatus, message string, visited int) PipeRouteResult {
		return PipeRouteResult{Points: Route(from.Point, to.Point, bends), Status: status, Message: message, Work: r.work - before, Visited: visited}
	}
	for _, stub := range []struct {
		a, b XY
		skip int
	}{{from.Point, a, ai}, {b, to.Point, bi}} {
		clear, budget := r.clear(stub.a, stub.b, stub.skip)
		if budget {
			return fallback(PipeRouteBudget, "geometry work budget exhausted", 0)
		}
		if !clear {
			return fallback(PipeRouteBlocked, "port exit is obstructed or outside drawing bounds", 0)
		}
	}
	points := []XY{from.Point}
	if a != from.Point {
		points = append(points, a)
	}
	start, visited := a, 0
	for i := 0; i <= len(bends); i++ {
		end := b
		if i < len(bends) {
			end = bends[i]
		}
		path, status, n := r.section(start, end, r.options.MaxVisited-visited)
		visited += n
		if status != PipeRouteOK {
			message := "no collision-free route through the authored waypoints"
			if status == PipeRouteBudget {
				message = "route search or geometry work budget exhausted"
			}
			return fallback(status, message, visited)
		}
		points = append(points, path[1:]...)
		start = end
	}
	if points[len(points)-1] != to.Point {
		points = append(points, to.Point)
	}
	return PipeRouteResult{Points: points, Status: PipeRouteOK, Work: r.work - before, Visited: visited}
}

func simplifyPipePath(p []XY) []XY {
	result := make([]XY, 0, len(p))
	for _, a := range p {
		if len(result) > 0 && result[len(result)-1] == a {
			continue
		}
		for len(result) >= 2 {
			b, c := result[len(result)-2], result[len(result)-1]
			if !(b.X == c.X && c.X == a.X && (c.Y-b.Y)*(a.Y-c.Y) >= 0 || b.Y == c.Y && c.Y == a.Y && (c.X-b.X)*(a.X-c.X) >= 0) {
				break
			}
			result = result[:len(result)-1]
		}
		result = append(result, a)
	}
	return result
}

func (r *PipeRouter) section(a, b XY, limit int) ([]XY, PipeRouteStatus, int) {
	// Common straight/L/dogleg paths avoid constructing a search graph at all.
	candidates := [][]XY{{a, {b.X, a.Y}, b}, {a, {a.X, b.Y}, b}, {a, {(a.X + b.X) / 2, a.Y}, {(a.X + b.X) / 2, b.Y}, b}, {a, {a.X, (a.Y + b.Y) / 2}, {b.X, (a.Y + b.Y) / 2}, b}}
	for _, candidate := range candidates {
		p := simplifyPipePath(candidate)
		clear := true
		for i := 0; i < len(p); i++ {
			previous := p[max(0, i-1)]
			ok, budget := r.clear(previous, p[i], -1)
			if budget {
				return nil, PipeRouteBudget, 0
			}
			if !ok {
				clear = false
				break
			}
		}
		if clear {
			return p, PipeRouteOK, 0
		}
	}
	if limit <= 0 {
		return nil, PipeRouteBudget, 0
	}
	xs, ys := []float64{a.X, b.X}, []float64{a.Y, b.Y}
	for _, o := range r.obstacles {
		xs = append(xs, o.Bounds.X, o.Bounds.X+o.Bounds.Width)
		ys = append(ys, o.Bounds.Y, o.Bounds.Y+o.Bounds.Height)
	}
	if r.options.Bounds != (Rect{}) {
		z := r.options.Bounds
		xs = append(xs, z.X, z.X+z.Width)
		ys = append(ys, z.Y, z.Y+z.Height)
	}
	axes := func(v []float64, horizontal bool) []float64 {
		sort.Float64s(v)
		v = slices.Compact(v)
		if r.options.Bounds != (Rect{}) {
			z := r.options.Bounds
			lo, hi := z.Y, z.Y+z.Height
			if horizontal {
				lo, hi = z.X, z.X+z.Width
			}
			v = slices.DeleteFunc(v, func(x float64) bool { return x < lo || x > hi })
		}
		return v
	}
	xs, ys = axes(xs, true), axes(ys, false)
	if !r.inBounds(a) || !r.inBounds(b) {
		return nil, PipeRouteBlocked, 0
	}
	nx := len(xs)
	grid := func(p XY) int { return sort.SearchFloat64s(xs, p.X) + sort.SearchFloat64s(ys, p.Y)*nx }
	start, goal := grid(a), grid(b)
	pointAt := func(i int) XY { return XY{xs[i%nx], ys[i/nx]} }
	key := start * 3
	best := map[int]float64{key: 0}
	parents := map[int]int{key: -1}
	queue := pipeSearchHeap{{key: key, f: math.Abs(a.X-b.X) + math.Abs(a.Y-b.Y)}}
	type edge struct{ a, b int }
	edges := map[edge]bool{}
	visited := 0
	for len(queue) > 0 {
		s := heap.Pop(&queue).(pipeSearchState)
		if s.g != best[s.key] {
			continue
		}
		if visited >= limit {
			return nil, PipeRouteBudget, visited
		}
		visited++
		current := s.key / 3
		if current == goal {
			var path []XY
			for k := s.key; k >= 0; k = parents[k] {
				path = append(path, pointAt(k/3))
			}
			slices.Reverse(path)
			return simplifyPipePath(path), PipeRouteOK, visited
		}
		x, y := current%nx, current/nx
		for _, d := range [][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
			u, v := x+d[0], y+d[1]
			if u < 0 || v < 0 || u >= nx || v >= len(ys) {
				continue
			}
			next := u + v*nx
			e := edge{min(current, next), max(current, next)}
			clear, known := edges[e]
			p, q := pointAt(current), pointAt(next)
			if !known {
				var budget bool
				clear, budget = r.clear(p, q, -1)
				if budget {
					return nil, PipeRouteBudget, visited
				}
				edges[e] = clear
			}
			if !clear {
				continue
			}
			axis := 1
			if d[1] != 0 {
				axis = 2
			}
			cost := s.g + math.Abs(p.X-q.X) + math.Abs(p.Y-q.Y)
			if s.key%3 != 0 && s.key%3 != axis {
				cost += r.options.TurnPenalty
			}
			k := next*3 + axis
			if old, ok := best[k]; ok && old <= cost {
				continue
			}
			best[k] = cost
			parents[k] = s.key
			heap.Push(&queue, pipeSearchState{key: k, g: cost, f: cost + math.Abs(q.X-b.X) + math.Abs(q.Y-b.Y)})
		}
	}
	return nil, PipeRouteBlocked, visited
}

type pipeSearchState struct {
	key  int
	g, f float64
}
type pipeSearchHeap []pipeSearchState

func (h pipeSearchHeap) Len() int { return len(h) }
func (h pipeSearchHeap) Less(i, j int) bool {
	a, b := h[i], h[j]
	if a.f != b.f {
		return a.f < b.f
	}
	if a.g != b.g {
		return a.g > b.g
	}
	return a.key < b.key
}
func (h pipeSearchHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *pipeSearchHeap) Push(x any)   { *h = append(*h, x.(pipeSearchState)) }
func (h *pipeSearchHeap) Pop() any     { a := *h; x := a[len(a)-1]; *h = a[:len(a)-1]; return x }
