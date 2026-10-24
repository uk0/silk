package gui

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"slices"
	"testing"
)

func diffTestLines(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("line %08d", i)
	}
	return out
}
func TestLineDiffSparseAllocationIsNotQuadratic(t *testing.T) {
	old := diffTestLines(2000)
	next := append([]string(nil), old...)
	next[1000] = "changed"
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	rows := lineDiff(old, next)
	runtime.ReadMemStats(&after)
	if len(rows) != 2000 || rows[1000].Status != DiffModified {
		t.Fatal("invalid sparse diff")
	}
	if bytes := after.TotalAlloc - before.TotalAlloc; bytes > 8<<20 {
		t.Fatalf("sparse 2000-line comparison allocated %d bytes; must not allocate an N*M table", bytes)
	}
}
func BenchmarkLineDiffSparse(b *testing.B) {
	old := diffTestLines(2000)
	next := append([]string(nil), old...)
	next[1000] = "changed"
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		lineDiff(old, next)
	}
}

// Independent dynamic-programming oracle for small inputs. Only the optimal
// LCS length is compared: repeated equal lines can have multiple valid optima.
func diffOracleLCS(a, b []string) int {
	prev := make([]int, len(b)+1)
	for _, x := range a {
		diagonal := 0
		for j, y := range b {
			up := prev[j+1]
			if x == y {
				prev[j+1] = diagonal + 1
			} else {
				prev[j+1] = max(prev[j], up)
			}
			diagonal = up
		}
	}
	return prev[len(b)]
}
func checkExactRows(t testing.TB, a, b []string, rows []DiffRow) {
	t.Helper()
	ai, bi, same := 0, 0, 0
	for _, r := range rows {
		switch r.Status {
		case DiffSame, DiffModified:
			if ai >= len(a) || bi >= len(b) || r.OldLine != a[ai] || r.NewLine != b[bi] {
				t.Fatalf("bad aligned row %+v at %d,%d", r, ai, bi)
			}
			if r.Status == DiffSame {
				if r.OldLine != r.NewLine {
					t.Fatal("false match")
				}
				same++
			}
			ai++
			bi++
		case DiffRemoved:
			if ai >= len(a) || r.OldLine != a[ai] || r.NewLine != "" {
				t.Fatal("bad removal")
			}
			ai++
		case DiffAdded:
			if bi >= len(b) || r.NewLine != b[bi] || r.OldLine != "" {
				t.Fatal("bad addition")
			}
			bi++
		default:
			t.Fatal("unexpected status")
		}
	}
	if ai != len(a) || bi != len(b) {
		t.Fatal("input lost or reordered")
	}
	if want := diffOracleLCS(a, b); same != want {
		t.Fatalf("nonminimal diff: a=%q b=%q got matches=%d want=%d rows=%+v", a, b, same, want, rows)
	}
}
func TestLineDiffExhaustiveOptimalAndLossless(t *testing.T) {
	seq := [][]string{nil}
	for n := 1; n <= 6; n++ {
		for bits := 0; bits < 1<<n; bits++ {
			s := make([]string, n)
			for i := range s {
				if bits&(1<<i) == 0 {
					s[i] = "a"
				} else {
					s[i] = "b"
				}
			}
			seq = append(seq, s)
		}
	}
	for _, a := range seq {
		for _, b := range seq {
			rows, err := lineDiffContext(context.Background(), a, b)
			if err != nil {
				t.Fatalf("%q vs %q: %v", a, b, err)
			}
			checkExactRows(t, a, b, rows)
		}
	}
}
func TestLineDiffUnicodeAndUnequalLengths(t *testing.T) {
	random := rand.New(rand.NewSource(4389))
	pool := []string{"", "same", "\r", "中文", "😀", "a\x00b"}
	for i := 0; i < 1500; i++ {
		a, b := make([]string, random.Intn(120)), make([]string, random.Intn(120))
		for j := range a {
			a[j] = pool[random.Intn(len(pool))]
		}
		for j := range b {
			b[j] = pool[random.Intn(len(pool))]
		}
		rows, err := lineDiffContext(context.Background(), a, b)
		if err != nil {
			t.Fatal(err)
		}
		checkExactRows(t, a, b, rows)
	}
}
func TestLineDiffLargeSparseAndReplacement(t *testing.T) {
	a := diffTestLines(100000)
	b := append([]string(nil), a...)
	b[1] = "changed first"
	b[len(b)-2] = "changed last"
	rows, err := lineDiffContext(context.Background(), a, b)
	if err != nil || len(rows) != len(a) {
		t.Fatalf("large sparse: %v rows=%d", err, len(rows))
	}
	count := 0
	for _, r := range rows {
		if r.Status == DiffModified {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("changed=%d", count)
	}
	for i := range b {
		b[i] = "different " + b[i]
	}
	rows, err = lineDiffContext(context.Background(), a, b)
	if err != nil || len(rows) != len(a) {
		t.Fatalf("large replacement: %v rows=%d", err, len(rows))
	}
	for _, r := range rows {
		if r.Status != DiffModified {
			t.Fatal("disjoint blocks were not paired")
		}
	}
}

type diffCancelChecks struct {
	context.Context
	remaining int
}

func (c *diffCancelChecks) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return nil
}
func TestLineDiffCancellationDuringTraversal(t *testing.T) {
	a := diffTestLines(10000)
	b := append([]string(nil), a...)
	slices.Reverse(b)
	for _, pair := range [][2][]string{{a, a}, {a, b}, {nil, b}} {
		ctx := &diffCancelChecks{Context: context.Background(), remaining: 10}
		rows, err := lineDiffContext(ctx, pair[0], pair[1])
		if !errors.Is(err, context.Canceled) || rows != nil {
			t.Fatalf("partial result escaped cancellation: %v rows=%d", err, len(rows))
		}
	}
}
func FuzzLineDiffExact(f *testing.F) {
	f.Add([]byte{0, 1, 2, 2, 1}, []byte{2, 0, 1, 2})
	f.Add([]byte{}, []byte{1})
	f.Fuzz(func(t *testing.T, aa, bb []byte) {
		if len(aa) > 64 || len(bb) > 64 {
			t.Skip()
		}
		a, b := make([]string, len(aa)), make([]string, len(bb))
		for i, x := range aa {
			a[i] = string(rune('a' + x%7))
		}
		for i, x := range bb {
			b[i] = string(rune('a' + x%7))
		}
		rows, err := lineDiffContext(context.Background(), a, b)
		if err != nil {
			t.Fatal(err)
		}
		checkExactRows(t, a, b, rows)
	})
}
