package hmi

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"unicode/utf8"
)

// Rect is an axis-aligned document-space rectangle, not a screen rectangle.
type Rect struct{ X, Y, Width, Height float64 }

// CloneProject returns an independent, validated document without I/O. Unlike a
// JSON roundtrip it preserves the exact scalar Go types of initial point values.
func CloneProject(p Project) (Project, error) {
	if _, err := p.JSON(); err != nil {
		return Project{}, err
	}
	q := p
	q.Points = slices.Clone(p.Points)
	q.Sources = slices.Clone(p.Sources)
	for i, s := range p.Sources {
		q.Sources[i].Headers = maps.Clone(s.Headers)
		q.Sources[i].Mappings = slices.Clone(s.Mappings)
		for j, m := range s.Mappings {
			q.Sources[i].Mappings[j].Steps = slices.Clone(m.Steps)
		}
	}
	q.Nodes = slices.Clone(p.Nodes)
	for i, n := range p.Nodes {
		q.Nodes[i].Steps = slices.Clone(n.Steps)
		q.Nodes[i].Colors = slices.Clone(n.Colors)
		q.Nodes[i].Bindings = slices.Clone(n.Bindings)
		for j, b := range n.Bindings {
			q.Nodes[i].Bindings[j].Steps = slices.Clone(b.Steps)
		}
	}
	q.Links = slices.Clone(p.Links)
	for i, l := range p.Links {
		q.Links[i].Bends = slices.Clone(l.Bends)
	}
	return q, nil
}
func editSelection(p Project, ids []string, minimum int) (map[string]bool, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	found := map[string]bool{}
	for _, n := range p.Nodes {
		found[n.ID] = true
	}
	selected := map[string]bool{}
	for _, id := range ids {
		if !found[id] || selected[id] {
			return nil, fmt.Errorf("hmi: unknown/duplicate selected node %q", id)
		}
		selected[id] = true
	}
	if len(selected) < minimum {
		return nil, fmt.Errorf("hmi: select at least %d nodes", minimum)
	}
	return selected, nil
}
func selectionBounds(p Project, selected map[string]bool, withBends bool) Rect {
	left, top, right, bottom := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	include := func(x, y, w, h float64) {
		left = min(left, x)
		top = min(top, y)
		right = max(right, x+w)
		bottom = max(bottom, y+h)
	}
	for _, n := range p.Nodes {
		if selected[n.ID] {
			include(n.X, n.Y, n.Width, n.Height)
		}
	}
	if withBends {
		for _, l := range p.Links {
			if selected[l.From.Node] && selected[l.To.Node] {
				for _, b := range l.Bends {
					include(b.X, b.Y, 0, 0)
				}
			}
		}
	}
	return Rect{left, top, right - left, bottom - top}
}

// SelectionBounds includes devices; TranslationBounds also includes waypoints
// of pipes wholly inside the selection, so a group move cannot lose a waypoint.
func SelectionBounds(p Project, ids []string) (Rect, error) {
	m, e := editSelection(p, ids, 1)
	if e != nil {
		return Rect{}, e
	}
	return selectionBounds(p, m, false), nil
}
func TranslationBounds(p Project, ids []string) (Rect, error) {
	m, e := editSelection(p, ids, 1)
	if e != nil {
		return Rect{}, e
	}
	return selectionBounds(p, m, true), nil
}

// ClampTranslation applies one common delta to the entire group. It never
// clamps each node separately (which would collapse relative spacing).
func ClampTranslation(width, height float64, b Rect, dx, dy float64) (XY, error) {
	if !finite(width) || !finite(height) || !finite(dx) || !finite(dy) || !finite(b.X) || !finite(b.Y) || !finite(b.Width) || !finite(b.Height) || b.X < 0 || b.Y < 0 || b.Width < 0 || b.Height < 0 || b.X+b.Width > width || b.Y+b.Height > height {
		return XY{}, fmt.Errorf("hmi: invalid translation bounds/delta")
	}
	return XY{max(-b.X, min(width-b.X-b.Width, dx)), max(-b.Y, min(height-b.Y-b.Height, dy))}, nil
}
func finishEdit(q Project) (Project, error) {
	if _, e := q.JSON(); e != nil {
		return Project{}, e
	}
	return q, nil
}

// MoveNodes moves selected nodes and internal pipe waypoints together. External
// pipes keep their authored waypoints and follow the moved endpoint ports.
func MoveNodes(p Project, ids []string, dx, dy float64) (Project, error) {
	m, e := editSelection(p, ids, 1)
	if e != nil {
		return Project{}, e
	}
	d, e := ClampTranslation(p.Width, p.Height, selectionBounds(p, m, true), dx, dy)
	if e != nil {
		return Project{}, e
	}
	q, e := CloneProject(p)
	if e != nil {
		return Project{}, e
	}
	for i := range q.Nodes {
		if m[q.Nodes[i].ID] {
			q.Nodes[i].X += d.X
			q.Nodes[i].Y += d.Y
		}
	}
	for i := range q.Links {
		l := &q.Links[i]
		if m[l.From.Node] && m[l.To.Node] {
			for j := range l.Bends {
				l.Bends[j].X += d.X
				l.Bends[j].Y += d.Y
			}
		}
	}
	return finishEdit(q)
}
func ResizeNode(p Project, id string, b Rect) (Project, error) {
	m, e := editSelection(p, []string{id}, 1)
	if e != nil {
		return Project{}, e
	}
	q, e := CloneProject(p)
	if e != nil {
		return Project{}, e
	}
	for i := range q.Nodes {
		if m[q.Nodes[i].ID] {
			q.Nodes[i].X = b.X
			q.Nodes[i].Y = b.Y
			q.Nodes[i].Width = b.Width
			q.Nodes[i].Height = b.Height
		}
	}
	return finishEdit(q)
}
func DeleteNodes(p Project, ids []string) (Project, error) {
	m, e := editSelection(p, ids, 1)
	if e != nil {
		return Project{}, e
	}
	q, e := CloneProject(p)
	if e != nil {
		return Project{}, e
	}
	q.Nodes = slices.DeleteFunc(q.Nodes, func(n Node) bool { return m[n.ID] })
	q.Links = slices.DeleteFunc(q.Links, func(l Link) bool { return m[l.From.Node] || m[l.To.Node] })
	return finishEdit(q)
}

// AlignNodes aligns against the selection's outer rectangle. Modes are left,
// right, top, bottom, horizontal-center and vertical-center.
func AlignNodes(p Project, ids []string, mode string) (Project, error) {
	m, e := editSelection(p, ids, 2)
	if e != nil {
		return Project{}, e
	}
	b := selectionBounds(p, m, false)
	q, e := CloneProject(p)
	if e != nil {
		return Project{}, e
	}
	for i := range q.Nodes {
		n := &q.Nodes[i]
		if !m[n.ID] {
			continue
		}
		switch mode {
		case "left":
			n.X = b.X
		case "right":
			n.X = b.X + b.Width - n.Width
		case "top":
			n.Y = b.Y
		case "bottom":
			n.Y = b.Y + b.Height - n.Height
		case "horizontal-center":
			n.X = b.X + (b.Width-n.Width)/2
		case "vertical-center":
			n.Y = b.Y + (b.Height-n.Height)/2
		default:
			return Project{}, fmt.Errorf("hmi: unknown alignment %q", mode)
		}
	}
	return finishEdit(q)
}

// DistributeNodes makes equal non-negative gaps, keeping the two end nodes
// fixed. Overfull selections fail rather than silently creating new overlaps.
func DistributeNodes(p Project, ids []string, horizontal bool) (Project, error) {
	m, e := editSelection(p, ids, 3)
	if e != nil {
		return Project{}, e
	}
	q, e := CloneProject(p)
	if e != nil {
		return Project{}, e
	}
	indexes := []int{}
	for i, n := range q.Nodes {
		if m[n.ID] {
			indexes = append(indexes, i)
		}
	}
	pos := func(n Node) float64 {
		if horizontal {
			return n.X
		}
		return n.Y
	}
	size := func(n Node) float64 {
		if horizontal {
			return n.Width
		}
		return n.Height
	}
	slices.SortStableFunc(indexes, func(a, b int) int {
		if pos(q.Nodes[a]) < pos(q.Nodes[b]) {
			return -1
		}
		if pos(q.Nodes[a]) > pos(q.Nodes[b]) {
			return 1
		}
		return 0
	})
	first, last := q.Nodes[indexes[0]], q.Nodes[indexes[len(indexes)-1]]
	total := 0.
	for _, i := range indexes {
		total += size(q.Nodes[i])
	}
	gap := (pos(last) + size(last) - pos(first) - total) / float64(len(indexes)-1)
	if gap < 0 {
		return Project{}, fmt.Errorf("hmi: insufficient room for equal gaps")
	}
	cursor := pos(first)
	for _, i := range indexes {
		if horizontal {
			q.Nodes[i].X = cursor
		} else {
			q.Nodes[i].Y = cursor
		}
		cursor += size(q.Nodes[i]) + gap
	}
	if horizontal {
		q.Nodes[indexes[len(indexes)-1]].X = last.X
	} else {
		q.Nodes[indexes[len(indexes)-1]].Y = last.Y
	}
	return finishEdit(q)
}

// ReorderNodes moves the group to the front/back, preserving relative order.
func ReorderNodes(p Project, ids []string, front bool) (Project, error) {
	m, e := editSelection(p, ids, 1)
	if e != nil {
		return Project{}, e
	}
	q, e := CloneProject(p)
	if e != nil {
		return Project{}, e
	}
	a, b := []Node{}, []Node{}
	for _, n := range q.Nodes {
		if m[n.ID] {
			b = append(b, n)
		} else {
			a = append(a, n)
		}
	}
	if front {
		q.Nodes = append(a, b...)
	} else {
		q.Nodes = append(b, a...)
	}
	return finishEdit(q)
}
func copyID(base string, used map[string]bool) string {
	for i := 1; ; i++ {
		suffix := fmt.Sprintf("_copy%d", i)
		for len(base)+len(suffix) > 256 {
			_, size := utf8.DecodeLastRuneInString(base)
			base = base[:len(base)-size]
		}
		id := base + suffix
		if !used[id] {
			used[id] = true
			return id
		}
	}
}

// DuplicateNodes copies a subgraph, including only internal pipes. Point names
// deliberately remain shared. All slices/maps are independent of the original.
func DuplicateNodes(p Project, ids []string, dx, dy float64) (Project, []string, error) {
	m, e := editSelection(p, ids, 1)
	if e != nil {
		return Project{}, nil, e
	}
	d, e := ClampTranslation(p.Width, p.Height, selectionBounds(p, m, true), dx, dy)
	if e != nil {
		return Project{}, nil, e
	}
	q, e := CloneProject(p)
	if e != nil {
		return Project{}, nil, e
	}
	copy, e := CloneProject(p)
	if e != nil {
		return Project{}, nil, e
	}
	used := map[string]bool{}
	for _, n := range p.Nodes {
		used[n.ID] = true
	}
	remap := map[string]string{}
	newIDs := []string{}
	for _, n := range copy.Nodes {
		if m[n.ID] {
			old := n.ID
			n.ID = copyID(n.ID, used)
			remap[old] = n.ID
			newIDs = append(newIDs, n.ID)
			n.X += d.X
			n.Y += d.Y
			q.Nodes = append(q.Nodes, n)
		}
	}
	used = map[string]bool{}
	for _, l := range p.Links {
		used[l.ID] = true
	}
	for _, l := range copy.Links {
		if m[l.From.Node] && m[l.To.Node] {
			l.ID = copyID(l.ID, used)
			l.From.Node = remap[l.From.Node]
			l.To.Node = remap[l.To.Node]
			for i := range l.Bends {
				l.Bends[i].X += d.X
				l.Bends[i].Y += d.Y
			}
			q.Links = append(q.Links, l)
		}
	}
	q, e = finishEdit(q)
	return q, newIDs, e
}

// EditLinkBend inserts (insert=true), replaces or deletes (point=nil) one
// waypoint. Invalid indexes/geometry reject the entire transaction.
func EditLinkBend(p Project, id string, index int, point *XY, insert bool) (Project, error) {
	q, e := CloneProject(p)
	if e != nil {
		return Project{}, e
	}
	for i := range q.Links {
		l := &q.Links[i]
		if l.ID != id {
			continue
		}
		if index < 0 || index > len(l.Bends) || !insert && index == len(l.Bends) || insert && point == nil {
			return Project{}, fmt.Errorf("hmi: invalid bend edit")
		}
		if insert {
			l.Bends = slices.Insert(l.Bends, index, *point)
		} else if point == nil {
			l.Bends = slices.Delete(l.Bends, index, index+1)
		} else {
			l.Bends[index] = *point
		}
		return finishEdit(q)
	}
	return Project{}, fmt.Errorf("hmi: unknown pipe %q", id)
}
