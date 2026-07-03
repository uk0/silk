package graph

import (
	"github.com/uk0/silk/geom"
	"testing"
)

// newTestItem creates a RectItem for testing purposes.
func newTestItem() *RectItem {
	return NewRectItem()
}

// --- Item creation and basic properties ---

func TestNewRectItem(t *testing.T) {
	item := newTestItem()
	if item == nil {
		t.Fatal("NewRectItem returned nil")
	}
	if item.Self() != IItem(item) {
		t.Error("Self() should return the item itself")
	}
	if item.NakedItem() != &item.Item {
		t.Error("NakedItem() should return the embedded Item")
	}
}

func TestItemDefaultValues(t *testing.T) {
	item := newTestItem()
	if item.X() != 0 || item.Y() != 0 {
		t.Errorf("default position should be (0,0), got (%v,%v)", item.X(), item.Y())
	}
	if item.Width() != 0 || item.Height() != 0 {
		t.Errorf("default size should be (0,0), got (%v,%v)", item.Width(), item.Height())
	}
	if !item.IsVisible() {
		t.Error("item should be visible by default")
	}
	if !item.IsSelectable() {
		t.Error("item should be selectable by default")
	}
	if item.IsLockPos() {
		t.Error("lockPos should be false by default")
	}
	if item.IsLockSize() {
		t.Error("lockSize should be false by default")
	}
	if item.HasChildren() {
		t.Error("new item should have no children")
	}
	if item.HasLocalCoord() {
		t.Error("localCoord should be false by default")
	}
}

// --- Bounds setting ---

func TestSetBounds(t *testing.T) {
	item := newTestItem()
	item.SetBounds(10, 20, 100, 200)

	x, y, w, h := item.Bounds()
	if x != 10 || y != 20 || w != 100 || h != 200 {
		t.Errorf("Bounds() = (%v,%v,%v,%v), want (10,20,100,200)", x, y, w, h)
	}
}

func TestSetBounds1(t *testing.T) {
	item := newTestItem()
	r := geom.Rect{X: 5, Y: 15, Width: 50, Height: 75}
	item.SetBounds1(r)

	got := item.Bounds1()
	if got != r {
		t.Errorf("Bounds1() = %v, want %v", got, r)
	}
}

func TestSetPos(t *testing.T) {
	item := newTestItem()
	item.SetBounds(0, 0, 100, 100)
	item.SetPos(30, 40)

	x, y := item.Pos()
	if x != 30 || y != 40 {
		t.Errorf("Pos() = (%v,%v), want (30,40)", x, y)
	}
	// size should remain unchanged
	w, h := item.Size()
	if w != 100 || h != 100 {
		t.Errorf("Size() = (%v,%v), want (100,100)", w, h)
	}
}

func TestSetSize(t *testing.T) {
	item := newTestItem()
	item.SetBounds(10, 20, 100, 200)
	item.SetSize(300, 400)

	w, h := item.Size()
	if w != 300 || h != 400 {
		t.Errorf("Size() = (%v,%v), want (300,400)", w, h)
	}
	// position should remain unchanged
	x, y := item.Pos()
	if x != 10 || y != 20 {
		t.Errorf("Pos() = (%v,%v), want (10,20)", x, y)
	}
}

func TestSetXYWidthHeight(t *testing.T) {
	item := newTestItem()
	item.SetX(11)
	item.SetY(22)
	item.SetWidth(33)
	item.SetHeight(44)

	if item.X() != 11 {
		t.Errorf("X() = %v, want 11", item.X())
	}
	if item.Y() != 22 {
		t.Errorf("Y() = %v, want 22", item.Y())
	}
	if item.Width() != 33 {
		t.Errorf("Width() = %v, want 33", item.Width())
	}
	if item.Height() != 44 {
		t.Errorf("Height() = %v, want 44", item.Height())
	}
}

// --- Visibility ---

func TestVisibility(t *testing.T) {
	item := newTestItem()
	if !item.IsVisible() {
		t.Error("should be visible by default")
	}

	item.Hide()
	if item.IsVisible() {
		t.Error("should be hidden after Hide()")
	}

	item.Show()
	if !item.IsVisible() {
		t.Error("should be visible after Show()")
	}

	item.SetVisible(false)
	if item.IsVisible() {
		t.Error("should be hidden after SetVisible(false)")
	}

	item.SetVisible(true)
	if !item.IsVisible() {
		t.Error("should be visible after SetVisible(true)")
	}
}

// --- Selectable, LockPos, LockSize ---

func TestSelectable(t *testing.T) {
	item := newTestItem()
	item.SetSelectable(false)
	if item.IsSelectable() {
		t.Error("should not be selectable after SetSelectable(false)")
	}
	item.SetSelectable(true)
	if !item.IsSelectable() {
		t.Error("should be selectable after SetSelectable(true)")
	}
}

func TestLockPos(t *testing.T) {
	item := newTestItem()
	item.SetLockPos(true)
	if !item.IsLockPos() {
		t.Error("expected lockPos true")
	}
	item.SetLockPos(false)
	if item.IsLockPos() {
		t.Error("expected lockPos false")
	}
}

func TestLockSize(t *testing.T) {
	item := newTestItem()
	item.SetLockSize(true)
	if !item.IsLockSize() {
		t.Error("expected lockSize true")
	}
	item.SetLockSize(false)
	if item.IsLockSize() {
		t.Error("expected lockSize false")
	}
}

// --- Parent-child relationships ---

func TestSetParentSingle(t *testing.T) {
	parent := newTestItem()
	child := newTestItem()

	child.SetParent(parent)

	if child.Parent() != IItem(parent) {
		t.Error("child.Parent() should be parent")
	}
	if !parent.HasChildren() {
		t.Error("parent should have children")
	}

	children := parent.Children()
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	if children[0] != IItem(child) {
		t.Error("child mismatch")
	}
}

func TestSetParentMultiple(t *testing.T) {
	parent := newTestItem()
	c1 := newTestItem()
	c2 := newTestItem()
	c3 := newTestItem()

	c1.SetParent(parent)
	c2.SetParent(parent)
	c3.SetParent(parent)

	children := parent.Children()
	if len(children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(children))
	}
	if children[0] != IItem(c1) || children[1] != IItem(c2) || children[2] != IItem(c3) {
		t.Error("children order mismatch")
	}
}

func TestDetach(t *testing.T) {
	parent := newTestItem()
	child := newTestItem()

	child.SetParent(parent)
	child.Detach()

	if child.Parent() != nil {
		t.Error("parent should be nil after Detach")
	}
	if parent.HasChildren() {
		t.Error("parent should have no children after child detached")
	}
}

func TestDetachMiddleChild(t *testing.T) {
	parent := newTestItem()
	c1 := newTestItem()
	c2 := newTestItem()
	c3 := newTestItem()

	c1.SetParent(parent)
	c2.SetParent(parent)
	c3.SetParent(parent)

	c2.Detach()

	children := parent.Children()
	if len(children) != 2 {
		t.Fatalf("expected 2 children after detach, got %d", len(children))
	}
	if children[0] != IItem(c1) || children[1] != IItem(c3) {
		t.Error("remaining children mismatch")
	}
}

func TestDetachFirstChild(t *testing.T) {
	parent := newTestItem()
	c1 := newTestItem()
	c2 := newTestItem()

	c1.SetParent(parent)
	c2.SetParent(parent)

	c1.Detach()

	children := parent.Children()
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	if children[0] != IItem(c2) {
		t.Error("remaining child should be c2")
	}
}

func TestDetachLastChild(t *testing.T) {
	parent := newTestItem()
	c1 := newTestItem()
	c2 := newTestItem()

	c1.SetParent(parent)
	c2.SetParent(parent)

	c2.Detach()

	children := parent.Children()
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	if children[0] != IItem(c1) {
		t.Error("remaining child should be c1")
	}
}

func TestReparent(t *testing.T) {
	p1 := newTestItem()
	p2 := newTestItem()
	child := newTestItem()

	child.SetParent(p1)
	if child.Parent() != IItem(p1) {
		t.Error("parent should be p1")
	}

	child.SetParent(p2)
	if child.Parent() != IItem(p2) {
		t.Error("parent should be p2 after reparent")
	}
	if p1.HasChildren() {
		t.Error("p1 should have no children")
	}
	if !p2.HasChildren() {
		t.Error("p2 should have children")
	}
}

// --- Tree operations ---

func TestRoot(t *testing.T) {
	root := newTestItem()
	child := newTestItem()
	grandchild := newTestItem()

	child.SetParent(root)
	grandchild.SetParent(child)

	if root.Root() != IItem(root) {
		t.Error("root.Root() should be itself")
	}
	if child.Root() != IItem(root) {
		t.Error("child.Root() should be root")
	}
	if grandchild.Root() != IItem(root) {
		t.Error("grandchild.Root() should be root")
	}
}

func TestTreeItems(t *testing.T) {
	root := newTestItem()
	c1 := newTestItem()
	c2 := newTestItem()
	gc1 := newTestItem()

	c1.SetParent(root)
	c2.SetParent(root)
	gc1.SetParent(c1)

	items := root.TreeItems()
	// Pre-order: root, c1, gc1, c2
	if len(items) != 4 {
		t.Fatalf("expected 4 tree items, got %d", len(items))
	}
	if items[0] != IItem(root) {
		t.Error("items[0] should be root")
	}
	if items[1] != IItem(c1) {
		t.Error("items[1] should be c1")
	}
	if items[2] != IItem(gc1) {
		t.Error("items[2] should be gc1")
	}
	if items[3] != IItem(c2) {
		t.Error("items[3] should be c2")
	}
}

func TestPath(t *testing.T) {
	root := newTestItem()
	child := newTestItem()
	grandchild := newTestItem()

	child.SetParent(root)
	grandchild.SetParent(child)

	path := grandchild.Path(IItem(root))
	if len(path) != 3 {
		t.Fatalf("expected path length 3, got %d", len(path))
	}
	if path[0] != IItem(root) {
		t.Error("path[0] should be root")
	}
	if path[1] != IItem(child) {
		t.Error("path[1] should be child")
	}
	if path[2] != IItem(grandchild) {
		t.Error("path[2] should be grandchild")
	}
}

func TestPathNoAncestor(t *testing.T) {
	a := newTestItem()
	b := newTestItem()

	// b is not a descendant of a
	path := b.Path(IItem(a))
	if path != nil {
		t.Error("path should be nil when no ancestor relationship exists")
	}
}

func TestPathNilAncestor(t *testing.T) {
	root := newTestItem()
	child := newTestItem()
	child.SetParent(root)

	// nil ancestor should use Root()
	path := child.Path(nil)
	if len(path) != 2 {
		t.Fatalf("expected path length 2, got %d", len(path))
	}
	if path[0] != IItem(root) || path[1] != IItem(child) {
		t.Error("path with nil ancestor should go from root to child")
	}
}

// --- Scene detection (no scene case) ---

func TestSceneNilForPlainItems(t *testing.T) {
	item := newTestItem()
	if item.Scene() != nil {
		t.Error("Scene() should be nil for items not attached to a scene")
	}
}

// --- Hit testing ---

func TestIsHitBody(t *testing.T) {
	item := newTestItem()
	item.SetBounds(10, 20, 100, 50)

	tests := []struct {
		x, y float64
		hit  bool
		desc string
	}{
		{50, 40, true, "center"},
		{10, 20, true, "top-left corner"},
		{110, 70, true, "bottom-right corner"},
		{5, 40, false, "left of bounds"},
		{120, 40, false, "right of bounds"},
		{50, 15, false, "above bounds"},
		{50, 80, false, "below bounds"},
	}

	for _, tt := range tests {
		got := item.IsHitBody(tt.x, tt.y)
		if got != tt.hit {
			t.Errorf("IsHitBody(%v,%v) [%s] = %v, want %v", tt.x, tt.y, tt.desc, got, tt.hit)
		}
	}
}

func TestEaveBounds(t *testing.T) {
	item := newTestItem()
	item.SetBounds(10, 20, 100, 50)

	eb := item.EaveBounds1()
	// default eave is 1, so bounds expand by 1 in each direction
	if eb.X != 9 || eb.Y != 19 || eb.Width != 102 || eb.Height != 52 {
		t.Errorf("EaveBounds1() = %v, want {9,19,102,52}", eb)
	}
}

func TestMaybeHitBody(t *testing.T) {
	item := newTestItem()
	item.SetBounds(10, 20, 100, 50)

	// Point just outside bounds but within eave
	if !item.MaybeHitBody(9.5, 20) {
		t.Error("MaybeHitBody should be true within eave zone")
	}
	// Point far outside
	if item.MaybeHitBody(0, 0) {
		t.Error("MaybeHitBody should be false far outside bounds")
	}
}

// --- Coordinate mapping ---

func TestMapToChildNoLocalCoord(t *testing.T) {
	item := newTestItem()
	item.SetBounds(10, 20, 100, 100)
	item.SetLocalCoord(false)

	x, y := item.MapToChild(50, 60)
	if x != 50 || y != 60 {
		t.Errorf("MapToChild without localCoord = (%v,%v), want (50,60)", x, y)
	}
}

func TestMapToChildWithLocalCoord(t *testing.T) {
	item := newTestItem()
	item.SetBounds(10, 20, 100, 100)
	item.SetLocalCoord(true)

	x, y := item.MapToChild(50, 60)
	if x != 40 || y != 40 {
		t.Errorf("MapToChild with localCoord = (%v,%v), want (40,40)", x, y)
	}
}

func TestMapFromChildNoLocalCoord(t *testing.T) {
	item := newTestItem()
	item.SetBounds(10, 20, 100, 100)
	item.SetLocalCoord(false)

	x, y := item.MapFromChild(50, 60)
	if x != 50 || y != 60 {
		t.Errorf("MapFromChild without localCoord = (%v,%v), want (50,60)", x, y)
	}
}

func TestMapFromChildWithLocalCoord(t *testing.T) {
	item := newTestItem()
	item.SetBounds(10, 20, 100, 100)
	item.SetLocalCoord(true)

	x, y := item.MapFromChild(40, 40)
	if x != 50 || y != 60 {
		t.Errorf("MapFromChild with localCoord = (%v,%v), want (50,60)", x, y)
	}
}

func TestMapRectToChildWithLocalCoord(t *testing.T) {
	item := newTestItem()
	item.SetBounds(10, 20, 100, 100)
	item.SetLocalCoord(true)

	x, y, w, h := item.MapRectToChild(50, 60, 30, 40)
	if x != 40 || y != 40 || w != 30 || h != 40 {
		t.Errorf("MapRectToChild = (%v,%v,%v,%v), want (40,40,30,40)", x, y, w, h)
	}
}

func TestMapRectFromChildWithLocalCoord(t *testing.T) {
	item := newTestItem()
	item.SetBounds(10, 20, 100, 100)
	item.SetLocalCoord(true)

	x, y, w, h := item.MapRectFromChild(40, 40, 30, 40)
	if x != 50 || y != 60 || w != 30 || h != 40 {
		t.Errorf("MapRectFromChild = (%v,%v,%v,%v), want (50,60,30,40)", x, y, w, h)
	}
}

func TestCoordOffsetRootItem(t *testing.T) {
	item := newTestItem()
	item.SetBounds(10, 20, 100, 100)

	dx, dy := item.CoordOffset()
	if dx != 0 || dy != 0 {
		t.Errorf("CoordOffset for root = (%v,%v), want (0,0)", dx, dy)
	}
}

func TestCoordOffsetWithLocalCoordParent(t *testing.T) {
	parent := newTestItem()
	parent.SetBounds(10, 20, 200, 200)
	parent.SetLocalCoord(true)

	child := newTestItem()
	child.SetBounds(5, 5, 50, 50)
	child.SetParent(parent)

	dx, dy := child.CoordOffset()
	if dx != 10 || dy != 20 {
		t.Errorf("CoordOffset = (%v,%v), want (10,20)", dx, dy)
	}
}

func TestCoordOffsetWithoutLocalCoordParent(t *testing.T) {
	parent := newTestItem()
	parent.SetBounds(10, 20, 200, 200)
	parent.SetLocalCoord(false)

	child := newTestItem()
	child.SetBounds(5, 5, 50, 50)
	child.SetParent(parent)

	dx, dy := child.CoordOffset()
	if dx != 0 || dy != 0 {
		t.Errorf("CoordOffset = (%v,%v), want (0,0)", dx, dy)
	}
}

func TestMapToFromSceneRoundtrip(t *testing.T) {
	parent := newTestItem()
	parent.SetBounds(10, 20, 200, 200)
	parent.SetLocalCoord(true)

	child := newTestItem()
	child.SetBounds(5, 5, 50, 50)
	child.SetParent(parent)

	// Map to scene and back
	sx, sy := child.MapToScene(100, 100)
	rx, ry := child.MapFromScene(sx, sy)
	if rx != 100 || ry != 100 {
		t.Errorf("MapToScene/MapFromScene roundtrip = (%v,%v), want (100,100)", rx, ry)
	}
}

// --- Rect1-variant coordinate mapping ---

func TestMapRectToScene1(t *testing.T) {
	parent := newTestItem()
	parent.SetBounds(10, 20, 200, 200)
	parent.SetLocalCoord(true)

	child := newTestItem()
	child.SetBounds(5, 5, 50, 50)
	child.SetParent(parent)

	r := geom.Rect{X: 0, Y: 0, Width: 30, Height: 40}
	mapped := child.MapRectToScene1(r)
	if mapped.X != 10 || mapped.Y != 20 || mapped.Width != 30 || mapped.Height != 40 {
		t.Errorf("MapRectToScene1 = %v, want {10,20,30,40}", mapped)
	}
}

// --- TraversalCond functions from misc.go ---

func TestTraversalCondSelectable(t *testing.T) {
	item := newTestItem()
	item.SetVisible(true)
	item.SetSelectable(true)

	self, desc := TraversalCond_Selectable(item)
	if !self || !desc {
		t.Errorf("visible+selectable: self=%v, desc=%v, want true,true", self, desc)
	}

	item.SetSelectable(false)
	self, desc = TraversalCond_Selectable(item)
	if self || !desc {
		t.Errorf("visible+!selectable: self=%v, desc=%v, want false,true", self, desc)
	}

	item.SetVisible(false)
	self, desc = TraversalCond_Selectable(item)
	if self || desc {
		t.Errorf("!visible: self=%v, desc=%v, want false,false", self, desc)
	}
}

func TestTraversalCondSelectableAndMoveable(t *testing.T) {
	item := newTestItem()
	item.SetVisible(true)
	item.SetSelectable(true)
	item.SetLockPos(false)

	self, desc := TraversalCond_SelectableAndMoveable(item)
	if !self || !desc {
		t.Errorf("moveable: self=%v, desc=%v, want true,true", self, desc)
	}

	item.SetLockPos(true)
	self, desc = TraversalCond_SelectableAndMoveable(item)
	if self || !desc {
		t.Errorf("locked: self=%v, desc=%v, want false,true", self, desc)
	}
}

// --- intersectRect (unexported helper) ---

func TestIntersectRect(t *testing.T) {
	tests := []struct {
		name                       string
		x0, y0, w0, h0             float64
		x1, y1, w1, h1             float64
		wantX, wantY, wantW, wantH float64
	}{
		{
			"overlapping",
			0, 0, 10, 10,
			5, 5, 10, 10,
			5, 5, 5, 5,
		},
		{
			"contained",
			0, 0, 20, 20,
			5, 5, 5, 5,
			5, 5, 5, 5,
		},
		{
			"no overlap horizontal",
			0, 0, 5, 5,
			10, 0, 5, 5,
			10, 0, -5, 5,
		},
		{
			"identical",
			10, 20, 30, 40,
			10, 20, 30, 40,
			10, 20, 30, 40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, w, h := intersectRect(tt.x0, tt.y0, tt.w0, tt.h0, tt.x1, tt.y1, tt.w1, tt.h1)
			if x != tt.wantX || y != tt.wantY || w != tt.wantW || h != tt.wantH {
				t.Errorf("intersectRect = (%v,%v,%v,%v), want (%v,%v,%v,%v)",
					x, y, w, h, tt.wantX, tt.wantY, tt.wantW, tt.wantH)
			}
		})
	}
}

// --- FindItemAt ---

func TestFindItemAtSimple(t *testing.T) {
	parent := newTestItem()
	parent.SetBounds(0, 0, 200, 200)

	child := newTestItem()
	child.SetBounds(10, 10, 50, 50)
	child.SetParent(parent)

	// Hit child
	found := parent.FindItemAt(25, 25, nil)
	if found != IItem(child) {
		t.Error("should find child at (25,25)")
	}

	// Hit parent but not child
	found = parent.FindItemAt(100, 100, nil)
	if found != IItem(parent) {
		t.Error("should find parent at (100,100)")
	}

	// Miss entirely
	found = parent.FindItemAt(300, 300, nil)
	if found != nil {
		t.Error("should find nothing at (300,300)")
	}
}

func TestFindItemsAtMultiple(t *testing.T) {
	parent := newTestItem()
	parent.SetBounds(0, 0, 200, 200)

	c1 := newTestItem()
	c1.SetBounds(10, 10, 100, 100)
	c1.SetParent(parent)

	c2 := newTestItem()
	c2.SetBounds(50, 50, 100, 100)
	c2.SetParent(parent)

	// Point in overlap zone
	found := parent.FindItemsAt(60, 60, nil)
	if len(found) < 2 {
		t.Errorf("expected at least 2 items at overlap, got %d", len(found))
	}
}

// --- FindItemsInRect ---

func TestFindItemsInRect(t *testing.T) {
	parent := newTestItem()
	parent.SetBounds(0, 0, 500, 500)
	parent.SetSelectable(false) // so only children match

	c1 := newTestItem()
	c1.SetBounds(10, 10, 20, 20)
	c1.SetParent(parent)

	c2 := newTestItem()
	c2.SetBounds(100, 100, 20, 20)
	c2.SetParent(parent)

	c3 := newTestItem()
	c3.SetBounds(200, 200, 20, 20)
	c3.SetParent(parent)

	// Rect that fully contains c1 and c2 but not c3
	found := parent.FindItemsInRect(geom.Rect{X: 0, Y: 0, Width: 150, Height: 150}, nil)

	foundSet := make(map[IItem]bool)
	for _, f := range found {
		foundSet[f] = true
	}

	if !foundSet[IItem(c1)] {
		t.Error("c1 should be in result")
	}
	if !foundSet[IItem(c2)] {
		t.Error("c2 should be in result")
	}
	if foundSet[IItem(c3)] {
		t.Error("c3 should not be in result")
	}
}

// --- LocalCoord flag ---

func TestSetLocalCoord(t *testing.T) {
	item := newTestItem()
	if item.HasLocalCoord() {
		t.Error("localCoord should be false initially")
	}
	item.SetLocalCoord(true)
	if !item.HasLocalCoord() {
		t.Error("localCoord should be true")
	}
	item.SetLocalCoord(false)
	if item.HasLocalCoord() {
		t.Error("localCoord should be false again")
	}
}

// --- Children on nil item ---

func TestChildrenOnNilItem(t *testing.T) {
	var item *Item
	children := item.Children()
	if children != nil {
		t.Error("Children() on nil should return nil")
	}
}

// --- SizeHints ---

func TestSizeHints(t *testing.T) {
	item := newTestItem()
	item.SetBounds(0, 0, 120, 80)

	hints := item.SizeHints()
	if hints.Width != 120 || hints.Height != 80 {
		t.Errorf("SizeHints = {%v,%v}, want {120,80}", hints.Width, hints.Height)
	}
}

// --- DebugFlagsString ---

func TestDebugFlagsString(t *testing.T) {
	item := newTestItem()
	// default: visible=true, selectable=true, lockPos=false, lockSize=false
	flags := item.DebugFlagsString()
	if flags == "" {
		t.Error("flags should not be empty for visible+selectable item")
	}

	item.SetVisible(false)
	flags = item.DebugFlagsString()
	// Not visible -> no "V" flag
	for _, ch := range flags {
		if ch == 'V' {
			t.Error("hidden item should not have V flag")
			break
		}
	}
}

// --- SetParent same parent is no-op ---

func TestSetParentSameParentNoop(t *testing.T) {
	parent := newTestItem()
	child := newTestItem()

	child.SetParent(parent)
	child.SetParent(parent) // should not duplicate

	children := parent.Children()
	if len(children) != 1 {
		t.Errorf("expected 1 child after duplicate SetParent, got %d", len(children))
	}
}

// --- SetParent nil on orphan is no-op ---

func TestDetachOrphan(t *testing.T) {
	item := newTestItem()
	item.Detach() // should not panic
	if item.Parent() != nil {
		t.Error("orphan detach should keep parent nil")
	}
}
