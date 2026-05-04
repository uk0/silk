package glui

import "testing"

func TestIntersectRectOverlap(t *testing.T) {
	x, y, w, h := intersectRect(0, 0, 100, 100, 50, 50, 100, 100)
	if x != 50 || y != 50 || w != 50 || h != 50 {
		t.Fatalf("got (%d,%d,%d,%d), want (50,50,50,50)", x, y, w, h)
	}
}

func TestIntersectRectContained(t *testing.T) {
	x, y, w, h := intersectRect(10, 10, 200, 200, 30, 30, 50, 50)
	if x != 30 || y != 30 || w != 50 || h != 50 {
		t.Fatalf("got (%d,%d,%d,%d), want (30,30,50,50)", x, y, w, h)
	}
}

func TestIntersectRectDisjoint(t *testing.T) {
	_, _, w, h := intersectRect(0, 0, 10, 10, 50, 50, 10, 10)
	if w != 0 || h != 0 {
		t.Fatalf("disjoint rects produced size (%d,%d), want (0,0)", w, h)
	}
}
