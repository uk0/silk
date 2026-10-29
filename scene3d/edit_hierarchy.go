package scene3d

import (
	"context"
	"fmt"
	"math"
)

// subtree uses the validated topological order, not recursive traversal. It
// works with hidden nodes, groups and input documents in arbitrary node order.
func (e *EditSession) subtree(ctx context.Context, id string) (map[string]bool, error) {
	if _, ok := e.scene.nodes[id]; !ok {
		return nil, ErrNode
	}
	found := map[string]bool{id: true}
	for _, key := range e.scene.order {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if found[e.scene.nodes[key].Parent] {
			found[key] = true
		}
	}
	return found, nil
}

// RemoveSubtree atomically removes a node, all descendants and their bindings.
// Shared and unreferenced mesh resources are retained; undo restores exact
// authored order and metadata. No external tags or subscriptions are changed.
func (e *EditSession) RemoveSubtree(ctx context.Context, id string) error {
	if err := e.ready(ctx); err != nil {
		return err
	}
	removed, err := e.subtree(ctx, id)
	if err != nil {
		return err
	}
	d := e.candidate()
	d.Nodes = d.Nodes[:0]
	for _, n := range e.state.document.Nodes {
		if !removed[n.ID] {
			d.Nodes = append(d.Nodes, n)
		}
	}
	d.Bindings = d.Bindings[:0]
	for _, b := range e.state.document.Bindings {
		if !removed[b.Node] {
			d.Bindings = append(d.Bindings, b)
		}
	}
	return e.commit(ctx, d, e.state.meshes)
}

// DuplicateOptions describes an inert sibling copy. Offset is a local-space
// translation delta on the copied root; descendant transforms are unchanged.
// Bindings are NOT copied by default. If enabled, unmapped tags retain their
// original names (both instances will then reference the same process points).
// Tags may remap copied bindings only; unused mappings are rejected as mistakes.
type DuplicateOptions struct {
	RootID, RootName string
	Offset           Vec3
	CopyBindings     bool
	Tags             map[string]string
}

// DuplicateSubtree shares immutable mesh resources and returns old -> new IDs.
// Descendant IDs are RootID + "/node-N", numbered in authored document order.
// Any collision, budget, hierarchy or binding error rejects the whole edit.
func (e *EditSession) DuplicateSubtree(ctx context.Context, id string, o DuplicateOptions) (map[string]string, error) {
	if err := e.ready(ctx); err != nil {
		return nil, err
	}
	if !validSceneText(o.RootID, 256, false) || !validSceneText(o.RootName, 4096, true) || !o.Offset.valid() {
		return nil, ErrNode
	}
	if len(o.Tags) > MaxBindings {
		return nil, ErrLimit
	}
	if !o.CopyBindings && len(o.Tags) > 0 {
		return nil, ErrBinding
	}
	subtree, err := e.subtree(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(e.state.document.Nodes)+len(subtree) > MaxNodes {
		return nil, ErrLimit
	}
	mapping := make(map[string]string, len(subtree))
	mapping[id] = o.RootID
	seq := 0
	for _, n := range e.state.document.Nodes {
		if subtree[n.ID] && n.ID != id {
			seq++
			mapping[n.ID] = fmt.Sprintf("%s/node-%d", o.RootID, seq)
		}
	}
	for _, newID := range mapping {
		if !validSceneText(newID, 256, false) {
			return nil, ErrNode
		}
		if _, exists := e.scene.nodes[newID]; exists {
			return nil, ErrNode
		}
	}
	d := e.candidate()
	for _, n := range e.state.document.Nodes {
		if !subtree[n.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		oldID := n.ID
		n.ID = mapping[oldID]
		if oldID == id {
			if o.RootName != "" {
				n.Name = o.RootName
			}
			if o.Offset != (Vec3{}) {
				m := Identity()
				if len(n.Transform) == 16 {
					copy(m[:], n.Transform)
				}
				m[12] += o.Offset.X
				m[13] += o.Offset.Y
				m[14] += o.Offset.Z
				n.Transform = append([]float64(nil), m[:]...)
			}
		} else {
			n.Parent = mapping[n.Parent]
		}
		d.Nodes = append(d.Nodes, n)
	}
	used := make(map[string]bool, len(o.Tags))
	if o.CopyBindings {
		for _, b := range e.state.document.Bindings {
			if !subtree[b.Node] {
				continue
			}
			if len(d.Bindings) >= MaxBindings {
				return nil, ErrLimit
			}
			b.Node = mapping[b.Node]
			if tag, ok := o.Tags[b.Tag]; ok {
				used[b.Tag] = true
				b.Tag = tag
			}
			d.Bindings = append(d.Bindings, b)
		}
	}
	if len(used) != len(o.Tags) {
		return nil, ErrBinding
	}
	if err = e.commit(ctx, d, e.state.meshes); err != nil {
		return nil, err
	}
	return mapping, nil
}

// WorldTransform returns the authored world transform even for hidden nodes and
// groups. Live binding values are intentionally not part of an edit session.
func (e *EditSession) WorldTransform(id string) (Mat4, error) {
	if err := e.ready(context.Background()); err != nil {
		return Mat4{}, err
	}
	return e.worldTransform(context.Background(), id)
}
func (e *EditSession) worldTransform(ctx context.Context, id string) (Mat4, error) {
	if _, ok := e.scene.nodes[id]; !ok {
		return Mat4{}, ErrNode
	}
	chain := make([]Mat4, 0, 16)
	for id != "" {
		if err := ctx.Err(); err != nil {
			return Mat4{}, err
		}
		n := e.scene.nodes[id]
		chain = append(chain, n.Transform)
		id = n.Parent
	}
	m := Identity()
	for i := len(chain) - 1; i >= 0; i-- {
		m = m.Mul(chain[i])
		if !m.affine() {
			return Mat4{}, ErrNode
		}
	}
	return m, nil
}

// Reparent moves a subtree in one undoable transaction. keepWorld preserves its
// authored world transform within 1e-9 relative/absolute element tolerance.
// Singular, ill-conditioned or numerically unsafe new parents are rejected in
// this mode rather than silently moving geometry. keepWorld=false keeps the
// exact authored local transform and accepts singular (finite affine) parents.
func (e *EditSession) Reparent(ctx context.Context, id, parent string, keepWorld bool) error {
	if err := e.ready(ctx); err != nil {
		return err
	}
	n, ok := e.scene.nodes[id]
	if !ok {
		return ErrNode
	}
	if n.Parent == parent {
		return nil
	}
	if parent != "" {
		if _, ok := e.scene.nodes[parent]; !ok {
			return ErrNode
		}
		for p := parent; p != ""; p = e.scene.nodes[p].Parent {
			if p == id {
				return ErrNode
			}
		}
	}
	d := e.candidate()
	for i, node := range d.Nodes {
		if node.ID != id {
			continue
		}
		node.Parent = parent
		if keepWorld {
			world, err := e.worldTransform(ctx, id)
			if err != nil {
				return err
			}
			pw := Identity()
			if parent != "" {
				pw, err = e.worldTransform(ctx, parent)
				if err != nil {
					return err
				}
			}
			inv, err := pw.InverseAffine()
			if err != nil {
				return err
			}
			local := inv.Mul(world)
			actual := pw.Mul(local)
			if !local.affine() || !actual.affine() {
				return ErrNode
			}
			for j, v := range world {
				if math.Abs(actual[j]-v) > 1e-9*math.Max(1, math.Abs(v)) {
					return ErrNode
				}
			}
			node.Transform = append([]float64(nil), local[:]...)
		}
		d.Nodes[i] = node
		return e.commit(ctx, d, e.state.meshes)
	}
	return ErrNode
}
