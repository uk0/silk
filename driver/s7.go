package driver

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/robinson/gos7"
)

// S7 is a Driver for Siemens S7 PLCs (S7-300/400/1200/1500) over ISO-on-TCP
// using gos7. Addresses use the classic Siemens syntax:
//
//	DB<n>.DBX<byte>.<bit>   bool in data block n         DB1.DBX0.3
//	DB<n>.DBB<byte>         byte in data block n         DB1.DBB4
//	DB<n>.DBW<byte>         16-bit word                  DB2.DBW10
//	DB<n>.DBD<byte>         32-bit dword                 DB3.DBD12
//	M<byte>.<bit>           merker (flag) bit            M10.1
//	MB<byte> MW<byte> MD<byte>  merker byte/word/dword   MB5 MW20 MD24
//	I<byte>.<bit>           input bit (E accepted)       I0.1  E0.1
//	Q<byte>.<bit>           output bit (A accepted)      Q0.5  A0.5
//
// The mnemonic (DBB/DBW/DBD, MB/MW/MD) only locates the byte offset; the
// number of bytes actually read or written comes from the point's
// DataType.ByteWidth(), so a TypeFloat64 point at DB1.DBD0 transfers 8 bytes.
// Bit addresses (DBX / M x.y / I / Q) require TypeBool; bit 0 is the byte's
// least significant bit, valid bits are 0-7.
type S7 struct {
	Host    string        // IP or hostname; port defaults to 102 when omitted
	Rack    int           // CPU rack (S7-300: 0; S7-1200/1500: 0)
	Slot    int           // CPU slot (S7-300: 2; S7-1200/1500: 0 or 1)
	Timeout time.Duration // dial/request timeout

	handler *gos7.TCPClientHandler
	client  gos7.Client
}

var _ Driver = (*S7)(nil)

var errS7NotConnected = errors.New("driver: S7 not connected")

// NewS7 builds an S7 driver for host (port 102 unless given) with a 2s timeout.
func NewS7(host string, rack, slot int) *S7 {
	return &S7{Host: host, Rack: rack, Slot: slot, Timeout: 2 * time.Second}
}

// Connect dials the PLC and negotiates the ISO-on-TCP / S7 session.
func (s *S7) Connect() error {
	h := gos7.NewTCPClientHandler(s.Host, s.Rack, s.Slot)
	if s.Timeout > 0 {
		h.Timeout = s.Timeout
	}
	if err := h.Connect(); err != nil {
		return err
	}
	s.handler = h
	s.client = gos7.NewClient(h)
	return nil
}

// Close shuts the connection down. Safe to call when not connected.
func (s *S7) Close() error {
	if s.handler == nil {
		return nil
	}
	err := s.handler.Close()
	s.handler, s.client = nil, nil
	return err
}

// ReadPoint reads one point. Bit addresses read the containing byte and
// extract the bit as a bool; numeric addresses read Type.ByteWidth() bytes and
// decode them with the point's byte order.
func (s *S7) ReadPoint(p TagPoint) (interface{}, error) {
	if s.client == nil {
		return nil, errS7NotConnected
	}
	a, err := parseS7Addr(p.Address)
	if err != nil {
		return nil, err
	}
	if a.isBit {
		if p.Type != TypeBool {
			return nil, fmt.Errorf("driver: S7 bit address %q requires Bool, got %s", p.Address, p.Type)
		}
		var b [1]byte
		if err := s.readArea(a, 1, b[:]); err != nil {
			return nil, err
		}
		return s7Bit(b[0], a.bitOffset), nil
	}
	buf := make([]byte, p.Type.ByteWidth())
	if err := s.readArea(a, len(buf), buf); err != nil {
		return nil, err
	}
	return Decode(buf, p.Type, p.Order)
}

// WritePoint writes one point. Numerics are encoded with the point's byte
// order; a bit is read-modify-written inside its byte so neighbouring bits are
// preserved (not atomic against concurrent writers of the same byte).
func (s *S7) WritePoint(p TagPoint, v interface{}) error {
	if s.client == nil {
		return errS7NotConnected
	}
	a, err := parseS7Addr(p.Address)
	if err != nil {
		return err
	}
	if a.isBit {
		if p.Type != TypeBool {
			return fmt.Errorf("driver: S7 bit address %q requires Bool, got %s", p.Address, p.Type)
		}
		var b [1]byte
		if err := s.readArea(a, 1, b[:]); err != nil {
			return err
		}
		b[0] = s7SetBit(b[0], a.bitOffset, asFloat(v) != 0)
		return s.writeArea(a, 1, b[:])
	}
	raw, err := Encode(v, p.Type, p.Order)
	if err != nil {
		return err
	}
	return s.writeArea(a, len(raw), raw)
}

// readArea reads size bytes at a into buf via the area-specific gos7 call.
func (s *S7) readArea(a s7Addr, size int, buf []byte) error {
	switch a.area {
	case s7AreaDB:
		return s.client.AGReadDB(a.dbNumber, a.byteOffset, size, buf)
	case s7AreaM:
		return s.client.AGReadMB(a.byteOffset, size, buf)
	case s7AreaI:
		return s.client.AGReadEB(a.byteOffset, size, buf)
	case s7AreaQ:
		return s.client.AGReadAB(a.byteOffset, size, buf)
	}
	return fmt.Errorf("driver: unknown S7 area %d", a.area)
}

// writeArea writes size bytes from buf at a via the area-specific gos7 call.
func (s *S7) writeArea(a s7Addr, size int, buf []byte) error {
	switch a.area {
	case s7AreaDB:
		return s.client.AGWriteDB(a.dbNumber, a.byteOffset, size, buf)
	case s7AreaM:
		return s.client.AGWriteMB(a.byteOffset, size, buf)
	case s7AreaI:
		return s.client.AGWriteEB(a.byteOffset, size, buf)
	case s7AreaQ:
		return s.client.AGWriteAB(a.byteOffset, size, buf)
	}
	return fmt.Errorf("driver: unknown S7 area %d", a.area)
}

// s7Bit reports bit bit (0 = LSB) of b.
func s7Bit(b byte, bit int) bool { return b&(1<<bit) != 0 }

// s7SetBit returns b with bit bit set or cleared.
func s7SetBit(b byte, bit int, on bool) byte {
	if on {
		return b | 1<<bit
	}
	return b &^ (1 << bit)
}

// s7Area is the PLC memory area an address points into.
type s7Area int

const (
	s7AreaDB s7Area = iota // data block
	s7AreaM                // merker / flags
	s7AreaI                // process inputs (I/E)
	s7AreaQ                // process outputs (Q/A)
)

// s7Addr is a parsed S7 address (see the S7 type doc for the syntax).
type s7Addr struct {
	area       s7Area
	dbNumber   int // DB area only, >= 1
	byteOffset int
	bitOffset  int // meaningful when isBit
	isBit      bool
}

// parseS7Addr parses the classic Siemens address syntax into an s7Addr.
// Matching is case-insensitive and ignores surrounding spaces.
func parseS7Addr(addr string) (s7Addr, error) {
	bad := func(why string) (s7Addr, error) {
		return s7Addr{}, fmt.Errorf("driver: bad S7 address %q: %s", addr, why)
	}
	s := strings.ToUpper(strings.TrimSpace(addr))
	switch {
	case strings.HasPrefix(s, "DB"):
		dot := strings.IndexByte(s, '.')
		if dot < 0 {
			return bad("want DB<n>.DB<X|B|W|D><byte>")
		}
		db, err := s7Int(s[2:dot])
		if err != nil || db < 1 {
			return bad("bad DB number")
		}
		loc := s[dot+1:]
		if len(loc) < 4 || loc[0] != 'D' || loc[1] != 'B' {
			return bad("want DBX/DBB/DBW/DBD after the DB number")
		}
		a := s7Addr{area: s7AreaDB, dbNumber: db}
		kind, num := loc[2], loc[3:]
		if kind == 'X' {
			if a.byteOffset, a.bitOffset, err = s7ByteBit(num); err != nil {
				return bad(err.Error())
			}
			a.isBit = true
			return a, nil
		}
		if kind != 'B' && kind != 'W' && kind != 'D' {
			return bad("want DBX/DBB/DBW/DBD after the DB number")
		}
		if a.byteOffset, err = s7Int(num); err != nil {
			return bad("bad byte offset")
		}
		return a, nil

	case strings.HasPrefix(s, "MB"), strings.HasPrefix(s, "MW"), strings.HasPrefix(s, "MD"):
		off, err := s7Int(s[2:])
		if err != nil {
			return bad("bad byte offset")
		}
		return s7Addr{area: s7AreaM, byteOffset: off}, nil

	case strings.HasPrefix(s, "M"):
		byteOff, bit, err := s7ByteBit(s[1:])
		if err != nil {
			return bad(err.Error())
		}
		return s7Addr{area: s7AreaM, byteOffset: byteOff, bitOffset: bit, isBit: true}, nil

	case strings.HasPrefix(s, "I"), strings.HasPrefix(s, "E"):
		byteOff, bit, err := s7ByteBit(s[1:])
		if err != nil {
			return bad(err.Error())
		}
		return s7Addr{area: s7AreaI, byteOffset: byteOff, bitOffset: bit, isBit: true}, nil

	case strings.HasPrefix(s, "Q"), strings.HasPrefix(s, "A"):
		byteOff, bit, err := s7ByteBit(s[1:])
		if err != nil {
			return bad(err.Error())
		}
		return s7Addr{area: s7AreaQ, byteOffset: byteOff, bitOffset: bit, isBit: true}, nil
	}
	return bad("unknown area (want DB/M/I/Q)")
}

// s7ByteBit parses "<byte>.<bit>" with bit 0-7.
func s7ByteBit(s string) (byteOff, bit int, err error) {
	i := strings.IndexByte(s, '.')
	if i < 0 {
		return 0, 0, errors.New("want <byte>.<bit>")
	}
	if byteOff, err = s7Int(s[:i]); err != nil {
		return 0, 0, errors.New("bad byte offset")
	}
	if bit, err = s7Int(s[i+1:]); err != nil || bit > 7 {
		return 0, 0, errors.New("bad bit number (0-7)")
	}
	return byteOff, bit, nil
}

// s7Int parses a non-negative decimal: digits only, no sign/space/dot.
func s7Int(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty number")
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, errors.New("not a number")
		}
	}
	return strconv.Atoi(s)
}
