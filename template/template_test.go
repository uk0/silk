package template

import (
	"testing"

	"github.com/uk0/silk/driver"
)

// motor is a reusable device type: a run command (coil, RW), a measured speed
// (holding register, RO) and a fault status (coil, RO).
func motor() Template {
	return Template{
		Name: "Motor",
		Tags: []TagDef{
			{Name: "Run", AddressSuffix: ".DBX0.0", Type: driver.TypeBool, Order: driver.BigEndian, Access: driver.ReadWrite},
			{Name: "Speed", AddressSuffix: ".DBD2", Type: driver.TypeFloat32, Order: driver.BigEndian, Access: driver.ReadOnly},
			{Name: "Fault", AddressSuffix: ".DBX0.1", Type: driver.TypeBool, Order: driver.BigEndian, Access: driver.ReadOnly},
		},
	}
}

// TestInstantiate stamps the Motor template onto two instances and checks each
// generated point's Tag, Address and carried Type/Access.
func TestInstantiate(t *testing.T) {
	tpl := motor()

	cases := []struct {
		inst, base string
		want       []driver.TagPoint
	}{
		{"M1", "DB1", []driver.TagPoint{
			{Tag: "M1.Run", Address: "DB1.DBX0.0", Type: driver.TypeBool, Order: driver.BigEndian, Access: driver.ReadWrite},
			{Tag: "M1.Speed", Address: "DB1.DBD2", Type: driver.TypeFloat32, Order: driver.BigEndian, Access: driver.ReadOnly},
			{Tag: "M1.Fault", Address: "DB1.DBX0.1", Type: driver.TypeBool, Order: driver.BigEndian, Access: driver.ReadOnly},
		}},
		{"M2", "DB2", []driver.TagPoint{
			{Tag: "M2.Run", Address: "DB2.DBX0.0", Type: driver.TypeBool, Order: driver.BigEndian, Access: driver.ReadWrite},
			{Tag: "M2.Speed", Address: "DB2.DBD2", Type: driver.TypeFloat32, Order: driver.BigEndian, Access: driver.ReadOnly},
			{Tag: "M2.Fault", Address: "DB2.DBX0.1", Type: driver.TypeBool, Order: driver.BigEndian, Access: driver.ReadOnly},
		}},
	}

	for _, c := range cases {
		got := tpl.Instantiate(c.inst, c.base)
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %d points, want %d", c.inst, len(got), len(c.want))
		}
		for i, w := range c.want {
			if got[i] != w {
				t.Errorf("%s point %d = %+v, want %+v", c.inst, i, got[i], w)
			}
		}
	}
}

// TestInstantiateMany batch-stamps the template over three instances and checks
// the total count and per-instance tag/address prefixes.
func TestInstantiateMany(t *testing.T) {
	tpl := motor()
	insts := []Instance{
		{Name: "M1", AddressBase: "DB1"},
		{Name: "M2", AddressBase: "DB2"},
		{Name: "M3", AddressBase: "DB3"},
	}

	pts := tpl.InstantiateMany(insts)
	if want := len(insts) * len(tpl.Tags); len(pts) != want {
		t.Fatalf("got %d points, want %d", len(pts), want)
	}

	// Points are concatenated per instance in template order; the Run tag of each
	// instance sits at the block boundary.
	for i, in := range insts {
		run := pts[i*len(tpl.Tags)]
		if run.Tag != in.Name+".Run" {
			t.Errorf("block %d tag = %q, want %q", i, run.Tag, in.Name+".Run")
		}
		if run.Address != in.AddressBase+".DBX0.0" {
			t.Errorf("block %d address = %q, want %q", i, run.Address, in.AddressBase+".DBX0.0")
		}
	}
}

// TestValidate confirms a well-formed template passes, an empty name fails, and
// duplicate tag names are rejected.
func TestValidate(t *testing.T) {
	if err := motor().Validate(); err != nil {
		t.Errorf("motor template: unexpected error %v", err)
	}

	if err := (Template{Tags: []TagDef{{Name: "Run"}}}).Validate(); err == nil {
		t.Error("empty template name: want error, got nil")
	}

	dup := Template{Name: "Motor", Tags: []TagDef{
		{Name: "Run", Type: driver.TypeBool},
		{Name: "Run", Type: driver.TypeBool},
	}}
	if err := dup.Validate(); err == nil {
		t.Error("duplicate tag name: want error, got nil")
	}
}
