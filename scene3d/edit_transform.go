package scene3d

import "context"

// TransformEdit captures an authored node for world-axis rotation/scaling about
// its own origin. It preserves parent reflection/shear and the exact local
// translation, without decomposing the authored matrix into lossy Euler/TRS.
// Preview is O(1), allocation-free, and does not mutate the session. Like
// TranslationEdit, it is single-owner and invalidated by any session revision.
type TransformEdit struct {
	base        *TranslationEdit
	worldLinear Mat4
}

func (e *EditSession) BeginTransform(ctx context.Context, id string) (*TransformEdit, error) {
	base, err := e.BeginTranslation(ctx, id)
	if err != nil {
		return nil, err
	}
	parent := Identity()
	if base.node.Parent != "" {
		parent, err = e.worldTransform(ctx, base.node.Parent)
		if err != nil {
			base.Cancel()
			return nil, err
		}
	}
	parent[12], parent[13], parent[14] = 0, 0, 0
	local := base.local
	local[12], local[13], local[14] = 0, 0, 0
	world := parent.Mul(local)
	if !world.affine() {
		base.Cancel()
		return nil, ErrGeometry
	}
	if err := ctx.Err(); err != nil {
		base.Cancel()
		return nil, err
	}
	return &TransformEdit{base: base, worldLinear: world}, nil
}

func (t *TransformEdit) ready(ctx context.Context) error {
	if t == nil {
		return ErrEditClosed
	}
	return t.base.ready(ctx)
}

// Transform applies an absolute world linear operation from the gesture start,
// not an incremental matrix. delta must be affine with zero translation. The
// node origin stays fixed; descendants transform normally. Identity is an exact
// no-op (including omitted identity encoding and redo history). Singular parents
// are rejected at BeginTransform; this API never silently drops shear.
func (t *TransformEdit) Transform(delta Mat4) (Mat4, error) {
	if err := t.ready(context.Background()); err != nil {
		return Mat4{}, err
	}
	if !delta.affine() || delta[12] != 0 || delta[13] != 0 || delta[14] != 0 {
		return Mat4{}, ErrGeometry
	}
	if delta == Identity() {
		return t.base.local, nil
	}
	m := t.base.inverse.Mul(delta.Mul(t.worldLinear))
	m[12], m[13], m[14] = t.base.local[12], t.base.local[13], t.base.local[14]
	if !m.affine() {
		return Mat4{}, ErrGeometry
	}
	return m, nil
}

// Commit publishes one validated edit, then retires the gesture. Errors retain
// the gesture for correction/cancellation and leave scene/history unchanged.
func (t *TransformEdit) Commit(ctx context.Context, delta Mat4) error {
	if err := t.ready(ctx); err != nil {
		return err
	}
	m, err := t.Transform(delta)
	if err != nil {
		return err
	}
	if m != t.base.local {
		node := t.base.node
		node.Transform = append([]float64(nil), m[:]...)
		if err := t.base.session.UpdateNode(ctx, node); err != nil {
			return err
		}
	}
	t.Cancel()
	return nil
}
func (t *TransformEdit) Cancel() {
	if t != nil {
		t.base.Cancel()
		*t = TransformEdit{}
	}
}
