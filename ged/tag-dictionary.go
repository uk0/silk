package ged

import (
	"sort"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/graph"
)

// A 组态 screen used to acquire its tags by accident: the only record of a tag
// was the free text typed into a widget's 数据绑定 row, and the generated app
// called GetOrCreate at bind time — creating it with an empty core.Meta, i.e.
// no unit, no engineering range and no alarm limits. TagDecl is the other
// direction: the design declares the tag, with the metadata the runtime needs,
// and the widget binding merely refers to it.
type TagDecl struct {
	Name string
	Meta core.Meta
}

// normalizeTagDict trims names and drops what a dictionary cannot hold: the
// nameless row 添加 starts with (it declares nothing until the user types a
// name) and a repeat of a name already declared — the registry keys on the name
// and GetOrCreate applies Meta only on creation, so a second declaration could
// never take effect at runtime. First one wins, order is otherwise preserved
// because the table shows the dictionary in exactly this order.
func normalizeTagDict(decls []TagDecl) []TagDecl {
	out := make([]TagDecl, 0, len(decls))
	seen := make(map[string]bool, len(decls))
	for _, d := range decls {
		d.Name = strings.TrimSpace(d.Name)
		if d.Name == "" || seen[d.Name] {
			continue
		}
		seen[d.Name] = true
		out = append(out, d)
	}
	return out
}

// undeclaredTags returns the tags widgets are bound to that the dictionary does
// not declare, deduplicated and sorted so the dialog's 未声明 column reads the
// same on every open.
//
// This is a hint, never a rule: scada.BindScreen resolves a widget's tag through
// GetOrCreate, so an undeclared tag still binds (with an empty Meta) and the
// design still saves. All the dictionary buys such a tag is its metadata.
func undeclaredTags(decls []TagDecl, referenced []string) []string {
	declared := make(map[string]bool, len(decls))
	for _, d := range decls {
		declared[strings.TrimSpace(d.Name)] = true
	}
	seen := make(map[string]bool, len(referenced))
	var out []string
	for _, name := range referenced {
		name = strings.TrimSpace(name)
		if name == "" || declared[name] || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// sceneTagNames returns the tag every widget in the scene is bound to, in scene
// order. It reads the TagName() seam — the same one scada.BindScreen resolves
// against and the code generator collects — and recurses into layout containers,
// since a widget nested in a VBox is bound exactly like a top-level one.
func sceneTagNames(scene *GedScene) []string {
	var out []string
	var walk func(items []graph.IItem)
	walk = func(items []graph.IItem) {
		for _, item := range items {
			fake, ok := item.(*FakeWidget)
			if !ok {
				continue
			}
			if tw, ok := fake.Widget().(interface{ TagName() string }); ok {
				if name := tw.TagName(); name != "" {
					out = append(out, name)
				}
			}
			if fake.HasChildren() {
				walk(fake.Children())
			}
		}
	}
	walk(scene.Children())
	return out
}

// writeTagDict appends the dictionary to a design document as a "tags" block:
// one child per declaration, holding the name as its value and each core.Meta
// field as an attribute — the shape SaveDesign already uses for widgets (value =
// identity, attrs = fields). The block is omitted entirely when the dictionary is
// empty, so designs without one keep the exact file shape they had before the
// dictionary existed.
func writeTagDict(doc *core.TDoc, decls []TagDecl) {
	if len(decls) == 0 {
		return
	}
	block := core.NewTDoc()
	block.SetKey("tags")
	for _, d := range decls {
		child := core.NewTDoc()
		child.SetValue(d.Name)
		child.WriteAttr("unit", d.Meta.Unit)
		child.WriteAttr("min", d.Meta.Min)
		child.WriteAttr("max", d.Meta.Max)
		child.WriteAttr("lolo", d.Meta.LoLo)
		child.WriteAttr("lo", d.Meta.Lo)
		child.WriteAttr("hi", d.Meta.Hi)
		child.WriteAttr("hihi", d.Meta.HiHi)
		child.WriteAttr("desc", d.Meta.Desc)
		block.AddChild(child)
	}
	doc.AddChild(block)
}

// readTagDict parses what writeTagDict wrote. A document with no block yields an
// empty dictionary, and a nameless entry is skipped rather than failing the load
// — the same tolerance loadChildWidgets applies to an unknown widget factory.
func readTagDict(doc *core.TDoc) []TagDecl {
	block := doc.ChildByKey("tags", false)
	if block == nil {
		return nil
	}
	var decls []TagDecl
	for _, child := range block.Childdren() {
		var d TagDecl
		child.Value(&d.Name)
		if strings.TrimSpace(d.Name) == "" {
			continue
		}
		child.ReadAttr("unit", &d.Meta.Unit)
		child.ReadAttr("min", &d.Meta.Min)
		child.ReadAttr("max", &d.Meta.Max)
		child.ReadAttr("lolo", &d.Meta.LoLo)
		child.ReadAttr("lo", &d.Meta.Lo)
		child.ReadAttr("hi", &d.Meta.Hi)
		child.ReadAttr("hihi", &d.Meta.HiHi)
		child.ReadAttr("desc", &d.Meta.Desc)
		decls = append(decls, d)
	}
	return normalizeTagDict(decls)
}
