package driver

import (
	"bytes"
	"testing"
	"time"
)

// TestS7ParseValid checks every documented address form parses to the right
// area / db / byte / bit / isBit.
func TestS7ParseValid(t *testing.T) {
	cases := []struct {
		addr string
		want s7Addr
	}{
		// DB bit
		{"DB1.DBX0.3", s7Addr{area: s7AreaDB, dbNumber: 1, byteOffset: 0, bitOffset: 3, isBit: true}},
		{"DB20.DBX35.7", s7Addr{area: s7AreaDB, dbNumber: 20, byteOffset: 35, bitOffset: 7, isBit: true}},
		{"db1.dbx2.0", s7Addr{area: s7AreaDB, dbNumber: 1, byteOffset: 2, isBit: true}},
		{" DB1.DBX0.0 ", s7Addr{area: s7AreaDB, dbNumber: 1, isBit: true}},
		// DB byte / word / dword (mnemonic only locates the offset)
		{"DB1.DBB4", s7Addr{area: s7AreaDB, dbNumber: 1, byteOffset: 4}},
		{"DB2.DBW10", s7Addr{area: s7AreaDB, dbNumber: 2, byteOffset: 10}},
		{"DB3.DBD12", s7Addr{area: s7AreaDB, dbNumber: 3, byteOffset: 12}},
		{"DB999.DBD0", s7Addr{area: s7AreaDB, dbNumber: 999, byteOffset: 0}},
		// Merker bit and byte/word/dword
		{"M10.1", s7Addr{area: s7AreaM, byteOffset: 10, bitOffset: 1, isBit: true}},
		{"M0.0", s7Addr{area: s7AreaM, isBit: true}},
		{"m3.7", s7Addr{area: s7AreaM, byteOffset: 3, bitOffset: 7, isBit: true}},
		{"MB5", s7Addr{area: s7AreaM, byteOffset: 5}},
		{"MW20", s7Addr{area: s7AreaM, byteOffset: 20}},
		{"MD24", s7Addr{area: s7AreaM, byteOffset: 24}},
		// Inputs (I, German E) and outputs (Q, German A) — bit only
		{"I0.1", s7Addr{area: s7AreaI, byteOffset: 0, bitOffset: 1, isBit: true}},
		{"E0.1", s7Addr{area: s7AreaI, byteOffset: 0, bitOffset: 1, isBit: true}},
		{"I2.7", s7Addr{area: s7AreaI, byteOffset: 2, bitOffset: 7, isBit: true}},
		{"Q0.5", s7Addr{area: s7AreaQ, byteOffset: 0, bitOffset: 5, isBit: true}},
		{"A0.5", s7Addr{area: s7AreaQ, byteOffset: 0, bitOffset: 5, isBit: true}},
		{"q1.0", s7Addr{area: s7AreaQ, byteOffset: 1, isBit: true}},
	}
	for _, c := range cases {
		got, err := parseS7Addr(c.addr)
		if err != nil {
			t.Errorf("parseS7Addr(%q): unexpected error %v", c.addr, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseS7Addr(%q) = %+v, want %+v", c.addr, got, c.want)
		}
	}
}

// TestS7ParseInvalid checks malformed addresses are rejected.
func TestS7ParseInvalid(t *testing.T) {
	cases := []string{
		"",
		"DB1",          // no location
		"DB1.DBX0",     // DBX needs a bit
		"DB1.DBX0.8",   // bit out of range
		"DB1.DBX0.3.1", // trailing garbage
		"DB.DBW0",      // missing DB number
		"DB0.DBW0",     // DB numbers start at 1
		"DB-1.DBW0",    // signed DB number
		"DBX0.3",       // DB number missing entirely
		"DB1.DBQ0",     // bad width mnemonic
		"DB1.DW0",      // location must start with DB
		"DB1DBW0",      // missing dot
		"DB1.DBW",      // missing byte offset
		"DB1.DBW1.2",   // byte form takes no bit
		"M10",          // merker bit needs .bit
		"M10.8",        // bit out of range
		"M.1",          // missing byte offset
		"MB",           // missing byte offset
		"MB1.2",        // byte form takes no bit
		"MW-4",         // signed offset
		"I5",           // input needs .bit
		"I0.9",         // bit out of range
		"E0",           // input needs .bit
		"Q",            // missing byte.bit
		"A",            // missing byte.bit
		"X0.1",         // unknown area
		"40001",        // Modbus address, not S7
	}
	for _, addr := range cases {
		if got, err := parseS7Addr(addr); err == nil {
			t.Errorf("parseS7Addr(%q) = %+v, want error", addr, got)
		}
	}
}

// TestS7BitHelpers checks bit extraction (read path) and read-modify-write bit
// setting (write path) against a known byte.
func TestS7BitHelpers(t *testing.T) {
	const b = byte(0xA5) // 1010 0101: bits 0,2,5,7 set
	want := [8]bool{true, false, true, false, false, true, false, true}
	for bit := 0; bit < 8; bit++ {
		if got := s7Bit(b, bit); got != want[bit] {
			t.Errorf("s7Bit(0xA5, %d) = %v, want %v", bit, got, want[bit])
		}
	}

	set := []struct {
		in   byte
		bit  int
		on   bool
		want byte
	}{
		{0x00, 3, true, 0x08},
		{0xFF, 3, false, 0xF7},
		{0xA5, 1, true, 0xA7},  // neighbours preserved
		{0xA5, 0, false, 0xA4}, // neighbours preserved
		{0xA5, 2, true, 0xA5},  // already set: unchanged
		{0xA5, 1, false, 0xA5}, // already clear: unchanged
	}
	for _, c := range set {
		if got := s7SetBit(c.in, c.bit, c.on); got != c.want {
			t.Errorf("s7SetBit(%#02x, %d, %v) = %#02x, want %#02x", c.in, c.bit, c.on, got, c.want)
		}
	}
}

// TestS7DecodeIntegration feeds known raw buffers — as readArea would return
// them — through Decode, exercising the read path's value decoding without a
// socket. The write path's Encode is checked as the inverse.
func TestS7DecodeIntegration(t *testing.T) {
	cases := []struct {
		raw   []byte
		dt    DataType
		order ByteOrder
		want  interface{}
	}{
		{[]byte{0x41, 0x48, 0x00, 0x00}, TypeFloat32, BigEndian, float32(12.5)},
		{[]byte{0x12, 0x34}, TypeUInt16, BigEndian, uint16(0x1234)},
		{[]byte{0xFF, 0xFE}, TypeInt16, BigEndian, int16(-2)},
		{[]byte{0x03, 0x04, 0x01, 0x02}, TypeInt32, LittleByteSwap, int32(0x01020304)},
	}
	for _, c := range cases {
		got, err := Decode(c.raw, c.dt, c.order)
		if err != nil {
			t.Errorf("Decode(% x, %s, %s): %v", c.raw, c.dt, c.order, err)
			continue
		}
		if got != c.want {
			t.Errorf("Decode(% x, %s, %s) = %v, want %v", c.raw, c.dt, c.order, got, c.want)
		}
		back, err := Encode(c.want, c.dt, c.order)
		if err != nil || !bytes.Equal(back, c.raw) {
			t.Errorf("Encode(%v, %s, %s) = % x, %v, want % x", c.want, c.dt, c.order, back, err, c.raw)
		}
	}
}

// TestS7NotConnected checks constructor defaults and that Read/Write/Close on
// a never-connected driver fail cleanly instead of dialing.
func TestS7NotConnected(t *testing.T) {
	s := NewS7("192.0.2.1", 0, 1) // TEST-NET address; never dialed
	if s.Timeout != 2*time.Second {
		t.Errorf("default Timeout = %v, want 2s", s.Timeout)
	}
	p := TagPoint{Tag: "t", Address: "DB1.DBW0", Type: TypeInt16, Order: BigEndian}
	if _, err := s.ReadPoint(p); err != errS7NotConnected {
		t.Errorf("ReadPoint on unconnected S7: err = %v, want errS7NotConnected", err)
	}
	if err := s.WritePoint(p, int16(1)); err != errS7NotConnected {
		t.Errorf("WritePoint on unconnected S7: err = %v, want errS7NotConnected", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close on unconnected S7: %v", err)
	}
}
