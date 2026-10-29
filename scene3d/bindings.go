package scene3d

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"unicode/utf8"
)

var ErrBinding = errors.New("scene3d: invalid binding")
var ErrSample = errors.New("scene3d: invalid binding sample")

// BindingProperty names authored local-space channels. Rotation is a relative
// XYZ rotation in radians; scale is a relative factor (1 preserves the model).
// Both post-multiply the authored matrix. Position overrides its translation.
type BindingProperty string

const (
	PositionX   BindingProperty = "position.x"
	PositionY   BindingProperty = "position.y"
	PositionZ   BindingProperty = "position.z"
	RotationX   BindingProperty = "rotation.x"
	RotationY   BindingProperty = "rotation.y"
	RotationZ   BindingProperty = "rotation.z"
	ScaleX      BindingProperty = "scale.x"
	ScaleY      BindingProperty = "scale.y"
	ScaleZ      BindingProperty = "scale.z"
	Visibility  BindingProperty = "visible"
	AlarmColor  BindingProperty = "alarm-color"
	MaxBindings                 = MaxNodes * 11
)

// Binding is inert configuration; it never looks up tags or subscribes to data.
// Numeric channels use value*Gain+Offset. Set Gain=1 for an identity mapping;
// Gain=0 deliberately represents a constant. Boolean channels require zero
// Gain/Offset. AlarmColor selects one of two explicit opaque colors.
type Binding struct {
	Node       string          `json:"node"`
	Tag        string          `json:"tag"`
	Property   BindingProperty `json:"property"`
	Gain       float64         `json:"gain,omitempty"`
	Offset     float64         `json:"offset,omitempty"`
	FalseColor color.RGBA      `json:"falseColor,omitzero"`
	TrueColor  color.RGBA      `json:"trueColor,omitzero"`
}

var bindingProperties = [...]BindingProperty{PositionX, PositionY, PositionZ, RotationX, RotationY, RotationZ, ScaleX, ScaleY, ScaleZ, Visibility, AlarmColor}

func bindingChannel(p BindingProperty) int {
	for i, x := range bindingProperties {
		if x == p {
			return i
		}
	}
	return -1
}
func validSceneText(s string, max int, empty bool) bool {
	return (empty || s != "") && len(s) <= max && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}

type boundNode struct {
	base                      Node
	position, rotation, scale Vec3
	hidden                    bool
	color                     color.RGBA
	mask                      uint16
}
type boundChannel struct {
	node, channel int
	spec          Binding
}
type boundUpdate struct {
	node int
	mask uint16
}
type tagPlan struct {
	boolean  bool
	channels []boundChannel
	updates  []boundUpdate
}

// BindingSet is a compiled, single-owner binding program. Apply reuses its
// scratch storage and does not allocate for valid scalar samples. It is not
// thread-safe: a GUI host must coalesce/marshal samples onto its UI queue.
// Bound transform channels own the authored local matrix until recompiled;
// direct transform edits during a binding session are not a new authored base.
// Input quality and lossless alarm/event history belong to the host, not here.
type BindingSet struct {
	scene          *Scene
	specs          []Binding
	tags           []string
	plans          map[string]*tagPlan
	nodes, scratch []boundNode
	staged         []Node
}

func NewBindingSet(s *Scene, bindings []Binding) (*BindingSet, error) {
	if s == nil {
		return nil, ErrNode
	}
	if len(bindings) > MaxBindings {
		return nil, ErrLimit
	}
	b := &BindingSet{scene: s, specs: append([]Binding(nil), bindings...), plans: make(map[string]*tagPlan)}
	indexes := make(map[string]int)
	masks := make(map[string]uint16)
	updateIndexes := make(map[string]map[int]int)
	textBytes := 0
	for _, spec := range bindings {
		textBytes += len(spec.Node) + len(spec.Tag) + len(spec.Property)
		if textBytes > maxDocumentTextBytes {
			return nil, ErrLimit
		}
		ch := bindingChannel(spec.Property)
		if ch < 0 || !validSceneText(spec.Node, 256, false) || !validSceneText(spec.Tag, 512, false) || !finite(spec.Gain) || !finite(spec.Offset) {
			return nil, ErrBinding
		}
		n, ok := s.Node(spec.Node)
		if !ok {
			return nil, fmt.Errorf("%w: unknown node %q", ErrBinding, spec.Node)
		}
		if ch >= 9 && (spec.Gain != 0 || spec.Offset != 0) {
			return nil, ErrBinding
		}
		if ch == 10 {
			if n.Mesh == nil || spec.FalseColor.A != 255 || spec.TrueColor.A != 255 {
				return nil, ErrBinding
			}
		} else if spec.FalseColor != (color.RGBA{}) || spec.TrueColor != (color.RGBA{}) {
			return nil, ErrBinding
		}
		bit := uint16(1) << ch
		if masks[spec.Node]&bit != 0 {
			return nil, fmt.Errorf("%w: duplicate channel for %q", ErrBinding, spec.Node)
		}
		masks[spec.Node] |= bit
		index, ok := indexes[spec.Node]
		if !ok {
			index = len(b.nodes)
			indexes[spec.Node] = index
			b.nodes = append(b.nodes, boundNode{base: n, position: Vec3{n.Transform[12], n.Transform[13], n.Transform[14]}, scale: Vec3{1, 1, 1}, hidden: n.Hidden, color: n.Color})
		}
		b.nodes[index].mask |= bit
		plan, ok := b.plans[spec.Tag]
		if !ok {
			plan = &tagPlan{boolean: ch >= 9}
			b.plans[spec.Tag] = plan
			b.tags = append(b.tags, spec.Tag)
			updateIndexes[spec.Tag] = make(map[int]int)
		} else if plan.boolean != (ch >= 9) {
			return nil, fmt.Errorf("%w: mixed boolean/numeric tag %q", ErrBinding, spec.Tag)
		}
		plan.channels = append(plan.channels, boundChannel{index, ch, spec})
		if u, ok := updateIndexes[spec.Tag][index]; ok {
			plan.updates[u].mask |= bit
		} else {
			updateIndexes[spec.Tag][index] = len(plan.updates)
			plan.updates = append(plan.updates, boundUpdate{index, bit})
		}
	}
	sort.Strings(b.tags)
	b.scratch = make([]boundNode, len(b.nodes))
	b.staged = make([]Node, len(b.nodes))
	return b, nil
}
func (b *BindingSet) Tags() []string      { return append([]string(nil), b.tags...) }
func (b *BindingSet) Bindings() []Binding { return append([]Binding(nil), b.specs...) }
func scalarNumber(v any) (float64, bool) {
	var f float64
	switch x := v.(type) {
	case float64:
		f = x
	case float32:
		f = float64(x)
	case int:
		f = float64(x)
	case int8:
		f = float64(x)
	case int16:
		f = float64(x)
	case int32:
		f = float64(x)
	case int64:
		f = float64(x)
	case uint:
		f = float64(x)
	case uint8:
		f = float64(x)
	case uint16:
		f = float64(x)
	case uint32:
		f = float64(x)
	case uint64:
		f = float64(x)
	case json.Number:
		var e error
		f, e = x.Float64()
		if e != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return f, finite(f)
}
func setComponent(v *Vec3, axis int, n float64) {
	switch axis {
	case 0:
		v.X = n
	case 1:
		v.Y = n
	case 2:
		v.Z = n
	}
}

// Apply updates all destinations for a tag atomically after validating the
// sample and local matrices. Invalid input/removed nodes leave state unchanged.
// Unknown tags are errors. Hierarchical world-space overflow is still reported
// by Scene.Snapshot, just as for direct Scene.SetTransform calls.
func (b *BindingSet) Apply(tag string, value any) (bool, error) {
	if b == nil || b.scene == nil {
		return false, ErrBinding
	}
	plan, ok := b.plans[tag]
	if !ok {
		return false, ErrBinding
	}
	var number float64
	var boolean bool
	if plan.boolean {
		boolean, ok = value.(bool)
	} else {
		number, ok = scalarNumber(value)
	}
	if !ok {
		return false, ErrSample
	}
	for _, u := range plan.updates {
		i := u.node
		n, exists := b.scene.Node(b.nodes[i].base.ID)
		if !exists {
			return false, ErrNode
		}
		b.scratch[i] = b.nodes[i]
		b.staged[i] = n
	}
	for _, ch := range plan.channels {
		state := &b.scratch[ch.node]
		if ch.channel < 9 {
			v := number*ch.spec.Gain + ch.spec.Offset
			if !finite(v) {
				return false, ErrSample
			}
			switch {
			case ch.channel < 3:
				setComponent(&state.position, ch.channel, v)
			case ch.channel < 6:
				setComponent(&state.rotation, ch.channel-3, v)
			default:
				setComponent(&state.scale, ch.channel-6, v)
			}
		} else if ch.channel == 9 {
			state.hidden = !boolean
		} else if boolean {
			state.color = ch.spec.TrueColor
		} else {
			state.color = ch.spec.FalseColor
		}
	}
	for _, u := range plan.updates {
		state := &b.scratch[u.node]
		n := &b.staged[u.node]
		if u.mask&0x1ff != 0 {
			m := state.base.Transform
			if state.rotation != (Vec3{}) {
				m = m.Mul(RotationXYZ(state.rotation))
			}
			if state.scale != (Vec3{1, 1, 1}) {
				m = m.Mul(Scaling(state.scale))
			}
			m[12], m[13], m[14] = state.position.X, state.position.Y, state.position.Z
			if !m.affine() {
				return false, ErrSample
			}
			n.Transform = m
		}
		if u.mask&(1<<9) != 0 {
			n.Hidden = state.hidden
		}
		if u.mask&(1<<10) != 0 {
			n.Color = state.color
		}
	}
	before := b.scene.Revision()
	for _, u := range plan.updates {
		b.nodes[u.node] = b.scratch[u.node]
		b.scene.update(b.staged[u.node])
	}
	return b.scene.Revision() != before, nil
}

// authoredScene copies node metadata only; immutable meshes remain shared.
// Removed nodes are skipped so a corrected binding set can recover after edits.
func (b *BindingSet) authoredScene() (*Scene, error) {
	if b == nil || b.scene == nil {
		return nil, ErrBinding
	}
	s := NewScene()
	for _, n := range b.scene.Nodes() {
		if err := s.Add(n); err != nil {
			return nil, err
		}
	}
	for _, state := range b.nodes {
		n, ok := s.Node(state.base.ID)
		if !ok {
			continue
		}
		if state.mask&0x1ff != 0 {
			n.Transform = state.base.Transform
		}
		if state.mask&(1<<9) != 0 {
			n.Hidden = state.base.Hidden
		}
		if state.mask&(1<<10) != 0 {
			n.Color = state.base.Color
		}
		s.update(n)
	}
	return s, nil
}

// Reconfigure validates a replacement against authored baselines, then restores
// the old bound channels and returns the new program targeting the SAME Scene.
// This avoids accumulating a live rotation/scale each time a registry/config is
// replaced. Errors leave both scene and program intact. Retire the old program
// after success; as with Apply, this method is single-owner, not thread-safe.
func (b *BindingSet) Reconfigure(bindings []Binding) (*BindingSet, error) {
	authored, err := b.authoredScene()
	if err != nil {
		return nil, err
	}
	next, err := NewBindingSet(authored, bindings)
	if err != nil {
		return nil, err
	}
	for _, n := range authored.Nodes() {
		b.scene.update(n)
	}
	next.scene = b.scene
	return next, nil
}

// CaptureDocument saves authored baselines rather than live tag-driven values,
// preventing cumulative rotation/scale on a save/reload/rebind cycle. Unbound
// properties retain their current values. No file or subscription is opened.
func (b *BindingSet) CaptureDocument() (*Document, error) {
	s, err := b.authoredScene()
	if err != nil {
		return nil, err
	}
	return CaptureDocument(s, b.specs)
}
