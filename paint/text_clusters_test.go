package paint

import "testing"

func TestClusterBoundaryAndWrapPolicy(t *testing.T) {
	l := TextLayout{Clusters: []TextCluster{{1, 5}, {7, 9}}}
	for _, tc := range []struct{ start, end, want int }{{0, 0, 0}, {0, 1, 1}, {0, 3, 1}, {1, 2, 5}, {5, 8, 7}, {7, 8, 9}, {0, 20, 20}} {
		if got := l.WrapEnd(tc.start, tc.end); got != tc.want {
			t.Fatalf("%+v got %d", tc, got)
		}
	}
	for _, p := range []int{0, 1, 5, 6, 7, 9, 20} {
		if l.Boundary(p, false) != p || l.Boundary(p, true) != p {
			t.Fatal("ordinary boundary moved")
		}
	}
}

func BenchmarkClusterASCIIProbe(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NeedsClusterLayout("func main() { println(12345) }")
	}
}
