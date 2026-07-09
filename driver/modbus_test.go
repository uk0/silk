package driver

import (
	"fmt"
	"math"
	"math/bits"
	"sync"
	"testing"
	"time"

	"github.com/simonvetter/modbus"
)

// mbHandler is the in-memory device behind the test server: the four Modbus
// data areas, indexed 0..1023.
type mbHandler struct {
	mu    sync.Mutex
	hr    [1024]uint16
	ir    [1024]uint16
	coils [1024]bool
	di    [1024]bool
}

func (h *mbHandler) HandleHoldingRegisters(req *modbus.HoldingRegistersRequest) ([]uint16, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if int(req.Addr)+int(req.Quantity) > len(h.hr) {
		return nil, modbus.ErrIllegalDataAddress
	}
	if req.IsWrite {
		copy(h.hr[req.Addr:], req.Args)
		return nil, nil
	}
	return append([]uint16(nil), h.hr[req.Addr:int(req.Addr)+int(req.Quantity)]...), nil
}

func (h *mbHandler) HandleInputRegisters(req *modbus.InputRegistersRequest) ([]uint16, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if int(req.Addr)+int(req.Quantity) > len(h.ir) {
		return nil, modbus.ErrIllegalDataAddress
	}
	return append([]uint16(nil), h.ir[req.Addr:int(req.Addr)+int(req.Quantity)]...), nil
}

func (h *mbHandler) HandleCoils(req *modbus.CoilsRequest) ([]bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if int(req.Addr)+int(req.Quantity) > len(h.coils) {
		return nil, modbus.ErrIllegalDataAddress
	}
	if req.IsWrite {
		copy(h.coils[req.Addr:], req.Args)
		return nil, nil
	}
	return append([]bool(nil), h.coils[req.Addr:int(req.Addr)+int(req.Quantity)]...), nil
}

func (h *mbHandler) HandleDiscreteInputs(req *modbus.DiscreteInputsRequest) ([]bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if int(req.Addr)+int(req.Quantity) > len(h.di) {
		return nil, modbus.ErrIllegalDataAddress
	}
	return append([]bool(nil), h.di[req.Addr:int(req.Addr)+int(req.Quantity)]...), nil
}

// startMbServer serves h over Modbus TCP on a loopback port and returns the
// URL. It tries a few fixed ports so a stray listener doesn't fail the suite.
func startMbServer(t *testing.T, h *mbHandler) string {
	t.Helper()
	for _, port := range []int{15502, 25502, 35502} {
		url := fmt.Sprintf("tcp://127.0.0.1:%d", port)
		srv, err := modbus.NewServer(&modbus.ServerConfiguration{
			URL:        url,
			Timeout:    30 * time.Second,
			MaxClients: 2,
		}, h)
		if err != nil {
			t.Fatalf("NewServer(%s): %v", url, err)
		}
		if err := srv.Start(); err != nil {
			continue // port in use, try the next
		}
		t.Cleanup(func() { srv.Stop() })
		return url
	}
	t.Fatal("all candidate ports busy for the test modbus server")
	return ""
}

// connectModbus stands up a server around h and returns a connected driver.
func connectModbus(t *testing.T, h *mbHandler) *ModbusTCP {
	t.Helper()
	d := NewModbusTCP(startMbServer(t, h), 1)
	if err := d.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// TestModbusTCPRead seeds the server's four areas and reads every value back
// through the driver, covering multi-register types in three byte orders.
func TestModbusTCPRead(t *testing.T) {
	h := &mbHandler{}
	u32 := uint32(0xF8A432EB) // int32(-123456789), bytes A=F8 B=A4 C=32 D=EB
	rb := bits.ReverseBytes32(u32)
	f32 := math.Float32bits(3.14159)
	h.hr[0] = 0xBEEF
	h.hr[10], h.hr[11] = uint16(u32>>16), uint16(u32) // ABCD (BigEndian)
	h.hr[12], h.hr[13] = uint16(u32), uint16(u32>>16) // CDAB (LittleByteSwap)
	h.hr[14], h.hr[15] = uint16(rb>>16), uint16(rb)   // DCBA (LittleEndian)
	h.ir[100], h.ir[101] = uint16(f32>>16), uint16(f32)
	h.coils[5] = true
	h.di[3] = true

	d := connectModbus(t, h)
	reads := []struct {
		name string
		p    TagPoint
		want interface{}
	}{
		{"uint16 hr", TagPoint{Address: "hr:0", Type: TypeUInt16, Order: BigEndian}, uint16(0xBEEF)},
		{"int32 hr ABCD", TagPoint{Address: "hr:10", Type: TypeInt32, Order: BigEndian}, int32(-123456789)},
		{"int32 hr CDAB", TagPoint{Address: "hr:12", Type: TypeInt32, Order: LittleByteSwap}, int32(-123456789)},
		{"int32 hr DCBA", TagPoint{Address: "hr:14", Type: TypeInt32, Order: LittleEndian}, int32(-123456789)},
		{"float32 ir", TagPoint{Address: "ir:100", Type: TypeFloat32, Order: BigEndian}, float32(3.14159)},
		{"coil", TagPoint{Address: "coil:5", Type: TypeBool}, true},
		{"discrete input", TagPoint{Address: "di:3", Type: TypeBool}, true},
	}
	for _, tc := range reads {
		got, err := d.ReadPoint(tc.p)
		if err != nil {
			t.Errorf("%s: ReadPoint: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s = %v (%T), want %v (%T)", tc.name, got, got, tc.want, tc.want)
		}
	}

	// Area/type mismatches must error, not silently read.
	if _, err := d.ReadPoint(TagPoint{Address: "coil:5", Type: TypeInt16}); err == nil {
		t.Error("numeric read from a coil succeeded, want error")
	}
	if _, err := d.ReadPoint(TagPoint{Address: "hr:0", Type: TypeBool}); err == nil {
		t.Error("bool read from a holding register succeeded, want error")
	}
}

// TestModbusTCPWriteReadBack writes holding registers and a coil, checks the
// server's raw memory, and reads each value back through the driver.
func TestModbusTCPWriteReadBack(t *testing.T) {
	h := &mbHandler{}
	d := connectModbus(t, h)

	pf := TagPoint{Address: "hr:20", Type: TypeFloat32, Order: BigEndian, Access: ReadWrite}
	if err := d.WritePoint(pf, 2.5); err != nil { // tags hand numerics over as float64
		t.Fatalf("WritePoint float32: %v", err)
	}
	fb := math.Float32bits(2.5)
	h.mu.Lock()
	r0, r1 := h.hr[20], h.hr[21]
	h.mu.Unlock()
	if r0 != uint16(fb>>16) || r1 != uint16(fb) {
		t.Errorf("device registers = %04x %04x, want %04x %04x", r0, r1, uint16(fb>>16), uint16(fb))
	}
	if got, err := d.ReadPoint(pf); err != nil || got != float32(2.5) {
		t.Errorf("read back float32 = %v, %v; want 2.5", got, err)
	}

	p16 := TagPoint{Address: "hr:30", Type: TypeInt16, Order: BigEndian, Access: ReadWrite}
	if err := d.WritePoint(p16, -42); err != nil {
		t.Fatalf("WritePoint int16: %v", err)
	}
	if got, err := d.ReadPoint(p16); err != nil || got != int16(-42) {
		t.Errorf("read back int16 = %v, %v; want -42", got, err)
	}

	pc := TagPoint{Address: "coil:7", Type: TypeBool, Access: ReadWrite}
	if err := d.WritePoint(pc, true); err != nil {
		t.Fatalf("WritePoint coil: %v", err)
	}
	if got, err := d.ReadPoint(pc); err != nil || got != true {
		t.Errorf("read back coil = %v, %v; want true", got, err)
	}

	// Protocol-read-only areas must reject writes.
	if err := d.WritePoint(TagPoint{Address: "ir:0", Type: TypeUInt16}, 1); err == nil {
		t.Error("write to input register succeeded, want error")
	}
	if err := d.WritePoint(TagPoint{Address: "di:0", Type: TypeBool}, true); err == nil {
		t.Error("write to discrete input succeeded, want error")
	}
}

// TestParseModbusAddress covers the "<area>:<offset>" scheme, including its
// rejections.
func TestParseModbusAddress(t *testing.T) {
	good := []struct {
		in   string
		area string
		off  uint16
	}{
		{"hr:0", "hr", 0},
		{"ir:100", "ir", 100},
		{"coil:5", "coil", 5},
		{"di:3", "di", 3},
		{"HR:65535", "hr", 65535}, // case-insensitive area, max offset
	}
	for _, tc := range good {
		area, off, err := parseModbusAddress(tc.in)
		if err != nil {
			t.Errorf("parse(%q): %v", tc.in, err)
			continue
		}
		if area != tc.area || off != tc.off {
			t.Errorf("parse(%q) = %s:%d, want %s:%d", tc.in, area, off, tc.area, tc.off)
		}
	}

	bad := []string{"", "hr", "40001", "xx:1", ":5", "hr:", "hr:abc", "hr:-1", "hr:65536", "hr:1.5"}
	for _, in := range bad {
		if _, _, err := parseModbusAddress(in); err == nil {
			t.Errorf("parse(%q) succeeded, want error", in)
		}
	}
}
