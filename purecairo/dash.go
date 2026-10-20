package purecairo

import "math"

// dashedPath flattens curves in user space before splitting by arc length.
// Phase persists across vertices, restarts at each subpath, and odd patterns
// alternate over two repetitions, as in Cairo. Invalid patterns stay solid.
func dashedPath(path []pathSeg, dash Dash) ([]pathSeg, bool) {
	period := 0.0
	for _, d := range dash.Dashes {
		if d < 0 || math.IsNaN(d) || math.IsInf(d, 0) {
			return nil, false
		}
		period += d
	}
	if period <= 0 || math.IsInf(period, 0) || math.IsNaN(dash.Offset) || math.IsInf(dash.Offset, 0) {
		return nil, false
	}
	count := len(dash.Dashes)
	if count%2 != 0 {
		period *= 2
		count *= 2
	}
	if math.IsInf(period, 0) {
		return nil, false
	}
	var out []pathSeg
	index, left, drawing := 0, 0.0, false
	advance := func() {
		for range count {
			index = (index + 1) % count
			left = dash.Dashes[index%len(dash.Dashes)]
			if left > 0 {
				break
			}
		}
		if index%2 != 0 {
			drawing = false
		}
	}
	reset := func() {
		index = 0
		left = dash.Dashes[0]
		drawing = false
		offset := math.Mod(dash.Offset, period)
		if offset < 0 {
			offset += period
		}
		for left == 0 || offset >= left {
			offset -= left
			advance()
		}
		left -= offset
	}
	reset()
	x, y, sx, sy := 0.0, 0.0, 0.0, 0.0
	have := false
	steps := 0
	line := func(ex, ey float64) {
		dx, dy := ex-x, ey-y
		length := math.Hypot(dx, dy)
		if !have || length == 0 || math.IsNaN(length) || math.IsInf(length, 0) {
			x, y = ex, ey
			return
		}
		distance := 0.0
		for distance < length {
			// Bound work for adversarial sub-pixel dash periods; normal UI strokes
			// remain far below this limit. Never spin on a non-advancing float.
			if steps >= 65536 {
				break
			}
			steps++
			if left <= 0 {
				advance()
			}
			n := math.Min(left, length-distance)
			if n <= 0 || distance+n == distance {
				break
			}
			px, py := x+dx*(distance/length), y+dy*(distance/length)
			qx, qy := x+dx*((distance+n)/length), y+dy*((distance+n)/length)
			if index%2 == 0 {
				if !drawing {
					out = append(out, pathSeg{op: 'M', x1: px, y1: py})
					drawing = true
				}
				out = append(out, pathSeg{op: 'L', x1: qx, y1: qy})
			}
			distance += n
			left -= n
			if left <= 0 {
				advance()
			}
		}
		x, y = ex, ey
	}
	for _, p := range path {
		switch p.op {
		case 'M':
			reset()
			x, y, sx, sy, have = p.x1, p.y1, p.x1, p.y1, true
		case 'L':
			line(p.x1, p.y1)
		case 'C':
			x0, y0 := x, y
			for i := 1; i <= 16; i++ {
				t := float64(i) / 16
				u := 1 - t
				line(u*u*u*x0+3*u*u*t*p.x1+3*u*t*t*p.x2+t*t*t*p.x3, u*u*u*y0+3*u*u*t*p.y1+3*u*t*t*p.y2+t*t*t*p.y3)
			}
		case 'Z':
			line(sx, sy)
			have = false
		}
	}
	return out, true
}
