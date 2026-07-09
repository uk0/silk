// Package device is the designer-facing bridge between silk's UI and the
// field-device layer (package driver). DeviceComponent is a placeable widget
// whose properties configure a Modbus TCP or S7 connection and its tag points;
// at runtime it builds a driver.Poller that streams the device into a
// core.TagDB, from which silk's existing bindings drive the screen. Keeping it
// in its own package means only apps that need device I/O pull the protocol
// dependencies.
package device

import (
	"fmt"
	"strings"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/driver"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

// Protocol selects the field-bus a DeviceComponent speaks.
type Protocol int

const (
	ProtoModbusTCP Protocol = iota
	ProtoS7
)

func parseProtocol(s string) Protocol {
	if strings.EqualFold(strings.TrimSpace(s), "s7") {
		return ProtoS7
	}
	return ProtoModbusTCP
}

func (p Protocol) String() string {
	if p == ProtoS7 {
		return "s7"
	}
	return "modbus"
}

// DeviceComponent configures one device connection and its tag points and,
// when Started, polls it into a tag database. Connection settings are exposed
// as design-time properties; points are an editable CSV list.
type DeviceComponent struct {
	gui.Widget
	protocol Protocol
	host     string
	port     int
	unitID   int // Modbus unit/slave id
	rack     int // S7 rack
	slot     int // S7 slot
	periodMs int // poll interval, milliseconds
	pointCSV string

	poller *driver.Poller
}

func init() {
	core.RegisterFactory("gui.DeviceComponent", core.TypeOf((*DeviceComponent)(nil)))
}

// NewDeviceComponent returns a component with sensible Modbus TCP defaults.
func NewDeviceComponent() *DeviceComponent {
	d := new(DeviceComponent)
	d.Init(d)
	d.host = "127.0.0.1"
	d.port = 502
	d.unitID = 1
	d.slot = 1
	d.periodMs = 1000
	return d
}

// --- design-time properties (scalars, so the property sheet + codegen handle them) ---

func (d *DeviceComponent) Protocol() string    { return d.protocol.String() }
func (d *DeviceComponent) SetProtocol(s string) { d.protocol = parseProtocol(s); d.Self().Update() }
func (d *DeviceComponent) Host() string         { return d.host }
func (d *DeviceComponent) SetHost(s string)     { d.host = s; d.Self().Update() }
func (d *DeviceComponent) Port() int            { return d.port }
func (d *DeviceComponent) SetPort(v int)        { d.port = v; d.Self().Update() }
func (d *DeviceComponent) UnitID() int          { return d.unitID }
func (d *DeviceComponent) SetUnitID(v int)      { d.unitID = v }
func (d *DeviceComponent) Rack() int            { return d.rack }
func (d *DeviceComponent) SetRack(v int)        { d.rack = v }
func (d *DeviceComponent) Slot() int            { return d.slot }
func (d *DeviceComponent) SetSlot(v int)        { d.slot = v }
func (d *DeviceComponent) Period() int          { return d.periodMs }
func (d *DeviceComponent) SetPeriod(v int)      { d.periodMs = v }
func (d *DeviceComponent) Points() string       { return d.pointCSV }
func (d *DeviceComponent) SetPoints(s string)   { d.pointCSV = s; d.Self().Update() }

// EnumProperties exposes the connection + point config in the property sheet.
func (d *DeviceComponent) EnumProperties(list core.IPropertyList) {
	list.AddProperty("协议", d.Protocol, d.SetProtocol)
	list.AddProperty("地址", d.Host, d.SetHost)
	list.AddProperty("端口", d.Port, d.SetPort)
	list.AddProperty("从站号", d.UnitID, d.SetUnitID)
	list.AddProperty("机架", d.Rack, d.SetRack)
	list.AddProperty("插槽", d.Slot, d.SetSlot)
	list.AddProperty("周期ms", d.Period, d.SetPeriod)
	list.AddProperty("点表", d.Points, d.SetPoints)
}

// SizeHints gives the designer a compact default box.
func (d *DeviceComponent) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 180, Height: 60}
}

// Draw renders a small labeled box so the component is visible and identifiable
// on the design surface.
func (d *DeviceComponent) Draw(g paint.Painter) {
	w, h := d.Size()
	g.SetBrush1(paint.Color{R: 236, G: 240, B: 245, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()
	g.SetPen1(paint.Color{R: 120, G: 144, B: 168, A: 255}, 1)
	g.Rectangle(0, 0, w, h)
	g.Stroke()
	g.SetBrush1(paint.Color{R: 40, G: 56, B: 72, A: 255})
	g.DrawText1(8, 20, fmt.Sprintf("⚙ %s", strings.ToUpper(d.protocol.String())))
	pts, _ := parsePoints(d.pointCSV)
	g.DrawText1(8, 38, fmt.Sprintf("%s:%d", d.host, d.port))
	g.DrawText1(8, 54, fmt.Sprintf("%d 点", len(pts)))
}

// Start builds the driver + poller from the current config and begins polling
// into tags. Stop it when the screen closes.
func (d *DeviceComponent) Start(tags *core.TagDB) error {
	points, err := parsePoints(d.pointCSV)
	if err != nil {
		return err
	}
	var drv driver.Driver
	switch d.protocol {
	case ProtoS7:
		drv = driver.NewS7(d.host, d.rack, d.slot)
	default:
		drv = driver.NewModbusTCP(fmt.Sprintf("tcp://%s:%d", d.host, d.port), uint8(d.unitID))
	}
	period := time.Duration(d.periodMs) * time.Millisecond
	if period <= 0 {
		period = time.Second
	}
	d.poller = driver.NewPoller(drv, points, tags, period)
	return d.poller.Start()
}

// Stop halts polling and closes the device. Safe if never started.
func (d *DeviceComponent) Stop() {
	if d.poller != nil {
		d.poller.Stop()
		d.poller = nil
	}
}

// parsePoints reads the point list. Each non-empty, non-comment line is
// "tag,address,type,order,access", e.g. "level,hr:0,Float32,ABCD,RO". order and
// access are optional (default ABCD / RO). Whitespace around fields is trimmed.
func parsePoints(csv string) ([]driver.TagPoint, error) {
	var out []driver.TagPoint
	for i, raw := range strings.Split(csv, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, ",")
		for j := range f {
			f[j] = strings.TrimSpace(f[j])
		}
		if len(f) < 3 {
			return nil, fmt.Errorf("device: line %d: need at least tag,address,type", i+1)
		}
		dt, err := parseType(f[2])
		if err != nil {
			return nil, fmt.Errorf("device: line %d: %w", i+1, err)
		}
		order := driver.BigEndian
		if len(f) >= 4 && f[3] != "" {
			if order, err = parseOrder(f[3]); err != nil {
				return nil, fmt.Errorf("device: line %d: %w", i+1, err)
			}
		}
		access := driver.ReadOnly
		if len(f) >= 5 && strings.EqualFold(f[4], "RW") {
			access = driver.ReadWrite
		}
		out = append(out, driver.TagPoint{Tag: f[0], Address: f[1], Type: dt, Order: order, Access: access})
	}
	return out, nil
}

func parseType(s string) (driver.DataType, error) {
	switch strings.ToLower(s) {
	case "bool":
		return driver.TypeBool, nil
	case "int16":
		return driver.TypeInt16, nil
	case "uint16":
		return driver.TypeUInt16, nil
	case "int32":
		return driver.TypeInt32, nil
	case "uint32":
		return driver.TypeUInt32, nil
	case "int64":
		return driver.TypeInt64, nil
	case "uint64":
		return driver.TypeUInt64, nil
	case "float32":
		return driver.TypeFloat32, nil
	case "float64":
		return driver.TypeFloat64, nil
	}
	return 0, fmt.Errorf("unknown type %q", s)
}

func parseOrder(s string) (driver.ByteOrder, error) {
	switch strings.ToUpper(s) {
	case "ABCD", "BIG":
		return driver.BigEndian, nil
	case "DCBA", "LITTLE":
		return driver.LittleEndian, nil
	case "BADC":
		return driver.BigByteSwap, nil
	case "CDAB":
		return driver.LittleByteSwap, nil
	}
	return 0, fmt.Errorf("unknown byte order %q", s)
}
