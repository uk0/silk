package scene3d

import (
	"context"
	"errors"
)

var ErrEditConflict = errors.New("scene3d: edit session changed during gesture")

// TranslationEdit captures an authored node's local transform and parent basis.
// Transform computes a preview without mutating the session, document, scene,
// bindings or history. Apply it only to an independent preview Scene. Commit
// validates/publishes the final translation as one undoable edit; Cancel publishes
// nothing. Every operation checks the captured session revision, including after
// undo/redo back to apparently identical contents. All methods are single-owner.
type TranslationEdit struct {
	session        *EditSession
	revision       uint64
	node           DocumentNode
	local, inverse Mat4
}

// BeginTranslation supports groups and hidden nodes. World-axis displacement is
// converted through the parent's inverse linear basis, preserving the node's
// rotation, scale, reflection and shear. A singular parent cannot be translated
// unambiguously in world space and is rejected without starting an edit.
func (e *EditSession) BeginTranslation(ctx context.Context, id string) (*TranslationEdit, error) {
	if err := e.ready(ctx); err != nil {
		return nil, err
	}
	n, ok := e.scene.nodes[id]
	if !ok {
		return nil, ErrNode
	}
	inverse := Identity()
	if n.Parent != "" {
		parent, err := e.worldTransform(ctx, n.Parent)
		if err != nil {
			return nil, err
		}
		// Only the linear basis is needed. Inverting a huge translation can
		// overflow even when the displacement conversion is well-conditioned.
		parent[12], parent[13], parent[14] = 0, 0, 0
		inverse, err = parent.InverseAffine()
		if err != nil {
			return nil, err
		}
	}
	for _, node := range e.state.document.Nodes {
		if node.ID == id {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			node.Transform = append([]float64(nil), node.Transform...)
			return &TranslationEdit{session: e, revision: e.revision, node: node, local: n.Transform, inverse: inverse}, nil
		}
	}
	return nil, ErrNode
}

func (t *TranslationEdit) ready(ctx context.Context) error {
	if t == nil || t.session == nil {
		return ErrEditClosed
	}
	if err := t.session.ready(ctx); err != nil {
		return err
	}
	if t.revision != t.session.Revision() {
		return ErrEditConflict
	}
	return nil
}

// Transform returns a local matrix for an absolute world displacement from the
// gesture start. It is O(1), allocation-free and never rebuilds a scene/history.
// Renderers still validate the preview; Commit validates the complete graph.
func (t *TranslationEdit) Transform(delta Vec3) (Mat4, error) {
	if err := t.ready(context.Background()); err != nil {
		return Mat4{}, err
	}
	if !delta.valid() {
		return Mat4{}, ErrGeometry
	}
	if delta == (Vec3{}) {
		return t.local, nil
	}
	p := t.inverse
	// Do not subtract transformed points: a large parent translation would
	// introduce catastrophic cancellation into a small drag displacement.
	d := Vec3{p[0]*delta.X + p[4]*delta.Y + p[8]*delta.Z, p[1]*delta.X + p[5]*delta.Y + p[9]*delta.Z, p[2]*delta.X + p[6]*delta.Y + p[10]*delta.Z}
	m := t.local
	m[12] += d.X
	m[13] += d.Y
	m[14] += d.Z
	if !m.affine() {
		return Mat4{}, ErrGeometry
	}
	return m, nil
}

// Commit publishes at most one edit and retires this gesture on success. An
// effective no-op preserves the original transform encoding and redo history.
// Errors (including cancellation/conflict) do not change history or retire the
// gesture; callers may correct the input or Cancel it explicitly.
func (t *TranslationEdit) Commit(ctx context.Context, delta Vec3) error {
	if err := t.ready(ctx); err != nil {
		return err
	}
	m, err := t.Transform(delta)
	if err != nil {
		return err
	}
	if m != t.local {
		node := t.node
		node.Transform = append([]float64(nil), m[:]...)
		if err := t.session.UpdateNode(ctx, node); err != nil {
			return err
		}
	}
	t.Cancel()
	return nil
}

// Cancel releases the captured session and node without changing authored data.
func (t *TranslationEdit) Cancel() {
	if t != nil {
		*t = TranslationEdit{}
	}
}
