package graph

import (
	"reflect"
	"testing"
)

func TestMoveSiblingToIndexPreservesParentAndLifecycle(t *testing.T) {
	for n := 2; n <= 8; n++ {
		for from := 0; from < n; from++ {
			for dest := 0; dest < n; dest++ {
				s := &SceneItem{}
				s.Init(s)
				items := make([]IItem, n)
				for i := range items {
					items[i] = NewRectItem()
					items[i].SetParent(s)
				}
				events := 0
				s.SigItemAttached(func(_ interface{}, _, _ IItem) { events++ })
				s.SigItemDetached(func(_ interface{}, _, _ IItem) { events++ })
				item := items[from]
				want := make([]IItem, 0, n)
				for i, c := range items {
					if i != from {
						want = append(want, c)
					}
				}
				want = append(want, nil)
				copy(want[dest+1:], want[dest:])
				want[dest] = item
				if ok := MoveSiblingToIndex(item, dest); ok != (from != dest) {
					t.Fatalf("unexpected outcome: n=%d %d→%d = %v", n, from, dest, ok)
				}
				if !reflect.DeepEqual(s.Children(), want) || item.Parent() != s || item.IndexInParent() != dest || events != 0 {
					t.Fatalf("corrupt order/lifecycle: n=%d %d→%d", n, from, dest)
				}
				if MoveSiblingToIndex(item, n) || MoveSiblingToIndex(item, -1) {
					t.Fatal("accepted invalid sibling slot")
				}
				MoveSiblingToIndex(item, from)
				if !reflect.DeepEqual(s.Children(), items) || events != 0 {
					t.Fatal("reverse changed tree identity or slot")
				}
			}
		}
	}
}
