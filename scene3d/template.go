package scene3d

import (
	"context"
	"fmt"
)

// Template is a validated, immutable device/assembly definition. Compile it once
// and share it across goroutines; each destination Scene still has one owner.
// Placed nodes share the template's immutable meshes, not their mutable state.
// This is scene-data instancing, not hardware-instanced GPU drawing.
type Template struct {
	nodes    []Node
	ids      map[string]struct{}
	bindings []Binding
	tags     map[string]bool
	warnings []string
}

// InstanceOptions describes one wrapper node and its device tag mapping.
// ID is required. Source nodes retain their hierarchy under ID + "/" + sourceID.
// Parent must already exist in the destination (or an earlier item in a batch).
// The zero Transform means identity. Hidden hides the entire placed subtree.
// TagMap overrides named source tags; otherwise TagPrefix is prepended verbatim.
// Map keys not present in the template are errors, not silently ignored typos.
// No point is created or subscribed by placement.
type InstanceOptions struct {
	ID, Parent, Name string
	Transform        Mat4
	Hidden           bool
	TagPrefix        string
	TagMap           map[string]string
}

// TemplateInstance describes a successful placement. It does not own a live
// subscription or keep the destination Scene alive. Removing its RootID from the
// scene removes the subtree; this descriptor then remains only metadata.
type TemplateInstance struct {
	template *Template
	root     string
	bindings []Binding
}

// NewTemplate validates and compiles an authored document. It copies mutable
// document data and geometry once; subsequent placements never copy mesh arrays.
// For a running scene use BindingSet.CaptureDocument to preserve authored values
// rather than baking live rotations/scales into the template a second time.
func NewTemplate(ctx context.Context, document *Document) (*Template, error) {
	s, err := document.Build(ctx)
	if err != nil {
		return nil, err
	}
	t := &Template{
		nodes: s.Nodes(), ids: make(map[string]struct{}, len(s.nodes)),
		bindings: append([]Binding(nil), document.Bindings...),
		tags:     make(map[string]bool), warnings: append([]string(nil), document.Warnings...),
	}
	for _, n := range t.nodes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		t.ids[n.ID] = struct{}{}
	}
	for _, b := range t.bindings {
		t.tags[b.Tag] = bindingChannel(b.Property) >= 9
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return t, nil
}

// NodeCount excludes the extra wrapper node created by each placement.
func (t *Template) NodeCount() int {
	if t == nil {
		return 0
	}
	return len(t.nodes)
}

// Warnings returns a copy of the original import diagnostics.
func (t *Template) Warnings() []string {
	if t == nil {
		return nil
	}
	return append([]string(nil), t.warnings...)
}

func (i *TemplateInstance) RootID() string {
	if i == nil {
		return ""
	}
	return i.root
}

// NodeID maps an exact source ID, not a display name or path prefix.
func (i *TemplateInstance) NodeID(sourceID string) (string, bool) {
	if i == nil || i.template == nil {
		return "", false
	}
	if _, ok := i.template.ids[sourceID]; !ok {
		return "", false
	}
	return i.root + "/" + sourceID, true
}

// Bindings returns independent, inert specifications using placed node IDs and
// mapped tag names. Combine the desired instances' bindings explicitly before
// installing a BindingSet or calling SceneView.SetSceneBindings.
func (i *TemplateInstance) Bindings() []Binding {
	if i == nil {
		return nil
	}
	return append([]Binding(nil), i.bindings...)
}

// Instantiate atomically places one device. See InstantiateBatch for guarantees
// and use the batch operation instead of repeated calls to assemble a large line.
func (t *Template) Instantiate(ctx context.Context, destination *Scene, options InstanceOptions) (*TemplateInstance, error) {
	instances, err := t.InstantiateBatch(ctx, destination, []InstanceOptions{options})
	if err != nil {
		return nil, err
	}
	return instances[0], nil
}

// InstantiateBatch places all devices in one transaction and increments the
// destination revision once. Validation, cancellation, duplicate IDs, invalid
// tag mappings, world-transform overflow or snapshot-budget failure leave the
// destination, its revision and its cached snapshot exactly unchanged.
//
// The candidate copies node metadata once (O(existing + placed nodes)); immutable
// meshes are shared, including between destinations. Snapshot budgets still count
// every visible instance. The returned bindings must fit one BindingSet and may
// not map a numeric and boolean source tag to the same destination tag. Existing
// external binding programs are not owned or reconfigured by this operation.
//
// No file I/O, UI dispatch or subscription occurs here. Do not mutate destination
// or options (including TagMap) concurrently. An empty batch is a no-op.
func (t *Template) InstantiateBatch(ctx context.Context, destination *Scene, options []InstanceOptions) ([]*TemplateInstance, error) {
	if t == nil || t.ids == nil || destination == nil || ctx == nil {
		return nil, ErrNode
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(options) == 0 {
		return []*TemplateInstance{}, nil
	}
	// Divide before multiplying so even adversarial slice lengths cannot wrap.
	nodesPerInstance := len(t.nodes) + 1
	if len(destination.nodes) > MaxNodes || len(options) > (MaxNodes-len(destination.nodes))/nodesPerInstance {
		return nil, ErrLimit
	}
	if len(t.bindings) != 0 && len(options) > MaxBindings/len(t.bindings) {
		return nil, ErrLimit
	}
	count := len(destination.nodes) + len(options)*nodesPerInstance
	candidate := &Scene{nodes: make(map[string]Node, count), order: make([]string, len(destination.order), count)}
	copy(candidate.order, destination.order)
	for id, node := range destination.nodes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		candidate.nodes[id] = node
	}
	instances := make([]*TemplateInstance, 0, len(options))
	// Structural/channel validation was done once by NewTemplate. Only the
	// mapped tag type/name/budget can change; do not allocate and discard a full
	// live BindingSet (plans and scratch buffers) just to validate placement.
	mappedTypes := make(map[string]bool, len(options)*len(t.tags))
	bindingTextBytes := 0
	for _, placement := range options {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !validSceneText(placement.ID, 256, false) || !validSceneText(placement.Parent, 256, true) || !validSceneText(placement.Name, 4096, true) {
			return nil, ErrNode
		}
		if !validSceneText(placement.TagPrefix, 512, true) || len(placement.TagMap) > len(t.tags) {
			return nil, ErrBinding
		}
		for tag, mapped := range placement.TagMap {
			if _, ok := t.tags[tag]; !ok || !validSceneText(mapped, 512, false) {
				return nil, fmt.Errorf("%w: template tag mapping %q", ErrBinding, tag)
			}
		}
		if err := candidate.Add(Node{ID: placement.ID, Parent: placement.Parent, Name: placement.Name, Transform: placement.Transform, Hidden: placement.Hidden}); err != nil {
			return nil, err
		}
		prefix := placement.ID + "/"
		for _, original := range t.nodes {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			node := original
			node.ID = prefix + original.ID
			if original.Parent == "" {
				node.Parent = placement.ID
			} else {
				node.Parent = prefix + original.Parent
			}
			if err := candidate.Add(node); err != nil {
				return nil, err
			}
		}
		bindings := make([]Binding, 0, len(t.bindings))
		for _, original := range t.bindings {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			b := original
			b.Node = prefix + original.Node
			if tag, ok := placement.TagMap[b.Tag]; ok {
				b.Tag = tag
			} else {
				b.Tag = placement.TagPrefix + b.Tag
			}
			bindingTextBytes += len(b.Node) + len(b.Tag) + len(b.Property)
			if bindingTextBytes > maxDocumentTextBytes {
				return nil, ErrLimit
			}
			if !validSceneText(b.Tag, 512, false) {
				return nil, ErrBinding
			}
			boolean := t.tags[original.Tag]
			if previous, ok := mappedTypes[b.Tag]; ok && previous != boolean {
				return nil, fmt.Errorf("%w: mixed boolean/numeric tag %q", ErrBinding, b.Tag)
			}
			mappedTypes[b.Tag] = boolean
			bindings = append(bindings, b)
		}
		// Each descriptor owns only its own bindings, not the entire batch.
		instances = append(instances, &TemplateInstance{template: t, root: placement.ID, bindings: bindings})
	}
	if _, err := candidate.Snapshot(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// No fallible operation remains; old snapshots still own immutable draw data.
	candidate.revision = destination.revision + 1
	*destination = *candidate
	return instances, nil
}
