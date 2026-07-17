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
	ProtoOPCUA
	ProtoMQTT
)

func parseProtocol(s string) Protocol {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "s7":
		return ProtoS7
	case "opcua", "opc-ua", "opc":
		return ProtoOPCUA
	case "mqtt":
		return ProtoMQTT
	}
	return ProtoModbusTCP
}

func (p Protocol) String() string {
	switch p {
	case ProtoS7:
		return "s7"
	case ProtoOPCUA:
		return "opcua"
	case ProtoMQTT:
		return "mqtt"
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
	case ProtoOPCUA:
		drv = driver.NewOPCUA(fmt.Sprintf("opc.tcp://%s:%d", d.host, d.port))
	case ProtoMQTT:
		drv = driver.NewMQTT(fmt.Sprintf("tcp://%s:%d", d.host, d.port), fmt.Sprintf("silk-%s-%d", d.host, d.port))
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

// parsePoints delegates to the exported ParsePoints (points.go), which is the
// single parser for the tag point-list CSV format.
func parsePoints(csv string) ([]driver.TagPoint, error) {
	return ParsePoints(csv)
}
