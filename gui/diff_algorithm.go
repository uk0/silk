package gui

import (
	"context"
	"fmt"
)

// Exact linear-space Myers bisection, not a heuristic/size-based replacement.
// Two frontiers are reused by all subproblems; matches use an explicit stack.
// Reference: E. Myers, An O(ND) Difference Algorithm and Its Variations, 1986,
// section 4b, https://neil.fraser.name/writing/diff/myers.pdf.
// Time remains O((N+M)*D) in the worst case. Cancellation is cooperative, so
// interactive hosts should use DiffView.SetTextsAsync rather than blocking UI.
type diffMatch struct{ old, next, count int }
type diffRange struct {
	a0, a1, b0, b1 int
	equal          bool
}

type diffAlgorithm struct {
	ctx              context.Context
	a, b             []string
	forward, reverse []int
	steps            uint
}

func (d *diffAlgorithm) check() error {
	d.steps++
	if d.steps&255 == 0 {
		return d.ctx.Err()
	}
	return nil
}
func (d *diffAlgorithm) common(a, b []string) (bool, error) {
	if len(a) > len(b) {
		a, b = b, a
	}
	set := make(map[string]struct{}, len(a))
	for _, line := range a {
		if err := d.check(); err != nil {
			return false, err
		}
		set[line] = struct{}{}
	}
	for _, line := range b {
		if err := d.check(); err != nil {
			return false, err
		}
		if _, ok := set[line]; ok {
			return true, nil
		}
	}
	return false, nil
}

// nextFrontier extends one edit, then all equal diagonals, within the edit grid.
// Clipping candidates to the grid avoids invalid overlap near very unequal ends.
func (d *diffAlgorithm) nextFrontier(v []int, offset, k, n, m int, a, b []string, backward bool) (int, error) {
	x := -1
	if from := v[offset+k+1]; from >= 0 && from <= n && from-k >= 0 && from-k <= m {
		x = from
	}
	if from := v[offset+k-1]; from >= 0 && from+1 <= n && from+1-k >= 0 && from+1-k <= m && from+1 > x {
		x = from + 1
	}
	if x < 0 {
		return -1, nil
	}
	y := x - k
	for x < n && y < m {
		ai, bi := x, y
		if backward {
			ai, bi = n-x-1, m-y-1
		}
		if a[ai] != b[bi] {
			break
		}
		x++
		y++
		if err := d.check(); err != nil {
			return 0, err
		}
	}
	return x, nil
}
func (d *diffAlgorithm) bisect(a, b []string) (int, int, error) {
	n, m := len(a), len(b)
	maxD := (n + m + 1) / 2
	offset := maxD + 1
	size := 2*maxD + 3
	if len(d.forward) < size {
		d.forward = make([]int, size)
		d.reverse = make([]int, size)
	}
	f, r := d.forward[:size], d.reverse[:size]
	for i := range f {
		f[i], r[i] = -1, -1
	}
	f[offset+1], r[offset+1] = 0, 0
	delta := n - m
	odd := delta%2 != 0
	for depth := 0; depth <= maxD; depth++ {
		if err := d.ctx.Err(); err != nil {
			return 0, 0, err
		}
		for k := -depth; k <= depth; k += 2 {
			if err := d.check(); err != nil {
				return 0, 0, err
			}
			x, err := d.nextFrontier(f, offset, k, n, m, a, b, false)
			if err != nil {
				return 0, 0, err
			}
			f[offset+k] = x
			opposite := delta - k
			if odd && x >= 0 && opposite >= -(depth-1) && opposite <= depth-1 && r[offset+opposite] >= 0 && x+r[offset+opposite] >= n {
				return x, x - k, nil
			}
		}
		for k := -depth; k <= depth; k += 2 {
			if err := d.check(); err != nil {
				return 0, 0, err
			}
			x, err := d.nextFrontier(r, offset, k, n, m, a, b, true)
			if err != nil {
				return 0, 0, err
			}
			r[offset+k] = x
			opposite := delta - k
			if !odd && x >= 0 && opposite >= -depth && opposite <= depth && f[offset+opposite] >= 0 && f[offset+opposite]+x >= n {
				fx := f[offset+opposite]
				return fx, fx - opposite, nil
			}
		}
	}
	return 0, 0, fmt.Errorf("diff: could not find exact middle path")
}
func (d *diffAlgorithm) matches() ([]diffMatch, error) {
	stack := []diffRange{{0, len(d.a), 0, len(d.b), false}}
	var matches []diffMatch
	add := func(a, b, n int) {
		if n == 0 {
			return
		}
		if len(matches) > 0 {
			last := &matches[len(matches)-1]
			if last.old+last.count == a && last.next+last.count == b {
				last.count += n
				return
			}
		}
		matches = append(matches, diffMatch{a, b, n})
	}
	for len(stack) > 0 {
		if err := d.ctx.Err(); err != nil {
			return nil, err
		}
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if s.equal {
			add(s.a0, s.b0, s.a1-s.a0)
			continue
		}
		prefix := 0
		for s.a0+prefix < s.a1 && s.b0+prefix < s.b1 && d.a[s.a0+prefix] == d.b[s.b0+prefix] {
			prefix++
			if err := d.check(); err != nil {
				return nil, err
			}
		}
		add(s.a0, s.b0, prefix)
		s.a0 += prefix
		s.b0 += prefix
		suffix := 0
		for s.a0 < s.a1-suffix && s.b0 < s.b1-suffix && d.a[s.a1-suffix-1] == d.b[s.b1-suffix-1] {
			suffix++
			if err := d.check(); err != nil {
				return nil, err
			}
		}
		if suffix > 0 {
			stack = append(stack, diffRange{s.a1 - suffix, s.a1, s.b1 - suffix, s.b1, true})
			s.a1 -= suffix
			s.b1 -= suffix
		}
		n, m := s.a1-s.a0, s.b1-s.b0
		if n == 0 || m == 0 {
			continue
		}
		if n == 1 || m == 1 {
			found := false
			for a := s.a0; a < s.a1 && !found; a++ {
				for b := s.b0; b < s.b1; b++ {
					if err := d.check(); err != nil {
						return nil, err
					}
					if d.a[a] == d.b[b] {
						add(a, b, 1)
						found = true
						break
					}
				}
			}
			continue
		}
		// Completely replaced blocks have no possible match; prove that cheaply
		// instead of searching every edit depth. This is exact, not a fallback.
		if n > 64 || m > 64 {
			common, err := d.common(d.a[s.a0:s.a1], d.b[s.b0:s.b1])
			if err != nil {
				return nil, err
			}
			if !common {
				continue
			}
		}
		x, y, err := d.bisect(d.a[s.a0:s.a1], d.b[s.b0:s.b1])
		if err != nil {
			return nil, err
		}
		if (x == 0 && y == 0) || (x == n && y == m) {
			return nil, fmt.Errorf("diff: middle path made no progress (%d,%d)", n, m)
		}
		stack = append(stack, diffRange{s.a0 + x, s.a1, s.b0 + y, s.b1, false}, diffRange{s.a0, s.a0 + x, s.b0, s.b0 + y, false})
	}
	return matches, nil
}

func lineDiffContext(ctx context.Context, oldLines, newLines []string) ([]DiffRow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(oldLines) == 0 && len(newLines) == 0 {
		return nil, nil
	}
	d := diffAlgorithm{ctx: ctx, a: oldLines, b: newLines}
	matches, err := d.matches()
	if err != nil {
		return nil, err
	}
	rows := make([]DiffRow, 0, max(len(oldLines), len(newLines)))
	oi, ni := 0, 0
	flush := func(oe, ne int) error {
		for oi < oe || ni < ne {
			if err := d.check(); err != nil {
				return err
			}
			row := DiffRow{}
			switch {
			case oi < oe && ni < ne:
				row = DiffRow{oldLines[oi], newLines[ni], DiffModified}
				oi++
				ni++
			case oi < oe:
				row = DiffRow{OldLine: oldLines[oi], Status: DiffRemoved}
				oi++
			default:
				row = DiffRow{NewLine: newLines[ni], Status: DiffAdded}
				ni++
			}
			rows = append(rows, row)
		}
		return nil
	}
	for _, match := range matches {
		if err := flush(match.old, match.next); err != nil {
			return nil, err
		}
		for i := 0; i < match.count; i++ {
			if err := d.check(); err != nil {
				return nil, err
			}
			rows = append(rows, DiffRow{oldLines[oi], newLines[ni], DiffSame})
			oi++
			ni++
		}
	}
	if err := flush(len(oldLines), len(newLines)); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

// lineDiff preserves the synchronous SDK setter contract. UI hosts comparing
// large snapshots should opt into SetTextsAsync so worst-case D cannot stall input.
func lineDiff(oldLines, newLines []string) []DiffRow {
	rows, err := lineDiffContext(context.Background(), oldLines, newLines)
	if err != nil {
		panic(err)
	} // Background cannot cancel; an algorithm invariant failed.
	return rows
}
