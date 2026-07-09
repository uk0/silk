// Package template provides 结构化组态 device templates: define a device type
// once — its tags, their address offsets, data types, byte orders and access
// modes — then stamp it across many instances in one batch. This is FameView's
// (杰控) signature 结构化数据批次处理: a "Motor" template with Run/Speed/Fault is
// authored once and instantiated over hundreds of motors, each carrying its own
// tag prefix and address base, without re-entering every point by hand. The
// output is a flat []driver.TagPoint ready to feed a driver.Poller — the 万点
// batch-config win.
package template

import (
	"fmt"

	"github.com/uk0/silk/driver"
)

// TagDef is one tag inside a template. AddressSuffix is the point's offset from
// an instance's address base (e.g. ".DBX0.0" or "1"); Type/Order/Access carry
// straight through to every stamped driver.TagPoint.
type TagDef struct {
	Name          string           // tag name relative to the instance (e.g. "Run")
	AddressSuffix string           // address offset appended to the instance base
	Type          driver.DataType  // value type
	Order         driver.ByteOrder // register/byte order for multi-byte types
	Access        driver.AccessMode
}

// Template is a reusable device type — e.g. a "Motor" with Run(bool),
// Speed(float) and Fault(bool) — authored once and stamped across instances.
type Template struct {
	Name string
	Tags []TagDef
}

// Instance is one placement of a template: a tag-name prefix and the address
// base every TagDef offset is applied to.
type Instance struct {
	Name        string // tag prefix, e.g. "M1" -> tags "M1.Run", "M1.Speed"
	AddressBase string // address base, e.g. "DB1" -> address "DB1.DBX0.0"
}

// Instantiate stamps the template onto one instance: each TagDef becomes a
// driver.TagPoint with Tag = instanceName+"."+def.Name and
// Address = addressBase+def.AddressSuffix, carrying Type/Order/Access.
func (t Template) Instantiate(instanceName, addressBase string) []driver.TagPoint {
	pts := make([]driver.TagPoint, 0, len(t.Tags))
	for _, def := range t.Tags {
		pts = append(pts, driver.TagPoint{
			Tag:     instanceName + "." + def.Name,
			Address: addressBase + def.AddressSuffix,
			Type:    def.Type,
			Order:   def.Order,
			Access:  def.Access,
		})
	}
	return pts
}

// InstantiateMany batch-stamps the template across every instance and returns
// the concatenated points — the 万点 batch-config path.
func (t Template) InstantiateMany(insts []Instance) []driver.TagPoint {
	pts := make([]driver.TagPoint, 0, len(insts)*len(t.Tags))
	for _, in := range insts {
		pts = append(pts, t.Instantiate(in.Name, in.AddressBase)...)
	}
	return pts
}

// Validate reports whether the template is well-formed: a non-empty name and no
// duplicate tag names (a duplicate would collide on Tag once stamped).
func (t Template) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("template: empty name")
	}
	seen := make(map[string]bool, len(t.Tags))
	for _, def := range t.Tags {
		if seen[def.Name] {
			return fmt.Errorf("template %q: duplicate tag name %q", t.Name, def.Name)
		}
		seen[def.Name] = true
	}
	return nil
}
