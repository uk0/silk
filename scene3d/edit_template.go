package scene3d

import (
	"context"
	"fmt"
)

// InstantiateTemplate inserts a complete device, including its inert bindings,
// as one undoable edit. Use InstantiateTemplates to place a whole line atomically.
func (e *EditSession) InstantiateTemplate(ctx context.Context, template *Template, options InstanceOptions) (*TemplateInstance, error) {
	instances, err := e.InstantiateTemplates(ctx, template, []InstanceOptions{options})
	if err != nil {
		return nil, err
	}
	return instances[0], nil
}

// InstantiateTemplates preserves existing authored IDs, order and matrix encoding.
// New resources share the compiled template's immutable geometry across placements
// and history. Mesh IDs are allocated without overwriting existing resources.
// Bindings are merged and validated with the destination program, including tag
// type conflicts. Warnings are deduplicated and retained in the saved document.
//
// Invalid placement, cancellation or any combined document/render/binding budget
// failure leaves revision, scene, document and both history stacks unchanged.
// An empty batch does not change history. No tag, file, goroutine or driver is
// activated. Session and options are single-owner; Template is immutable/shareable.
func (e *EditSession) InstantiateTemplates(ctx context.Context, template *Template, options []InstanceOptions) ([]*TemplateInstance, error) {
	if err := e.ready(ctx); err != nil {
		return nil, err
	}
	if template == nil || template.ids == nil {
		return nil, ErrNode
	}
	if len(options) == 0 {
		return []*TemplateInstance{}, nil
	}
	if len(template.bindings) > 0 && len(options) > (MaxBindings-len(e.state.document.Bindings))/len(template.bindings) {
		return nil, ErrLimit
	}
	// InstantiateBatch only publishes to its destination after validation. A shallow
	// Scene header is safe here: it never mutates the old node map/order/snapshot.
	placedScene := *e.scene
	instances, err := template.InstantiateBatch(ctx, &placedScene, options)
	if err != nil {
		return nil, err
	}
	d := e.candidate()
	d.Meshes = append([]DocumentMesh(nil), d.Meshes...)
	cache := make(map[string]*Mesh, len(e.state.meshes))
	ids := make(map[*Mesh]string, len(e.state.meshes))
	for _, resource := range d.Meshes {
		mesh := e.state.meshes[resource.ID]
		cache[resource.ID] = mesh
		if _, found := ids[mesh]; !found {
			ids[mesh] = resource.ID
		}
	}
	meshNumber := 0
	for _, id := range placedScene.order[len(e.scene.order):] {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n := placedScene.nodes[id]
		meshID := ""
		if n.Mesh != nil {
			meshID = ids[n.Mesh]
			if meshID == "" {
				for {
					meshNumber++
					meshID = fmt.Sprintf("template-mesh/%d", meshNumber)
					if _, found := cache[meshID]; !found {
						break
					}
				}
				cache[meshID], ids[n.Mesh] = n.Mesh, meshID
				// These arrays are private to immutable Mesh; Document() copies on export.
				d.Meshes = append(d.Meshes, DocumentMesh{ID: meshID, Vertices: n.Mesh.vertices, Indices: n.Mesh.indices})
			}
		}
		d.Nodes = append(d.Nodes, DocumentNode{ID: n.ID, Parent: n.Parent, Name: n.Name, Mesh: meshID, Transform: append([]float64(nil), n.Transform[:]...), Color: n.Color, Hidden: n.Hidden})
	}
	for _, instance := range instances {
		d.Bindings = append(d.Bindings, instance.bindings...)
	}
	warnings := make(map[string]bool, len(d.Warnings)+len(template.warnings))
	for _, warning := range d.Warnings {
		warnings[warning] = true
	}
	d.Warnings = append([]string(nil), d.Warnings...)
	for _, warning := range template.warnings {
		if !warnings[warning] {
			d.Warnings = append(d.Warnings, warning)
			warnings[warning] = true
		}
	}
	if err := e.commit(ctx, d, cache); err != nil {
		return nil, err
	}
	return instances, nil
}

// Warnings returns an independent copy of retained import diagnostics.
func (e *EditSession) Warnings() []string {
	if e == nil || e.closed {
		return nil
	}
	return append([]string(nil), e.state.document.Warnings...)
}
