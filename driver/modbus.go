package driver

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/simonvetter/modbus"
)

// ModbusTCP is a Driver for Modbus TCP devices (PLCs, gateways, meters).
//
// Point addresses use the scheme "<area>:<offset>", where area is one of
//
//	hr    holding register (16-bit, read/write)
//	ir    input register   (16-bit, read-only)
//	coil  coil             (1 bit,  read/write)
//	di    discrete input   (1 bit,  read-only)
//
// and offset is the 0-based register/coil number, e.g. "hr:0", "ir:100",
// "coil:5". Bool points map to coil/di, numeric points to hr/ir; a type
// spanning several registers (Int32, Float64, ...) occupies RegisterCount()
// consecutive registers from offset, laid out per the point's ByteOrder.
type ModbusTCP struct {
	URL     string        // e.g. "tcp://127.0.0.1:502"
	UnitID  uint8         // slave/unit id
	Timeout time.Duration // per-request timeout

	client *modbus.ModbusClient
}

// NewModbusTCP builds a driver for the device at url (form "tcp://host:port")
// with the given unit id and a 1s request timeout.
func NewModbusTCP(url string, unitID uint8) *ModbusTCP {
	return &ModbusTCP{URL: url, UnitID: unitID, Timeout: time.Second}
}

// Connect dials the device and selects the unit id.
func (m *ModbusTCP) Connect() error {
	c, err := modbus.NewClient(&modbus.ClientConfiguration{URL: m.URL, Timeout: m.Timeout})
	if err != nil {
		return err
	}
	if err := c.Open(); err != nil {
		return err
	}
	if err := c.SetUnitId(m.UnitID); err != nil {
		c.Close()
		return err
	}
	m.client = c
	return nil
}

// Close shuts the connection down. Safe on a never-connected driver.
func (m *ModbusTCP) Close() error {
	if m.client == nil {
		return nil
	}
	err := m.client.Close()
	m.client = nil
	return err
}

// parseModbusAddress splits an "<area>:<offset>" address. area is normalized
// to lower case and must be hr, ir, coil or di; offset is the 0-based
// register/coil number.
func parseModbusAddress(s string) (area string, offset uint16, err error) {
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return "", 0, fmt.Errorf("driver: modbus address %q: want \"<area>:<offset>\", e.g. \"hr:0\"", s)
	}
	area = strings.ToLower(s[:i])
	switch area {
	case "hr", "ir", "coil", "di":
	default:
		return "", 0, fmt.Errorf("driver: modbus address %q: unknown area %q (want hr, ir, coil or di)", s, area)
	}
	n, err := strconv.ParseUint(s[i+1:], 10, 16)
	if err != nil {
		return "", 0, fmt.Errorf("driver: modbus address %q: bad offset %q", s, s[i+1:])
	}
	return area, uint16(n), nil
}

// ReadPoint reads one point: coils/discrete inputs as bool, registers decoded
// through the codec with the point's type and byte order.
func (m *ModbusTCP) ReadPoint(p TagPoint) (interface{}, error) {
	if m.client == nil {
		return nil, fmt.Errorf("driver: modbus %s: not connected", m.URL)
	}
	area, offset, err := parseModbusAddress(p.Address)
	if err != nil {
		return nil, err
	}
	switch area {
	case "coil", "di":
		if p.Type != TypeBool {
			return nil, fmt.Errorf("driver: modbus %s: area %q holds bits, not %s", p.Address, area, p.Type)
		}
		var bits []bool
		if area == "coil" {
			bits, err = m.client.ReadCoils(offset, 1)
		} else {
			bits, err = m.client.ReadDiscreteInputs(offset, 1)
		}
		if err != nil {
			return nil, err
		}
		return bits[0], nil
	default: // hr, ir
		if p.Type == TypeBool {
			return nil, fmt.Errorf("driver: modbus %s: Bool points need a coil or di area", p.Address)
		}
		regType := modbus.HOLDING_REGISTER
		if area == "ir" {
			regType = modbus.INPUT_REGISTER
		}
		regs, err := m.client.ReadRegisters(offset, uint16(p.Type.RegisterCount()), regType)
		if err != nil {
			return nil, err
		}
		return Decode(regsToBytes(regs), p.Type, p.Order)
	}
}

// WritePoint writes one point: bool to a coil, numerics encoded through the
// codec into holding registers. Input registers and discrete inputs are
// read-only by the protocol.
func (m *ModbusTCP) WritePoint(p TagPoint, v interface{}) error {
	if m.client == nil {
		return fmt.Errorf("driver: modbus %s: not connected", m.URL)
	}
	area, offset, err := parseModbusAddress(p.Address)
	if err != nil {
		return err
	}
	switch area {
	case "coil":
		if p.Type != TypeBool {
			return fmt.Errorf("driver: modbus %s: area \"coil\" holds bits, not %s", p.Address, p.Type)
		}
		return m.client.WriteCoil(offset, asFloat(v) != 0)
	case "hr":
		if p.Type == TypeBool {
			return fmt.Errorf("driver: modbus %s: Bool points need a coil area", p.Address)
		}
		b, err := Encode(v, p.Type, p.Order)
		if err != nil {
			return err
		}
		return m.client.WriteRegisters(offset, bytesToRegs(b))
	default: // ir, di
		return fmt.Errorf("driver: modbus %s: area %q is read-only", p.Address, area)
	}
}

// regsToBytes flattens registers to the Modbus wire byte stream (each 16-bit
// register hi byte first), the canonical form the codec's ByteOrder expects.
func regsToBytes(regs []uint16) []byte {
	b := make([]byte, 2*len(regs))
	for i, r := range regs {
		binary.BigEndian.PutUint16(b[2*i:], r)
	}
	return b
}

// bytesToRegs is the inverse of regsToBytes. len(b) must be even, which the
// codec guarantees for every register type.
func bytesToRegs(b []byte) []uint16 {
	regs := make([]uint16, len(b)/2)
	for i := range regs {
		regs[i] = binary.BigEndian.Uint16(b[2*i:])
	}
	return regs
}
