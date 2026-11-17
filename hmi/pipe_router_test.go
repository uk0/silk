package hmi

import (
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"slices"
	"testing"
)

func pipeFixture() ([]PipeObstacle, PipeAnchor, PipeAnchor) {
	return []PipeObstacle{{"a", Rect{40, 100, 80, 60}}, {"b", Rect{400, 100, 80, 60}}, {"blocker", Rect{220, 70, 80, 120}}}, PipeAnchor{XY{120, 130}, "east", "a"}, PipeAnchor{XY{400, 130}, "west", "b"}
}

// Independent assertion: inspect every rendered segment, not the router's status
// or its private visibility predicate. Only the first/last own-body stubs may
// traverse the matching inflated owner.
func assertPipeClear(t *testing.T, p []XY, obstacles []PipeObstacle, from, to PipeAnchor, clearance float64) {
	t.Helper()
	if len(p) < 2 || p[0] != from.Point || p[len(p)-1] != to.Point {
		t.Fatal("wrong route endpoints", p)
	}
	for i := 1; i < len(p); i++ {
		a, b := p[i-1], p[i]
		if a == b || a.X != b.X && a.Y != b.Y {
			t.Fatal("non-rectilinear or empty segment", i, p)
		}
		for _, o := range obstacles {
			if i == 1 && o.ID == from.Owner || i == len(p)-1 && o.ID == to.Owner {
				continue
			}
			x0, y0 := o.Bounds.X-clearance, o.Bounds.Y-clearance
			x1, y1 := o.Bounds.X+o.Bounds.Width+clearance, o.Bounds.Y+o.Bounds.Height+clearance
			cross := a.Y == b.Y && a.Y > y0 && a.Y < y1 && math.Min(a.X, b.X) < x1 && math.Max(a.X, b.X) > x0 || a.X == b.X && a.X > x0 && a.X < x1 && math.Min(a.Y, b.Y) < y1 && math.Max(a.Y, b.Y) > y0
			if cross {
				t.Fatalf("segment %d crosses %s: %v", i, o.ID, p)
			}
		}
	}
}

func TestPipeRouterAvoidsBlockerAndUsesPortDirections(t *testing.T) {
	obs, from, to := pipeFixture()
	r, err := NewPipeRouter(obs, PipeRouteOptions{Bounds: Rect{0, 0, 600, 300}})
	if err != nil {
		t.Fatal(err)
	}
	got := r.Route(from, to, nil)
	if got.Status != PipeRouteOK || got.Work == 0 || got.Visited == 0 {
		t.Fatal("did not search around blocker", got)
	}
	assertPipeClear(t, got.Points, obs, from, to, 8)
	if got.Points[1].X <= from.Point.X || got.Points[len(got.Points)-2].X >= to.Point.X {
		t.Fatal("port directions lost", got.Points)
	}
	for _, fd := range []string{"north", "south", "east", "west"} {
		for _, td := range []string{"north", "south", "east", "west"} {
			p := Template()
			p.Nodes = p.Nodes[:2]
			a, b := p.Nodes[0], p.Nodes[1]
			obs := []PipeObstacle{{a.ID, Rect{a.X, a.Y, a.Width, a.Height}}, {b.ID, Rect{b.X, b.Y, b.Width, b.Height}}}
			from, to := PipeAnchor{Port(a, fd), fd, a.ID}, PipeAnchor{Port(b, td), td, b.ID}
			r, e := NewPipeRouter(obs, PipeRouteOptions{WorkLimit: 2000000})
			if e != nil {
				t.Fatal(e)
			}
			got := r.Route(from, to, nil)
			if got.Status != PipeRouteOK {
				t.Fatal(fd, td, got)
			}
			assertPipeClear(t, got.Points, obs, from, to, 8)
		}
	}
}

func TestPipeRouterWaypointsAndHonestFallback(t *testing.T) {
	obs, from, to := pipeFixture()
	r, _ := NewPipeRouter(obs, PipeRouteOptions{Bounds: Rect{0, 0, 600, 300}})
	bends := []XY{{190, 25}, {350, 25}, {350, 230}}
	got := r.Route(from, to, bends)
	if got.Status != PipeRouteOK {
		t.Fatal(got)
	}
	assertPipeClear(t, got.Points, obs, from, to, 8)
	previous := -1
	for _, b := range bends {
		index := slices.Index(got.Points, b)
		if index <= previous {
			t.Fatal("authored waypoint/order lost", b, got.Points)
		}
		previous = index
	}
	for _, bends := range [][]XY{{{250, 100}}, {{-1, 100}}, {{30, 30}, {250, 100}}} {
		r, _ := NewPipeRouter(obs, PipeRouteOptions{Bounds: Rect{0, 0, 600, 300}})
		got := r.Route(from, to, bends)
		if got.Status != PipeRouteBlocked || got.Message == "" || !reflect.DeepEqual(got.Points, Route(from.Point, to.Point, bends)) {
			t.Fatal("false collision-free claim", got)
		}
	}
	blocked := append(slices.Clone(obs), PipeObstacle{"exit-blocker", Rect{122, 120, 20, 20}})
	r, _ = NewPipeRouter(blocked, PipeRouteOptions{})
	got = r.Route(from, to, nil)
	if got.Status != PipeRouteBlocked || got.Visited != 0 {
		t.Fatal("obstructed port tunneled through equipment", got)
	}
	from.Point.X = 40
	from.Direction = "west"
	r, _ = NewPipeRouter(obs, PipeRouteOptions{Bounds: Rect{40, 0, 560, 300}})
	got = r.Route(from, to, nil)
	if got.Status != PipeRouteBlocked {
		t.Fatal("escaped outside drawing", got)
	}
	obs, from, to = pipeFixture()
	obs[2].Bounds = Rect{220, 0, 80, 300}
	r, _ = NewPipeRouter(obs, PipeRouteOptions{Bounds: Rect{0, 0, 600, 300}})
	if got = r.Route(from, to, nil); got.Status != PipeRouteBlocked || got.Visited == 0 {
		t.Fatal("solid wall reported success or an unproven budget failure", got)
	}
}

func TestPipeRouterBudgetIsSharedBoundedAndDeterministic(t *testing.T) {
	obs, from, to := pipeFixture()
	for _, o := range []PipeRouteOptions{{MaxVisited: 1}, {WorkLimit: 1}} {
		r, _ := NewPipeRouter(obs, o)
		got := r.Route(from, to, nil)
		if got.Status != PipeRouteBudget || got.Message == "" || !reflect.DeepEqual(got.Points, Route(from.Point, to.Point, nil)) {
			t.Fatal(got)
		}
		if o.MaxVisited != 0 && got.Visited > o.MaxVisited || o.WorkLimit != 0 && r.WorkUsed() > o.WorkLimit {
			t.Fatal("budget exceeded", got, r.WorkUsed())
		}
	}
	r, _ := NewPipeRouter(obs, PipeRouteOptions{WorkLimit: 200})
	for i := 0; i < 100; i++ {
		got := r.Route(from, to, nil)
		if r.WorkUsed() > 200 || got.Work > 200 {
			t.Fatal("work is not cumulative", got, r.WorkUsed())
		}
	}
	if r.WorkUsed() != 200 || r.Route(from, to, nil).Status != PipeRouteBudget {
		t.Fatal("work budget silently restarted")
	}
	r, _ = NewPipeRouter(obs, PipeRouteOptions{})
	first := r.Route(from, to, nil)
	slices.Reverse(obs)
	other, _ := NewPipeRouter(obs, PipeRouteOptions{})
	second := other.Route(from, to, nil)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("obstacle order changed result", first, second)
	}
	obs[0].Bounds.X = 100000 // Router owns a geometry copy.
	first.Points[0].X = -1
	third, _ := NewPipeRouter([]PipeObstacle{{"a", Rect{40, 100, 80, 60}}, {"b", Rect{400, 100, 80, 60}}, {"blocker", Rect{220, 70, 80, 120}}}, PipeRouteOptions{})
	if !reflect.DeepEqual(other.Route(from, to, nil).Points, third.Route(from, to, nil).Points) {
		t.Fatal("caller modified router geometry or result storage")
	}
}

func TestPipeRouterValidation(t *testing.T) {
	obs, from, to := pipeFixture()
	for _, o := range []PipeRouteOptions{{Clearance: -1}, {Clearance: math.NaN()}, {Stub: math.Inf(1)}, {TurnPenalty: -1}, {MaxVisited: 32769}, {WorkLimit: 2000001}, {Bounds: Rect{1, 1, 0, 0}}} {
		if _, e := NewPipeRouter(obs, o); e == nil {
			t.Fatal("invalid options admitted", o)
		}
	}
	for _, invalid := range [][]PipeObstacle{append(slices.Clone(obs), obs[0]), {{"bad", Rect{0, 0, 0, 1}}}, {{"bad", Rect{math.NaN(), 0, 1, 1}}}, make([]PipeObstacle, 513)} {
		if _, e := NewPipeRouter(invalid, PipeRouteOptions{}); e == nil {
			t.Fatal("invalid obstacles admitted")
		}
	}
	r, _ := NewPipeRouter(obs, PipeRouteOptions{})
	for _, a := range []PipeAnchor{{XY{math.NaN(), 0}, "east", "a"}, {from.Point, "right", "a"}, {from.Point, "east", "missing"}, {XY{999, 999}, "east", "a"}} {
		if got := r.Route(a, to, nil); got.Status != PipeRouteInvalid || len(got.Points) != 0 {
			t.Fatal("invalid anchor produced geometry", got)
		}
	}
	if got := r.Route(from, to, make([]XY, 33)); got.Status != PipeRouteInvalid {
		t.Fatal("unbounded waypoints", got)
	}
	if got := r.Route(from, to, []XY{{math.Inf(1), 0}}); got.Status != PipeRouteInvalid {
		t.Fatal(got)
	}
	var nilRouter *PipeRouter
	if nilRouter.Route(from, to, nil).Status != PipeRouteInvalid || nilRouter.WorkUsed() != 0 {
		t.Fatal("nil receiver contract")
	}
}

func TestPipeRouterRandomSuccessesNeverCrossBodies(t *testing.T) {
	random := rand.New(rand.NewSource(413))
	success := 0
	for i := 0; i < 200; i++ {
		obs, from, to := pipeFixture()
		obs = obs[:2]
		for j := 0; j < 12; j++ {
			obs = append(obs, PipeObstacle{ID: string(rune('d' + j)), Bounds: Rect{150 + random.Float64()*220, 20 + random.Float64()*230, 10 + random.Float64()*30, 10 + random.Float64()*30}})
		}
		r, e := NewPipeRouter(obs, PipeRouteOptions{Bounds: Rect{0, 0, 600, 300}})
		if e != nil {
			t.Fatal(e)
		}
		got := r.Route(from, to, nil)
		if got.Status == PipeRouteOK {
			success++
			assertPipeClear(t, got.Points, obs, from, to, 8)
		} else if got.Status != PipeRouteBlocked && got.Status != PipeRouteBudget {
			t.Fatal(got)
		}
		if r.WorkUsed() > 200000 || got.Visited > 2048 {
			t.Fatal("random request exceeded budget", got)
		}
	}
	if success < 100 {
		t.Fatal("avoidance too weak", success)
	}
}

func TestPipeRoutingSchemaCompatibilityAndCloning(t *testing.T) {
	p := Template()
	data, e := p.JSON()
	if e != nil {
		t.Fatal(e)
	}
	old, e := Parse(data)
	if e != nil || old.Links[0].Routing != "" {
		t.Fatal("legacy routing changed", e)
	}
	p.Links[0].Routing = RoutingAvoid
	p.Links[1].Routing = RoutingOrthogonal
	data, e = p.JSON()
	if e != nil {
		t.Fatal(e)
	}
	round, e := Parse(data)
	if e != nil || round.Links[0].Routing != RoutingAvoid || round.Links[1].Routing != RoutingOrthogonal {
		t.Fatal("routing lost in document", e)
	}
	clone, e := CloneProject(round)
	if e != nil || clone.Links[0].Routing != RoutingAvoid {
		t.Fatal("routing lost on clone", e)
	}
	device, e := CaptureDeviceTemplate(round, []string{"reactor", "exchanger", "separator", "temperature"}, "route-device", "Routed device")
	if e != nil {
		t.Fatal(e)
	}
	host := Project{Version: Version, Width: 2000, Height: 1200, Title: "Host"}
	placed, _, e := PlaceDeviceTemplate(host, device, DevicePlacement{ID: "instance", X: 30, Y: 30})
	if e != nil || placed.Links[0].Routing != RoutingAvoid || placed.Links[1].Routing != RoutingOrthogonal {
		t.Fatal("routing lost in device placement", e)
	}
	p.Links[0].Routing = "shortest"
	if e = p.Validate(); e == nil {
		t.Fatal("unknown routing mode accepted")
	}
}

func densePipeObstacles() []PipeObstacle {
	obs, _, _ := pipeFixture()
	for i := 0; i < 509; i++ {
		obs = append(obs, PipeObstacle{ID: fmt.Sprintf("far-%03d", i), Bounds: Rect{1000 + float64(i%24)*120, 1000 + float64(i/24)*120, 30, 30}})
	}
	return obs
}

func TestPipeSpatialIndexMatchesFlatCollisionOracle(t *testing.T) {
	random := rand.New(rand.NewSource(612))
	var obs []PipeObstacle
	for i := 0; i < 128; i++ {
		obs = append(obs, PipeObstacle{ID: fmt.Sprint(i), Bounds: Rect{random.Float64() * 600, random.Float64() * 600, 5 + random.Float64()*70, 5 + random.Float64()*70}})
	}
	r, e := NewPipeRouter(obs, PipeRouteOptions{WorkLimit: 2000000})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 10000; i++ {
		a := XY{random.Float64()*800 - 50, random.Float64()*800 - 50}
		b := XY{random.Float64()*800 - 50, a.Y}
		if i%2 != 0 {
			b = XY{a.X, random.Float64()*800 - 50}
		}
		// Include exact indexed boundaries, not just generic random coordinates.
		if i%4 == 0 {
			box := r.obstacles[i%len(obs)].Bounds
			a.Y = box.Y
			b.Y = box.Y
		}
		if i%4 == 1 {
			box := r.obstacles[i%len(obs)].Bounds
			a.X = box.X + box.Width
			b.X = a.X
		}
		skip := -1
		if i%3 == 0 {
			skip = i % len(obs)
		}
		want := true
		for j, o := range r.obstacles {
			if j == skip {
				continue
			}
			z := o.Bounds
			hit := a.Y == b.Y && a.Y > z.Y && a.Y < z.Y+z.Height && min(a.X, b.X) < z.X+z.Width && max(a.X, b.X) > z.X || a.X == b.X && a.X > z.X && a.X < z.X+z.Width && min(a.Y, b.Y) < z.Y+z.Height && max(a.Y, b.Y) > z.Y
			if hit {
				want = false
				break
			}
		}
		got, budget := r.clear(a, b, skip)
		if budget || got != want {
			t.Fatal("index differs from flat open-rectangle oracle", i, a, b, skip, got, want, budget)
		}
	}
}

func TestPipeSpatialIndexSkipsRemoteEquipmentWithinSharedBudget(t *testing.T) {
	obs := densePipeObstacles()
	_, from, to := pipeFixture()
	r, e := NewPipeRouter(obs, PipeRouteOptions{})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 100; i++ {
		got := r.Route(from, to, nil)
		if got.Status != PipeRouteOK {
			t.Fatal("unrelated equipment consumed routing budget", i, got)
		}
		if i == 0 {
			assertPipeClear(t, got.Points, obs, from, to, 8)
		}
	}
	before := r.WorkUsed()
	clear, budget := r.clear(XY{0, 20000}, XY{9000, 20000}, -1)
	if !clear || budget || r.WorkUsed()-before > 30 {
		t.Fatal("query scanned remote equipment", clear, budget, r.WorkUsed()-before)
	}
	t.Logf("512 obstacles, 100 independent routes: %d shared work units", before)
}

func BenchmarkPipeRouterDenseGeometry(b *testing.B) {
	obs := densePipeObstacles()
	_, from, to := pipeFixture()
	b.ReportAllocs()
	for b.Loop() {
		r, _ := NewPipeRouter(obs, PipeRouteOptions{})
		if r.Route(from, to, nil).Status != PipeRouteOK {
			b.Fatal("route failed")
		}
	}
}

func BenchmarkPipeRouterBlockedGeometry(b *testing.B) {
	obs, from, to := pipeFixture()
	b.ReportAllocs()
	for b.Loop() {
		r, _ := NewPipeRouter(obs, PipeRouteOptions{})
		if r.Route(from, to, nil).Status != PipeRouteOK {
			b.Fatal("route failed")
		}
	}
}
